// 前端逻辑：只通过 Wails 绑定（window.go.main.App）与 Go 通信，走单一通道。
// 流式回答不依赖请求的 Promise，而是监听 chat:chunk / chat:done / chat:error 三个事件。

const SESSION_KEY = "yh_session_id";

let sessionId = localStorage.getItem(SESSION_KEY);
let busy = false;
let currentBubble = null;
let answerText = "";
let lastQuestion = ""; // 本轮提问原文（反馈接口关联"这条回答对应的问题"）

const messagesEl = document.getElementById("messages");
const inputEl = document.getElementById("input");
const sendBtn = document.getElementById("sendBtn");
const pinBtn = document.getElementById("pinBtn");
const sessionsBtn = document.getElementById("sessionsBtn");
const newChatBtn = document.getElementById("newChatBtn");
const sessionSidebar = document.getElementById("sessionSidebar");
const sessionListEl = document.getElementById("sessionList");
const minBtn = document.getElementById("minBtn");
const closeBtn = document.getElementById("closeBtn");
let pinned = false;

// ---------- DOM 工具 ----------

function escapeHtml(s) {
  return s
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

function scrollToBottom() {
  // 智能跟随：仅当用户在底部附近时才跟随流式输出；上滚回看时不打扰
  if (!stickBottom) return;
  messagesEl.scrollTop = messagesEl.scrollHeight;
}

let stickBottom = true;
messagesEl.addEventListener("scroll", () => {
  const nearBottom =
    messagesEl.scrollHeight - messagesEl.scrollTop - messagesEl.clientHeight < 80;
  stickBottom = nearBottom;
});

// ---------- 轻量 Markdown 渲染（与网页版同款） ----------
// 输入先整体 escapeHtml；标题渲染为小节条而非大标题；链接仅放行 http(s)。

function mdInline(s) {
  return s
    .replace(/`([^`]+)`/g, '<code class="md-code">$1</code>')
    .replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>")
    .replace(/\[([^\]]+)\]\((https?:\/\/[^)\s]+)\)/g, '$1（<a href="$2" target="_blank" rel="noopener">链接</a>）');
}

function mdParseTableRow(line) {
  if (!/^\s*\|/.test(line)) return null;
  let cells = line.trim().replace(/^\|/, "").replace(/\|$/, "").split("|");
  return cells.map((c) => c.trim());
}

function renderMarkdown(src) {
  const lines = escapeHtml(src).split("\n");
  let html = "";
  let i = 0;
  const isBlockStart = (l) =>
    /^\s*(#{1,6}\s|[-*]\s|\d+\.\s|>|&gt;|\|)/.test(l) || /^\s*(-{3,}|\*{3,}|_{3,})\s*$/.test(l);

  while (i < lines.length) {
    const line = lines[i];

    if (/^\s*(-{3,}|\*{3,}|_{3,})\s*$/.test(line)) {
      html += '<div class="md-hr"></div>';
      i++;
      continue;
    }

    const hm = line.match(/^\s*#{1,6}\s+(.*)$/);
    if (hm) {
      html += '<div class="md-h">' + mdInline(hm[1]) + "</div>";
      i++;
      continue;
    }

    const headCells = mdParseTableRow(line);
    if (
      headCells &&
      i + 1 < lines.length &&
      /^\s*\|?[\s:|-]+\|?\s*$/.test(lines[i + 1]) &&
      lines[i + 1].includes("-")
    ) {
      let rows = [];
      let j = i + 2;
      while (j < lines.length) {
        const cells = mdParseTableRow(lines[j]);
        if (!cells) break;
        rows.push(cells);
        j++;
      }
      html += '<table class="md-table"><thead><tr>';
      headCells.forEach((c) => {
        html += "<th>" + mdInline(c) + "</th>";
      });
      html += "</tr></thead><tbody>";
      rows.forEach((cells) => {
        html += "<tr>";
        headCells.forEach((_, ci) => {
          html += "<td>" + mdInline(cells[ci] || "") + "</td>";
        });
        html += "</tr>";
      });
      html += "</tbody></table>";
      i = j;
      continue;
    }

    // 引用块（escapeHtml 已把 > 转成 &gt;，两种都识别）
    if (/^\s*(>|&gt;)/.test(line)) {
      let buf = [];
      while (i < lines.length && /^\s*(>|&gt;)/.test(lines[i])) {
        buf.push(lines[i].replace(/^\s*(>|&gt;)\s?/, ""));
        i++;
      }
      html += '<div class="md-quote">' + mdInline(buf.join("<br>")) + "</div>";
      continue;
    }

    if (/^\s*[-*]\s+/.test(line)) {
      html += '<ul class="md-list">';
      while (i < lines.length && /^\s*[-*]\s+/.test(lines[i])) {
        html += "<li>" + mdInline(lines[i].replace(/^\s*[-*]\s+/, "")) + "</li>";
        i++;
      }
      html += "</ul>";
      continue;
    }
    if (/^\s*\d+\.\s+/.test(line)) {
      html += '<ol class="md-list">';
      while (i < lines.length && /^\s*\d+\.\s+/.test(lines[i])) {
        html += "<li>" + mdInline(lines[i].replace(/^\s*\d+\.\s+/, "")) + "</li>";
        i++;
      }
      html += "</ol>";
      continue;
    }

    if (!line.trim()) {
      i++;
      continue;
    }

    let buf = [];
    while (i < lines.length && lines[i].trim() && !isBlockStart(lines[i])) {
      buf.push(lines[i]);
      i++;
    }
    if (buf.length) {
      html += '<div class="md-p">' + mdInline(buf.join("<br>")) + "</div>";
    }
  }
  return html;
}

// 新增一条消息气泡，返回 bubble 元素
function addMessage(role, text) {
  const wrap = document.createElement("div");
  wrap.className = "msg " + role;
  const bubble = document.createElement("div");
  bubble.className = "bubble";
  if (text) bubble.innerHTML = escapeHtml(text).replace(/\n/g, "<br>");
  wrap.appendChild(bubble);
  messagesEl.appendChild(wrap);
  scrollToBottom();
  return bubble;
}

function showTyping(bubble) {
  bubble.innerHTML =
    '<div class="typing"><span></span><span></span><span></span></div>';
}

// ---------- 回答反馈（👍👎 自学习闭环） ----------
// 点赞：该问答对自动进入服务端 few-shot 案例库，之后相似问题会注入这条成功答法；
// 点踩：落库供失败模式分析。提交后按钮固化防重复；失败只给轻提示，不打扰玩家。

function appendFeedbackBar(bubble, question, answer) {
  if (!bubble || !answer) return;
  const bar = document.createElement("div");
  bar.className = "feedback-bar";
  const up = document.createElement("button");
  up.type = "button";
  up.className = "fb-btn";
  up.title = "这条回答有帮助";
  up.textContent = "👍";
  const down = document.createElement("button");
  down.type = "button";
  down.className = "fb-btn";
  down.title = "这条回答有问题";
  down.textContent = "👎";
  const hint = document.createElement("span");
  hint.className = "fb-hint";
  bar.appendChild(up);
  bar.appendChild(down);
  bar.appendChild(hint);
  bubble.appendChild(bar);

  let voted = false;
  const vote = (rating, btn) => {
    if (voted) return;
    voted = true;
    up.disabled = true;
    down.disabled = true;
    btn.classList.add("voted");
    hint.textContent = "提交中…";
    const sid = sessionId || "";
    window.go.main.App
      .SubmitFeedback(sid, rating, (question || "").slice(0, 2000), (answer || "").slice(0, 8000))
      .then(() => {
        hint.textContent = rating > 0 ? "已记录，谢谢！" : "已记录，我们会改进";
      })
      .catch(() => {
        hint.textContent = "记录失败（不影响使用）";
      });
  };
  up.addEventListener("click", () => vote(1, up));
  down.addEventListener("click", () => vote(-1, down));
}

function setBusy(b) {
  busy = b;
  sendBtn.disabled = b;
  inputEl.disabled = false;
}

// ---------- 后端事件 ----------

window.runtime.EventsOn("chat:chunk", (chunk) => {
  if (!currentBubble) return;
  // 性能关键路径：流式期间只追加文本节点（O(1)）；逐 chunk 全量重渲染
  // Markdown 是 O(n²)，长回答会卡死 WebView。done 后统一渲染一次。
  if (!answerText) currentBubble.innerHTML = ""; // 首个分片清掉打字指示
  answerText += chunk || "";
  currentBubble.classList.add("stream-text");
  currentBubble.appendChild(document.createTextNode(chunk || ""));
  scrollToBottom();
});

window.runtime.EventsOn("chat:done", () => {
  if (currentBubble) {
    // 流结束：一次性渲染成 Markdown 排版
    currentBubble.classList.remove("stream-text");
    if (answerText) currentBubble.innerHTML = renderMarkdown(answerText);
    else currentBubble.textContent = "（没有收到回复内容，请再试一次）";
    // 反馈条（👍👎）：点赞进 few-shot 案例库，点踩供失败分析
    appendFeedbackBar(currentBubble, lastQuestion, answerText);
    // 侧边栏展开时刷新会话列表（标题/活跃时间更新）
    if (!sessionSidebar.classList.contains("hidden")) renderSessions();
  }
  currentBubble = null;
  answerText = "";
  setBusy(false);
  inputEl.focus();
  // busy 期间玩家点了 clarify 选项：流结束后自动发送（不吞点击）
  if (pendingClarifyText) {
    const t = pendingClarifyText;
    pendingClarifyText = null;
    inputEl.value = t;
    send();
  }
});

window.runtime.EventsOn("chat:error", (msg) => {
  if (currentBubble) {
    currentBubble.classList.remove("stream-text");
    currentBubble.classList.add("error");
    currentBubble.textContent = "⚠️ " + (msg || "服务异常，请稍后再试");
  }
  currentBubble = null;
  answerText = "";
  setBusy(false);
  // 出错时不自动发送排队的选项（防错误风暴）：留给玩家手动决定
  if (pendingClarifyText) {
    inputEl.value = pendingClarifyText;
    pendingClarifyText = null;
  }
  inputEl.focus();
});

// ask_user 澄清选项卡：模型拿不准时列出候选方向，玩家点选/自定义输入
// → 文本进主输入框触发发送（作为下一轮消息，模型看到明示选择）。
// 竞态处理：clarify 到达时流仍在收尾（busy=true），点选不吞——排队
// pendingClarifyText，chat:done 解除 busy 后自动发送。
let pendingClarifyText = null;

window.runtime.EventsOn("chat:clarify", (ev) => {
  if (!ev || !Array.isArray(ev.options) || !ev.options.length) return;
  const card = document.createElement("div");
  card.className = "clarify-card";
  const q = document.createElement("div");
  q.className = "clarify-q";
  q.textContent = "❓ " + (ev.text || "想先确认一下你的需求：");
  card.appendChild(q);
  const btns = document.createElement("div");
  btns.className = "clarify-btns";
  card.appendChild(btns);

  let used = false;
  const choose = (text, btn) => {
    if (used) return;
    used = true;
    card.querySelectorAll("button").forEach((b) => (b.disabled = true));
    if (btn) btn.classList.add("chosen");
    if (busy) {
      pendingClarifyText = text;
      const tip = document.createElement("div");
      tip.className = "clarify-meta";
      tip.textContent = "✓ 已选择，将在本轮回答结束后自动发送";
      card.appendChild(tip);
      return;
    }
    inputEl.value = text;
    send();
  };

  ev.options.forEach((o) => {
    if (!o || !o.label) return;
    const b = document.createElement("button");
    b.type = "button";
    b.className = "clarify-opt";
    const label = document.createElement("b");
    label.textContent = o.label;
    b.appendChild(label);
    if (o.description) {
      const d = document.createElement("span");
      d.textContent = o.description;
      b.appendChild(d);
    }
    b.addEventListener("click", () => choose(o.label, b));
    btns.appendChild(b);
  });

  // 自定义输入：都不满意时玩家自己写
  const other = document.createElement("div");
  other.className = "clarify-other";
  const inp = document.createElement("input");
  inp.type = "text";
  inp.className = "clarify-input";
  inp.placeholder = "都不合适？直接输入你的想法…";
  const sendBtn2 = document.createElement("button");
  sendBtn2.type = "button";
  sendBtn2.className = "clarify-send";
  sendBtn2.textContent = "发送";
  const submitOther = () => {
    const t = inp.value.trim();
    if (t) choose(t, null);
  };
  sendBtn2.addEventListener("click", submitOther);
  inp.addEventListener("keydown", (e) => {
    if (e.key === "Enter" && !e.isComposing) submitOther();
  });
  other.appendChild(inp);
  other.appendChild(sendBtn2);
  card.appendChild(other);

  messagesEl.appendChild(card);
  scrollToBottom();
});

// ---------- 交互 ----------

async function ensureSession() {
  if (sessionId) return;
  sessionId = await window.go.main.App.NewSession();
  localStorage.setItem(SESSION_KEY, sessionId);
}

async function send() {
  const text = inputEl.value.trim();
  if (!text || busy) return;

  stickBottom = true; // 用户发出新消息 → 恢复跟随到底部
  addMessage("user", text);
  inputEl.value = "";
  inputEl.style.height = "auto";
  setBusy(true);

  lastQuestion = text;
  currentBubble = addMessage("assistant", "");
  showTyping(currentBubble);
  answerText = "";

  try {
    // 方法会立即返回（后端异步执行），真正内容走事件推送
    await window.go.main.App.Chat(sessionId, text);
  } catch (e) {
    currentBubble.classList.add("error");
    currentBubble.textContent = "⚠️ " + (e && e.message ? e.message : String(e));
    currentBubble = null;
    answerText = "";
    setBusy(false);
  }
}

sendBtn.addEventListener("click", send);

inputEl.addEventListener("keydown", (e) => {
  // IME 组合期间（敲拼音/选候选词）的按键不触发发送：
  // 组合中按回车是"上屏候选词"，此时 isComposing=true / keyCode=229，
  // 若不放过会 preventDefault 阻断上屏并把拼音原文误发出去
  if (e.isComposing || e.keyCode === 229) return;
  if (e.key === "Enter" && !e.shiftKey) {
    e.preventDefault();
    send();
  }
});

// 输入框随内容自动增高
inputEl.addEventListener("input", () => {
  inputEl.style.height = "auto";
  inputEl.style.height = Math.min(inputEl.scrollHeight, 96) + "px";
});

// 窗口置顶切换
pinBtn.addEventListener("click", async () => {
  pinned = !pinned;
  await window.go.main.App.SetAlwaysOnTop(pinned);
  pinBtn.classList.toggle("active", pinned);
});

// 会话列表开关（悬浮窗空间有限：默认收起，🗂️ 切换）
sessionsBtn.addEventListener("click", () => {
  sessionSidebar.classList.toggle("hidden");
  if (!sessionSidebar.classList.contains("hidden")) renderSessions();
});

// ---------- 多会话管理（侧边栏） ----------
// 会话列表存 localStorage（yh_session_ids，JSON 数组，新→旧）：客户端上报
// 本地记录的 sid 集合，服务端返回摘要（标题/最后活跃/消息数）。
const SESSIONS_KEY = "yh_session_ids";

function loadSids() {
  try {
    const arr = JSON.parse(localStorage.getItem(SESSIONS_KEY) || "[]");
    return Array.isArray(arr) ? arr.filter((s) => typeof s === "string") : [];
  } catch (e) {
    return [];
  }
}

function saveSids(sids) {
  if (sids.length > 50) sids = sids.slice(0, 50);
  localStorage.setItem(SESSIONS_KEY, JSON.stringify(sids));
}

function rememberSid(sid) {
  if (!sid) return;
  const sids = loadSids().filter((s) => s !== sid);
  sids.unshift(sid);
  saveSids(sids);
}

function forgetSid(sid) {
  saveSids(loadSids().filter((s) => s !== sid));
}

function fmtTimeAgo(ts) {
  if (!ts) return "";
  const diff = Date.now() / 1000 - ts;
  if (diff < 60) return "刚刚";
  if (diff < 3600) return Math.floor(diff / 60) + " 分钟前";
  if (diff < 86400) return Math.floor(diff / 3600) + " 小时前";
  return Math.floor(diff / 86400) + " 天前";
}

// 拉取并渲染会话列表；服务端无记录的 sid（未开始/过期）显示"新对话"占位。
// 渲染失败静默（列表是增值能力，不阻塞聊天）。
async function renderSessions() {
  const sids = loadSids();
  if (!sids.length) {
    sessionListEl.innerHTML = "";
    return;
  }
  let items = sids.map((sid) => ({ session_id: sid, title: "新对话", updated_at: 0 }));
  try {
    const got = await window.go.main.App.ListSessions(sids);
    if (Array.isArray(got) && got.length) {
      const byId = new Map(got.map((s) => [s.session_id, s]));
      items = sids.map((sid) => byId.get(sid) || { session_id: sid, title: "新对话", updated_at: 0 });
    }
  } catch (e) { /* 网络异常：退回占位列表 */ }
  sessionListEl.innerHTML = "";
  items.forEach((it) => {
    const li = document.createElement("li");
    li.className = "session-item" + (it.session_id === sessionId ? " active" : "");
    const info = document.createElement("div");
    info.className = "session-info";
    const title = document.createElement("div");
    title.className = "session-title";
    title.textContent = it.title || "新对话";
    title.title = it.title || "新对话";
    const meta = document.createElement("div");
    meta.className = "session-meta";
    meta.textContent = it.updated_at ? fmtTimeAgo(it.updated_at) : "未开始";
    info.appendChild(title);
    info.appendChild(meta);
    info.addEventListener("click", () => switchSession(it.session_id));
    const del = document.createElement("button");
    del.type = "button";
    del.className = "session-del";
    del.title = "删除该对话";
    del.textContent = "×";
    del.addEventListener("click", (e) => {
      e.stopPropagation();
      deleteSession(it.session_id);
    });
    li.appendChild(info);
    li.appendChild(del);
    sessionListEl.appendChild(li);
  });
}

// 切换会话：换当前 sid、清空消息区、拉取该会话可见历史
async function switchSession(sid) {
  if (busy || !sid || sid === sessionId) return;
  sessionId = sid;
  localStorage.setItem(SESSION_KEY, sid);
  messagesEl.innerHTML = "";
  renderSessions();
  let msgs = [];
  try {
    msgs = await window.go.main.App.SessionHistory(sid);
  } catch (e) { /* 历史拉取失败：显示欢迎语 */ }
  if (Array.isArray(msgs) && msgs.length) {
    let lastUserText = "";
    msgs.forEach((m) => {
      if (!m || !m.content) return;
      if (m.role === "user") {
        lastUserText = m.content;
        addMessage("user", m.content);
      } else if (m.role === "assistant") {
        const bubble = addMessage("assistant", "");
        bubble.innerHTML = renderMarkdown(m.content);
        appendFeedbackBar(bubble, lastUserText, m.content);
      }
    });
    scrollToBottom();
  } else {
    addMessage("assistant", "切换到了一个新对话，有什么可以帮你？");
  }
  inputEl.focus();
}

// 开新对话：签发新 sid（旧会话原样保留在列表里，随时切回）
newChatBtn.addEventListener("click", async () => {
  if (busy) return;
  try {
    sessionId = await window.go.main.App.NewSession();
    localStorage.setItem(SESSION_KEY, sessionId);
    rememberSid(sessionId);
  } catch (e) { /* 签发失败：保留当前会话继续聊 */ }
  messagesEl.innerHTML = "";
  addMessage("assistant", "新对话开始～\n可以问我：价格、密码、钥匙卡、战备配装、改枪方案等。");
  renderSessions();
  inputEl.focus();
});

// 删除会话：服务端清上下文+落库+解绑，本地列表移除；删的是当前会话则自动开新对话
async function deleteSession(sid) {
  if (busy) return;
  try {
    await window.go.main.App.DeleteSession(sid);
  } catch (e) { /* 删除失败也先移除本地记录，避免死条目反复出现 */ }
  forgetSid(sid);
  if (sid === sessionId) {
    newChatBtn.click();
  } else {
    renderSessions();
  }
}

// 最小化 / 关闭：直接使用 Wails 注入的官方 window.runtime 窗口 API
minBtn.addEventListener("click", () => {
  window.runtime.WindowMinimise();
});
closeBtn.addEventListener("click", () => {
  window.runtime.Quit();
});

// 启动：确保会话ID就绪 → 恢复当前会话历史（重开窗口不再失忆）→ 聚焦输入框
ensureSession().then(async () => {
  rememberSid(sessionId);
  let msgs = [];
  try {
    msgs = await window.go.main.App.SessionHistory(sessionId);
  } catch (e) { /* 历史拉取失败：保留欢迎语 */ }
  if (Array.isArray(msgs) && msgs.length) {
    messagesEl.innerHTML = "";
    let lastUserText = "";
    msgs.forEach((m) => {
      if (!m || !m.content) return;
      if (m.role === "user") {
        lastUserText = m.content;
        addMessage("user", m.content);
      } else if (m.role === "assistant") {
        const bubble = addMessage("assistant", "");
        bubble.innerHTML = renderMarkdown(m.content);
        appendFeedbackBar(bubble, lastUserText, m.content);
      }
    });
    scrollToBottom();
  }
  inputEl.focus();
});
