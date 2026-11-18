// app.js —— 桌宠前端逻辑：状态机/走动漫游/气泡/拖拽戳摸/输入/PTT 听写。
// 与 Go 的通道只有两条：window.go.main.App.*（绑定方法）与 window.runtime.EventsOn（事件）。

const SESSION_KEY = "yh_session_id";
const BUBBLE_FADE_MS = 12000;        // 气泡无更新后淡出
const SLEEP_AFTER_MS = 5 * 60 * 1000; // 无交互入睡

const petEl = document.getElementById("pet");
const videoA = document.getElementById("petA");
const videoB = document.getElementById("petB");

// 宠物尺寸随视口自适应（CSS --pet-h），运行期从元素实测
function petW() { return petEl.offsetWidth || 150; }
function petH() { return petEl.offsetHeight || 150; }
const bubbleEl = document.getElementById("bubble");
const bubbleBody = document.getElementById("bubbleBody");
const inputPanel = document.getElementById("inputPanel");
const inputEl = document.getElementById("input");
const sendBtn = document.getElementById("sendBtn");
const menuEl = document.getElementById("menu");
const micDot = document.getElementById("micDot");
const settingsPanel = document.getElementById("settingsPanel");
const sessionListEl = document.getElementById("sessionList");
const historyViewEl = document.getElementById("historyView");
const SESSIONS_KEY = "yh_sessions";

// ---------- Go 绑定安全封装（页面早于 Wails 注入时排队不丢） ----------

function callGo(fn, ...args) {
  try {
    if (window.go && window.go.main && window.go.main.App) {
      return window.go.main.App[fn](...args);
    }
  } catch (e) { console.warn("[go] " + fn, e); }
  return Promise.resolve();
}

// ---------- 动画（双 video 交替消抖：后置位加载新动画，loadeddata 后切前排） ----------

const ANIM = {
  idle: { loop: true },
  walk: { loop: true },
  happy: { loop: true },
  think: { loop: true },
  work: { loop: true },   // 听写中（忙碌点按）
  talk: { loop: true },
  drag: { loop: true },
  sleep: { loop: true },
  celebrate: { loop: false },
  startled: { loop: false },
  poke1: { loop: false },
  poke2: { loop: false },
  poke3: { loop: false },
};

let frontVideo = videoA;
let backVideo = videoB;
let currentAnim = null;

function setAnim(name) {
  if (currentAnim === name) return;
  currentAnim = name;
  const cfg = ANIM[name];
  if (!cfg) return;
  // 后置位加载新动画，loadeddata 后切前排；一次性动画播完回 idle
  const v = backVideo;
  v.loop = cfg.loop;
  v.src = "assets/pet/" + name + ".webm";
  v.onloadeddata = () => {
    v.classList.add("is-front");
    frontVideo.classList.remove("is-front");
    frontVideo.onended = null;
    [frontVideo, backVideo] = [v, frontVideo];
    if (!cfg.loop) {
      v.onended = () => { v.onended = null; setState("idle"); };
    }
    reportHitBoxSoon();
  };
}

// ---------- 状态机 ----------

// idle 自由漫游（随机走停）/ think 等首个 chunk / talk 流式回答中 /
// celebrate 回答完成 / poke 被戳 / drag 拖拽悬空 / sleep 入睡 / work 听写中
let state = "boot";
let walkTimer = null, sleepTimer = null;
let walkDir = 1;

function setState(next) {
  if (state === next) return;
  state = next;
  petEl.classList.toggle("flying", next === "drag");
  switch (next) {
    case "idle":
      setAnim("idle");
      scheduleIdle(); break;
    case "walk":
      setAnim("walk"); break;
    case "think":
      setAnim("think"); break;
    case "talk":
      setAnim("talk"); break;
    case "work":
      setAnim("work"); break;
    case "celebrate":
      setAnim("celebrate"); break;
    case "startled":
      setAnim("startled"); break;
    case "drag":
      setAnim("drag"); break;
    case "sleep":
      setAnim("sleep"); break;
    case "poke": {
      const n = 1 + Math.floor(Math.random() * 3);
      setAnim("poke" + n); break;
    }
  }
}

// 走动 tick：walk 态下水平移动，边界掉头
setInterval(() => {
  if (state !== "walk") return;
  const maxX = window.innerWidth - petW();
  let x = petX() + walkDir * 0.7;
  if (x <= 0) { x = 0; walkDir = 1; }
  if (x >= maxX) { x = maxX; walkDir = -1; }
  setPetX(x);
  petEl.style.transform = walkDir < 0 ? "scaleX(-1)" : "";
}, 16);

function scheduleIdle() {
  clearTimeout(walkTimer);
  clearTimeout(sleepTimer);
  if (state !== "idle") return;
  // 6~16s 后随机开走（聊天气泡活跃时不动，保持安静聆听）
  walkTimer = setTimeout(() => { if (state === "idle") setState("walk"); }, 6000 + Math.random() * 10000);
  sleepTimer = setTimeout(() => { if (state === "idle") setState("sleep"); }, SLEEP_AFTER_MS);
}

function wakeUp() {
  if (state === "sleep") setState("idle");
  else scheduleIdle();
  clearTimeout(sleepTimer);
  sleepTimer = setTimeout(() => { if (state === "idle") setState("sleep"); }, SLEEP_AFTER_MS);
}

// ---------- 宠物位置 ----------

let petLeft = 0;
function petX() { return petLeft; }
function setPetX(x) {
  petLeft = Math.max(0, Math.min(window.innerWidth - petW(), x));
  petEl.style.left = petLeft + "px";
  reportHitBoxSoon();
}

// ---------- 命中盒上报（Go 侧穿透轮询的数据源，100ms 节流） ----------

let hitTimer = null;
function reportHitBoxSoon() {
  if (hitTimer) return;
  hitTimer = setTimeout(() => { hitTimer = null; reportHitBox(); }, 100);
}
function reportHitBox() {
  // 命中盒 = 宠物矩形外扩 10px；输入面板/设置面板/菜单打开时一并纳入可交互区
  const r = petEl.getBoundingClientRect();
  let x = r.left - 10, right = r.right + 10, y = r.top - 10, bottom = r.bottom + 10;
  const merge = el => {
    const er = el.getBoundingClientRect();
    x = Math.min(x, er.left); right = Math.max(right, er.right);
    y = Math.min(y, er.top); bottom = Math.max(bottom, er.bottom);
  };
  if (!inputPanel.classList.contains("hidden")) merge(inputPanel);
  if (!settingsPanel.classList.contains("hidden")) merge(settingsPanel);
  if (!menuEl.classList.contains("hidden")) merge(menuEl);
  callGo("SetPetHitBox", x, y, right - x, bottom - y, window.innerWidth, window.innerHeight);
}

// ---------- 气泡 ----------

let fadeTimer = null;
// sticky=true 的气泡不自动淡出（错误/澄清选项需常驻直到用户处理）
function showBubble(html, cls, sticky) {
  bubbleEl.classList.remove("hidden", "fading");
  bubbleBody.innerHTML = html;
  bubbleBody.className = "bubble-body " + (cls || "");
  positionBubble();
  clearTimeout(fadeTimer);
  if (!sticky) {
    fadeTimer = setTimeout(() => bubbleEl.classList.add("fading"), BUBBLE_FADE_MS);
  }
}
function positionBubble() {
  const bw = bubbleEl.offsetWidth || 300;
  const cx = petX() + petW() / 2;
  // 左右避让：宠物偏左则气泡向右展开，反之向左；整体钳制在窗口内
  let left = cx < window.innerWidth / 2 ? petX() + petW() * 0.4 : cx - bw - petW() * 0.4;
  left = Math.max(8, Math.min(window.innerWidth - bw - 8, left));
  bubbleEl.style.left = left + "px";
  const tail = bubbleEl.querySelector(".bubble-tail");
  const tx = Math.max(18, Math.min(bw - 32, cx - left - 7));
  tail.style.left = tx + "px";
}

// ---------- 轻量 Markdown（转义 + code/strong/链接/表格，与服务端回答格式对齐） ----------

function escapeHtml(s) {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;")
          .replace(/"/g, "&quot;").replace(/'/g, "&#39;");
}
function mdInline(s) {
  return s.replace(/`([^`]+)`/g, '<code class="md-code">$1</code>')
          .replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>")
          .replace(/\[([^\]]+)\]\((https?:\/\/[^)\s]+)\)/g, '$1（<a href="$2" target="_blank" rel="noopener">链接</a>）');
}
function mdRender(text) {
  const lines = escapeHtml(text).split(/\r?\n/);
  let html = "", inTable = false;
  for (const line of lines) {
    if (/^\s*\|/.test(line)) {
      const cells = line.trim().replace(/^\|/, "").replace(/\|$/, "").split("|");
      if (cells.every(c => /^[\s:-]*$/.test(c))) continue; // 分隔行
      if (!inTable) { html += "<table>"; inTable = true; }
      html += "<tr>" + cells.map(c => "<td>" + mdInline(c.trim()) + "</td>").join("") + "</tr>";
      continue;
    }
    if (inTable) { html += "</table>"; inTable = false; }
    if (/^#{1,6}\s+/.test(line)) { html += '<span class="md-h">' + mdInline(line.replace(/^#{1,6}\s+/, "")) + "</span>"; continue; }
    if (/^[-*]\s+/.test(line)) { html += "• " + mdInline(line.replace(/^[-*]\s+/, "")) + "<br>"; continue; }
    if (line.trim() === "") { html += "<br>"; continue; }
    html += mdInline(line) + "<br>";
  }
  if (inTable) html += "</table>";
  return html;
}

// ---------- 会话与对话 ----------

let sessionId = localStorage.getItem(SESSION_KEY);
let busy = false;
let answerText = "";

function ensureSession() {
  if (sessionId) { rememberSid(sessionId); return; }
  callGo("NewSession").then(id => {
    if (id) {
      sessionId = id;
      localStorage.setItem(SESSION_KEY, id);
      rememberSid(id); // 进历史对话列表（设置面板可见）
    }
  });
}

function send(text) {
  text = (text || "").trim();
  if (!text || busy) { if (busy) showBubble("轩还在回答上一个问题哦…", "thinking"); return; }
  hideInput();
  busy = true; answerText = "";
  setState("think");
  showBubble('<span class="thinking">让我想想…</span>', "thinking");
  callGo("Chat", sessionId || "default", text).catch(err => {
    busy = false;
    setState("startled");
    showBubble('<span class="err">' + escapeHtml(String(err)) + "</span>", "err");
  });
}

window.runtime && [
  ["chat:chunk", c => {
    if (state === "think") setState("talk");
    answerText += c;
    showBubble(mdRender(answerText), "");
    // 流式期间锚定气泡
  }],
  ["chat:done", () => {
    busy = false;
    setState("celebrate");
    setTimeout(() => { if (state === "celebrate") setState("idle"); scheduleIdle(); }, 2200);
    showBubble(mdRender(answerText), "");
  }],
  ["chat:error", msg => {
    busy = false;
    setState("startled");
    showBubble('<span class="err">' + escapeHtml(String(msg)) + "</span>", "err", true);
  }],
  ["chat:interrupted", p => {
    busy = false;
    setState("idle");
    showBubble("（回答被中断，已生成内容如上；重新问一次就好）", "thinking");
  }],
  ["chat:clarify", p => {
    busy = false;
    setState("idle");
    let html = escapeHtml(p.text || "需要确认一下：") + '<div class="clarify-opts">';
    for (const o of (p.options || [])) {
      html += '<button class="clarify-opt" data-q="' + escapeHtml(o.label) + '">' + escapeHtml(o.label) +
              (o.description ? "<small>" + escapeHtml(o.description) + "</small>" : "") + "</button>";
    }
    html += "</div>";
    showBubble(html, "", true);
    bubbleBody.querySelectorAll(".clarify-opt").forEach(b => {
      b.onclick = () => send(b.dataset.q);
    });
  }],
  // ---------- PTT 听写 ----------
  ["mic:on", () => {
    micDot.classList.remove("hidden");
    if (!busy) setState("work");
    showBubble('<span class="listening">🎤 在听，请讲…</span>', "listening");
  }],
  ["mic:off", () => micDot.classList.add("hidden")],
  ["asr:partial", p => showBubble('<span class="listening">🎤 ' + escapeHtml(p.text || "") + "</span>", "listening")],
  ["asr:final", p => showBubble('<span class="listening">🎤 ' + escapeHtml(p.text || "") + "</span>", "listening")],
  ["asr:utterance", p => {
    const text = (p.text || "").trim();
    if (!text) { showBubble("（没听清，再试一次？）", "thinking"); return; }
    if (p.autosend) { send(text); return; }
    showInput(text);
  }],
  ["asr:error", p => showBubble('<span class="err">' + escapeHtml(p.message || "听写不可用") + "</span>", "err", true)],
].forEach(([name, fn]) => window.runtime.EventsOn(name, fn));

// ---------- 输入面板 ----------

function showInput(text) {
  settingsPanel.classList.add("hidden");
  inputPanel.classList.remove("hidden");
  positionInput();
  if (text != null) inputEl.value = text;
  inputEl.focus();
  reportHitBox();
}
function hideInput() {
  inputPanel.classList.add("hidden");
  reportHitBox();
}
function positionInput() {
  const iw = inputPanel.offsetWidth || 360;
  let left = petX() - 60;
  left = Math.max(8, Math.min(window.innerWidth - iw - 8, left));
  inputPanel.style.left = left + "px";
  reportHitBox();
}

function submitInput() {
  if (inputEl.composing) return; // 输入法组合态保护
  const t = inputEl.value;
  if (!t.trim()) return;
  inputEl.value = "";
  send(t);
}
sendBtn.addEventListener("click", submitInput);
document.getElementById("closeInputBtn").addEventListener("click", hideInput);
inputEl.addEventListener("keydown", e => {
  if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); submitInput(); }
});
inputEl.addEventListener("compositionstart", () => (inputEl.composing = true));
inputEl.addEventListener("compositionend", () => (inputEl.composing = false));

// Esc 全局兜底：关闭最上层的面板（穿透模式下点空白关不掉，键盘是唯一通道）
window.addEventListener("keydown", e => {
  if (e.key === "Escape") {
    if (!menuEl.classList.contains("hidden")) { menuEl.classList.add("hidden"); callGo("SetDragging", false); }
    else if (!settingsPanel.classList.contains("hidden")) closeSettings();
    else if (!inputPanel.classList.contains("hidden")) hideInput();
  }
});

// ---------- 设置面板（Coopanion 形态：历史对话 + 设置） ----------

const tabHistoryBtn = document.getElementById("tabHistory");
const tabSettingsBtn = document.getElementById("tabSettings");
const pageHistoryEl = document.getElementById("pageHistory");
const pageSettingsEl = document.getElementById("pageSettings");

function openSettings() {
  menuEl.classList.add("hidden");
  hideInput();
  settingsPanel.classList.remove("hidden");
  callGo("SetDragging", true); // 面板期间整窗可交互
  renderSessions();
  initOptionToggles();
  reportHitBox();
}
function closeSettings() {
  settingsPanel.classList.add("hidden");
  historyViewEl.classList.add("hidden");
  sessionListEl.classList.remove("hidden");
  callGo("SetDragging", false);
  reportHitBox();
}
document.getElementById("closeSettingsBtn").addEventListener("click", closeSettings);

tabHistoryBtn.addEventListener("click", () => {
  tabHistoryBtn.classList.add("active"); tabSettingsBtn.classList.remove("active");
  pageHistoryEl.classList.remove("hidden"); pageSettingsEl.classList.add("hidden");
});
tabSettingsBtn.addEventListener("click", () => {
  tabSettingsBtn.classList.add("active"); tabHistoryBtn.classList.remove("active");
  pageSettingsEl.classList.remove("hidden"); pageHistoryEl.classList.add("hidden");
});

// 保存的会话 ID 集合（最新签发的排最前）
function savedSids() {
  try { return JSON.parse(localStorage.getItem(SESSIONS_KEY) || "[]"); } catch { return []; }
}
function rememberSid(sid) {
  if (!sid) return;
  const list = savedSids().filter(s => s !== sid);
  list.unshift(sid);
  localStorage.setItem(SESSIONS_KEY, JSON.stringify(list.slice(0, 30)));
}

function fmtTime(ts) {
  if (!ts) return "";
  const d = new Date(ts * 1000);
  const now = Date.now() / 1000;
  if (now - ts < 86400) return `${d.getHours()}:${String(d.getMinutes()).padStart(2, "0")}`;
  return `${d.getMonth() + 1}/${d.getDate()}`;
}

// 渲染历史会话列表（标题/时间/消息数 + 切换/删除）
function renderSessions() {
  const sids = savedSids();
  if (!sids.length) {
    sessionListEl.innerHTML = '<li><span class="s-title" style="color:#8a94a6">还没有对话记录～</span></li>';
    return;
  }
  callGo("ListSessions", sids).then(list => {
    if (!list || !list.length) {
      sessionListEl.innerHTML = '<li><span class="s-title" style="color:#8a94a6">（服务端没有这些会话的记录，可能已过期清理）</span></li>';
      return;
    }
    list.sort((a, b) => (b.updated_at || 0) - (a.updated_at || 0));
    sessionListEl.innerHTML = "";
    for (const s of list) {
      const li = document.createElement("li");
      if (s.session_id === sessionId) li.classList.add("current");
      li.innerHTML = `<span class="s-title">${escapeHtml(s.title || "（未命名对话）")}</span>` +
        `<span class="s-meta">${fmtTime(s.updated_at)} · ${s.msg_count}条</span>` +
        `<button class="s-del" title="切换到此对话" style="color:#7ec2ff;font-size:12px">切换</button>` +
        `<button class="s-del" title="删除此对话">删</button>`;
      // 点行：面板内回看历史；「切换」：设为当前会话；「删」：删除
      li.addEventListener("click", () => viewHistory(s.session_id, s.title));
      li.querySelectorAll("button")[0].addEventListener("click", e => { e.stopPropagation(); switchSession(s.session_id); });
      li.querySelectorAll("button")[1].addEventListener("click", e => {
        e.stopPropagation();
        callGo("DeleteSession", s.session_id);
        localStorage.setItem(SESSIONS_KEY, JSON.stringify(savedSids().filter(x => x !== s.session_id)));
        if (s.session_id === sessionId) switchToNewSession();
        renderSessions();
      });
      sessionListEl.appendChild(li);
    }
  });
}

// 面板内回看指定会话的历史消息
function viewHistory(sid, title) {
  callGo("SessionHistory", sid).then(msgs => {
    historyViewEl.classList.remove("hidden");
    sessionListEl.style.display = "none";
    let html = `<button class="hback">← 返回列表</button>`;
    if (!msgs || !msgs.length) {
      html += `<div class="hmsg" style="color:#8a94a6">（没有可回看的消息）</div>`;
    } else {
      html += `<div style="color:#8a94a6;font-size:11px;margin-bottom:4px">${escapeHtml(title || "")} 共 ${msgs.length} 条</div>`;
      for (const m of msgs) {
        html += `<div class="hmsg ${m.role === "user" ? "user" : "assistant"}">${m.role === "user" ? "问：" : "答："}${escapeHtml(m.content)}</div>`;
      }
    }
    historyViewEl.innerHTML = html;
    historyViewEl.querySelector(".hback").addEventListener("click", () => {
      historyViewEl.classList.add("hidden");
      sessionListEl.style.display = "";
    });
  });
}

// 切换当前会话
function switchSession(sid) {
  if (busy) { showBubble("轩正在回答中，稍等再切换～", "thinking"); return; }
  sessionId = sid;
  localStorage.setItem(SESSION_KEY, sessionId);
  showBubble("已切换到该对话～", "");
  renderSessions();
}

// 开新对话（菜单/设置面板共用）
function switchToNewSession() {
  if (busy) { showBubble("轩正在回答中，稍等再开新对话～", "thinking"); return false; }
  localStorage.removeItem(SESSION_KEY);
  sessionId = null;
  ensureSession();
  showBubble("新对话开启～", "");
  renderSessions();
  return true;
}

document.getElementById("newChatInPanel").addEventListener("click", switchToNewSession);
document.getElementById("refreshSessions").addEventListener("click", renderSessions);

// 设置页开关：置顶 / PTT / 自动发送（后两者运行时生效并回读初始值）
let optInitDone = false;
function initOptionToggles() {
  if (optInitDone) return;
  optInitDone = true;
  const optPinEl = document.getElementById("optPin");
  const optPTTEl = document.getElementById("optPTT");
  const optAutoSendEl = document.getElementById("optAutoSend");
  optPinEl.checked = pinTop;
  optPinEl.addEventListener("change", () => {
    pinTop = optPinEl.checked;
    localStorage.setItem("yh_pin_top", pinTop ? "1" : "0");
    callGo("SetAlwaysOnTop", pinTop);
  });
  Promise.all([callGo("GetPTTEnabled"), callGo("GetASRAutoSend")]).then(([ptt, auto]) => {
    optPTTEl.checked = !!ptt;
    optAutoSendEl.checked = !!auto;
  });
  optPTTEl.addEventListener("change", () => callGo("SetPTTEnabled", optPTTEl.checked));
  optAutoSendEl.addEventListener("change", () => callGo("SetASRAutoSend", optAutoSendEl.checked));
}

// ---------- 拖拽 / 戳摸 ----------

let dragging = false, dragMoved = false, downAt = 0, downX = 0, downY = 0, grabDX = 0, grabDY = 0;

petEl.addEventListener("pointerdown", e => {
  if (e.button === 2) return; // 右键走菜单
  wakeUp();
  petEl.setPointerCapture(e.pointerId);
  dragging = true; dragMoved = false; downAt = Date.now();
  downX = e.clientX; downY = e.clientY;
  const r = petEl.getBoundingClientRect();
  grabDX = e.clientX - r.left;
  grabDY = e.clientY - r.top;
});
petEl.addEventListener("pointermove", e => {
  if (!dragging) return;
  if (!dragMoved) {
    if (Math.hypot(e.clientX - downX, e.clientY - downY) < 5) return; // 容差内的微动不算拖
    dragMoved = true;
    setState("drag");
    callGo("SetDragging", true);
  }
  setPetX(e.clientX - grabDX);
  const y = Math.max(0, Math.min(window.innerHeight - petH(), e.clientY - grabDY));
  petEl.style.bottom = (window.innerHeight - petH() - y) + "px";
});
petEl.addEventListener("pointerup", e => {
  if (!dragging) return;
  dragging = false;
  callGo("SetDragging", false);
  if (state === "drag") {
    // 落回地面（弹跳缓动），然后开心一下
    petEl.classList.add("dropping");
    petEl.style.bottom = "0px";
    setState("happy");
    setTimeout(() => {
      petEl.classList.remove("dropping");
      setState("idle");
    }, 480);
    return;
  }
  // 未位移的快速点击 = 戳摸：播点击回应或唤起输入
  if (Date.now() - downAt < 350 && !dragMoved) {
    poke();
  }
});

function poke() {
  // 输入/设置面板开着时，点宠物 = 关闭面板（穿透模式下点空白无效，宠物即"关闭按钮"）
  if (!inputPanel.classList.contains("hidden")) { hideInput(); return; }
  if (!settingsPanel.classList.contains("hidden")) { closeSettings(); return; }
  if (busy) { showBubble("轩正在回答中…", "thinking"); return; }
  setState("poke");
  const msgs = ["轩？", "干嘛戳我～", "想问什么就戳出输入框吧", "轩在呢！", "轻点轻点…"];
  showBubble(msgs[Math.floor(Math.random() * msgs.length)], "");
  setTimeout(() => { if (state === "poke") setState("idle"); }, 1600);
  // 300ms 内第二次戳 → 唤起输入面板（戳两下比单击更不易误触）
  const now = Date.now();
  if (now - (poke.last || 0) < 600) showInput();
  poke.last = now;
}

// ---------- 右键菜单 ----------

petEl.addEventListener("contextmenu", e => {
  e.preventDefault();
  menuEl.classList.remove("hidden");
  menuEl.style.left = Math.min(e.clientX, window.innerWidth - 130) + "px";
  menuEl.style.top = Math.max(4, e.clientY - 150) + "px";
  callGo("SetDragging", true); // 菜单期间强制可交互
  reportHitBox();
});
document.addEventListener("pointerdown", e => {
  if (!menuEl.classList.contains("hidden") && !menuEl.contains(e.target)) {
    menuEl.classList.add("hidden");
    callGo("SetDragging", false);
    reportHitBox();
  }
});
menuEl.addEventListener("click", e => {
  const act = e.target.dataset && e.target.dataset.act;
  menuEl.classList.add("hidden");
  callGo("SetDragging", false);
  if (act === "input") {
    showInput();
  } else if (act === "new") {
    switchToNewSession();
  } else if (act === "settings") {
    openSettings();
  } else if (act === "quit") {
    callGo("Quit");
  }
});
// 置顶初始态：跟随上次设置（localStorage），默认开
let pinTop = localStorage.getItem("yh_pin_top") !== "0";

// ---------- 启动 ----------

window.addEventListener("resize", () => {
  setPetX(petX());
  positionBubble();
  positionInput();
  reportHitBox();
});

setPetX(Math.max(0, window.innerWidth / 2 - petW() / 2));
setState("idle");
ensureSession();
if (!pinTop) callGo("SetAlwaysOnTop", false); // 上次关掉过置顶则启动即取消
callGo("ConfigError").then(err => {
  if (err) {
    setState("startled");
    showBubble('<span class="err">' + escapeHtml(String(err)) + "</span>", "err", true);
    return;
  }
  showBubble("指挥官好，我是小小轩～<br>戳两下我提问，<b>按住左 Alt 说话</b>；右键有菜单（设置/历史对话）。", "");
});
