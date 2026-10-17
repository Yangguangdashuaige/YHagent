// 网页版前端逻辑：
// 与桌面客户端共用同一套服务端接口（/v1/chat SSE、/v1/session/reset），
// 区别仅在于通信方式：Wails 绑定 -> 浏览器 fetch。
// 浏览器原生 EventSource 不支持 POST 和自定义鉴权头，因此用 fetch + ReadableStream
// 手动解析 SSE（按 \n\n 切分事件块，: 开头的行是服务端心跳，直接忽略）。

const TOKEN_KEY = "yh_token";
const ADMIN_TOKEN_KEY = "yh_admin_token";
const SESSION_KEY = "yh_session_id";

// ---------- DOM ----------
const loginView = document.getElementById("loginView");
const loginForm = document.getElementById("loginForm");
const tokenInput = document.getElementById("tokenInput");
const loginError = document.getElementById("loginError");
const loginBtn = document.getElementById("loginBtn");

const chatView = document.getElementById("chatView");
const messagesEl = document.getElementById("messages");
const inputEl = document.getElementById("input");
const sendBtn = document.getElementById("sendBtn");
const clearBtn = document.getElementById("clearBtn");
const logoutBtn = document.getElementById("logoutBtn");
const imageInput = document.getElementById("imageInput");
const attachBtn = document.getElementById("attachBtn");
const imagePreview = document.getElementById("imagePreview");

// ---------- 会话状态 ----------
let busy = false;
let currentBubble = null;
let answerText = "";
// 待发送图片（已压缩的 data URL），二期：截图识别
let pendingImages = [];
const MAX_IMAGES = 3;
const MAX_IMAGE_EDGE = 1280; // 服务端/模型只需要看清文字，1280px 足够且大幅省流量
const JPEG_QUALITY = 0.82;

// ---------- 工具 ----------

function escapeHtml(s) {
  return s
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

function scrollToBottom() {
  messagesEl.scrollTop = messagesEl.scrollHeight;
}

function getToken() {
  return localStorage.getItem(TOKEN_KEY) || "";
}

function authHeaders(extra) {
  return Object.assign(
    { Authorization: "Bearer " + getToken() },
    extra || {}
  );
}

// 网页版没有 Wails 的 NewSession 绑定，会话ID本地生成（优先用浏览器原生 UUID）
function ensureSessionId() {
  let id = localStorage.getItem(SESSION_KEY);
  if (id) return id;
  if (window.crypto && crypto.randomUUID) {
    id = crypto.randomUUID();
  } else {
    id =
      Date.now().toString(36) + "-" + Math.random().toString(36).slice(2, 10);
  }
  localStorage.setItem(SESSION_KEY, id);
  return id;
}

function addMessage(role, text, images) {
  const wrap = document.createElement("div");
  wrap.className = "msg " + role;
  const bubble = document.createElement("div");
  bubble.className = "bubble";
  (images || []).forEach((src) => {
    const img = document.createElement("img");
    img.className = "msg-image";
    img.src = src;
    img.alt = "用户发送的截图";
    bubble.appendChild(img);
  });
  if (text) {
    const span = document.createElement("span");
    span.innerHTML = escapeHtml(text).replace(/\n/g, "<br>");
    bubble.appendChild(span);
  }
  wrap.appendChild(bubble);
  messagesEl.appendChild(wrap);
  scrollToBottom();
  return bubble;
}

function showTyping(bubble) {
  bubble.innerHTML =
    '<div class="typing"><span></span><span></span><span></span></div>';
}

function setBusy(b) {
  busy = b;
  sendBtn.disabled = b;
  attachBtn.disabled = b;
}

// ---------- 图片处理（二期：截图识别） ----------

// 把任意图片文件压缩到最长边 1280px 的 JPEG data URL，失败时返回 null
function compressImage(file) {
  return new Promise((resolve) => {
    if (!file || !file.type || !file.type.startsWith("image/")) {
      resolve(null);
      return;
    }
    const url = URL.createObjectURL(file);
    const img = new Image();
    img.onload = () => {
      URL.revokeObjectURL(url);
      let { width, height } = img;
      if (width > MAX_IMAGE_EDGE || height > MAX_IMAGE_EDGE) {
        const scale = MAX_IMAGE_EDGE / Math.max(width, height);
        width = Math.round(width * scale);
        height = Math.round(height * scale);
      }
      const canvas = document.createElement("canvas");
      canvas.width = width;
      canvas.height = height;
      const ctx = canvas.getContext("2d");
      ctx.fillStyle = "#000"; // 透明 PNG 转 JPEG 时底色兜底
      ctx.fillRect(0, 0, width, height);
      ctx.drawImage(img, 0, 0, width, height);
      resolve(canvas.toDataURL("image/jpeg", JPEG_QUALITY));
    };
    img.onerror = () => {
      URL.revokeObjectURL(url);
      resolve(null);
    };
    img.src = url;
  });
}

async function addImageFiles(files) {
  if (!files || !files.length || busy) return;
  for (const file of files) {
    if (pendingImages.length >= MAX_IMAGES) {
      alert("最多只能发送 " + MAX_IMAGES + " 张图片");
      break;
    }
    const dataURL = await compressImage(file);
    if (dataURL) {
      pendingImages.push(dataURL);
    }
  }
  renderImagePreview();
}

function renderImagePreview() {
  imagePreview.innerHTML = "";
  if (!pendingImages.length) {
    imagePreview.classList.add("hidden");
    return;
  }
  imagePreview.classList.remove("hidden");
  pendingImages.forEach((src, idx) => {
    const thumb = document.createElement("div");
    thumb.className = "thumb";
    const img = document.createElement("img");
    img.src = src;
    const btn = document.createElement("button");
    btn.className = "thumb-remove";
    btn.type = "button";
    btn.textContent = "×";
    btn.title = "移除";
    btn.addEventListener("click", () => {
      pendingImages.splice(idx, 1);
      renderImagePreview();
    });
    thumb.appendChild(img);
    thumb.appendChild(btn);
    imagePreview.appendChild(thumb);
  });
}

// ---------- 视图切换 ----------

function showLogin(msg) {
  chatView.classList.add("hidden");
  loginView.classList.remove("hidden");
  loginError.textContent = msg || "";
  tokenInput.focus();
}

function enterChat() {
  loginView.classList.add("hidden");
  chatView.classList.remove("hidden");
  ensureSessionId();
  inputEl.focus();
}

// ---------- 服务端通信 ----------

// 校验访问码是否有效（登录时调用，而不是等发消息才发现 401）
async function verifyToken(token) {
  const resp = await fetch("/v1/auth/check", {
    headers: { Authorization: "Bearer " + token },
  });
  return resp.ok;
}

// 流式提问：images 为可选的 data URL 数组；onChunk 每收到一段回答文本被回调；
// onCandidate 在模型联网搜索时收到候选知识卡片；出错时 throw Error（中文信息）
async function streamChat(sessionId, text, images, onChunk, onCandidate) {
  let resp;
  try {
    resp = await fetch("/v1/chat", {
      method: "POST",
      headers: authHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({
        session_id: sessionId,
        message: text,
        images: images || [],
      }),
    });
  } catch (e) {
    throw new Error("无法连接服务器，请检查网络后重试");
  }

  if (resp.status === 401) {
    localStorage.removeItem(TOKEN_KEY);
    showLogin("访问码无效或已过期，请重新输入");
    throw new Error("访问码无效，请重新登录");
  }
  if (!resp.ok || !resp.body) {
    let msg = "服务异常，请稍后再试";
    try {
      const j = await resp.json();
      if (j.error) msg = j.error;
    } catch (_) {}
    throw new Error(msg);
  }

  const reader = resp.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let errMsg = "";

  // SSE 以空行（\n\n）分隔事件；心跳行 ": ping" 没有 event 字段，直接跳过
  const handleBlock = (block) => {
    let event = "";
    let data = "";
    for (const line of block.split("\n")) {
      if (line.startsWith("event:")) event = line.slice(6).trim();
      else if (line.startsWith("data:")) data = line.slice(5).trim();
    }
    if (!event) return;
    const payload = data ? JSON.parse(data) : "";
    if (event === "chunk") onChunk(payload || "");
    else if (event === "candidate" && onCandidate && payload) onCandidate(payload);
    else if (event === "error") errMsg = payload || "服务异常，请稍后再试";
  };

  while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    let sep;
    // 一个 read 分片里可能包含多个完整事件
    while ((sep = buffer.indexOf("\n\n")) >= 0) {
      handleBlock(buffer.slice(0, sep));
      buffer = buffer.slice(sep + 2);
    }
  }
  if (buffer.trim()) handleBlock(buffer);

  if (errMsg) throw new Error(errMsg);
}

async function resetSession() {
  try {
    await fetch("/v1/session/reset", {
      method: "POST",
      headers: authHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ session_id: ensureSessionId() }),
    });
  } catch (e) {
    /* 清空本地上下文即可，重置失败不阻塞界面 */
  }
}

// ---------- 联网知识候选卡片 ----------

// 构造一张"待确认知识卡片"：展示搜索词、来源，管理员可采纳入库或忽略
function buildCandidateCard(cand) {
  const card = document.createElement("div");
  card.className = "kb-card";

  const head = document.createElement("div");
  head.className = "kb-card-head";
  head.textContent = "🔎 联网知识 · 待确认入库";
  card.appendChild(head);

  if (cand.query) {
    const q = document.createElement("div");
    q.className = "kb-card-query";
    q.textContent = "搜索词：" + cand.query;
    card.appendChild(q);
  }

  const sources = Array.isArray(cand.sources) ? cand.sources : [];
  if (sources.length) {
    const list = document.createElement("ul");
    list.className = "kb-card-sources";
    sources.slice(0, 6).forEach((s) => {
      const li = document.createElement("li");
      if (s.url) {
        const a = document.createElement("a");
        a.href = s.url;
        a.target = "_blank";
        a.rel = "noopener noreferrer";
        a.textContent = s.title || s.url;
        li.appendChild(a);
      } else {
        li.textContent = s.title || "";
      }
      list.appendChild(li);
    });
    card.appendChild(list);
  }

  const actions = document.createElement("div");
  actions.className = "kb-card-actions";

  const adoptBtn = document.createElement("button");
  adoptBtn.type = "button";
  adoptBtn.className = "kb-btn kb-btn-primary";
  adoptBtn.textContent = "采纳入库";

  const dismissBtn = document.createElement("button");
  dismissBtn.type = "button";
  dismissBtn.className = "kb-btn";
  dismissBtn.textContent = "仅本次参考";

  // 内联管理员口令输入区（不用 window.prompt：手机浏览器/内嵌 webview 兼容性差）
  const adminRow = document.createElement("div");
  adminRow.className = "kb-admin-row";
  const adminInput = document.createElement("input");
  adminInput.type = "password";
  adminInput.className = "kb-admin-input";
  adminInput.placeholder = "管理员口令";
  // new-password 比 off 更能阻止浏览器密码管理器串填
  adminInput.autocomplete = "new-password";
  adminInput.setAttribute("data-lpignore", "true");
  const adminOkBtn = document.createElement("button");
  adminOkBtn.type = "button";
  adminOkBtn.className = "kb-btn kb-btn-primary";
  adminOkBtn.textContent = "确认入库";
  const adminCancelBtn = document.createElement("button");
  adminCancelBtn.type = "button";
  adminCancelBtn.className = "kb-btn";
  adminCancelBtn.textContent = "取消";
  adminRow.appendChild(adminInput);
  adminRow.appendChild(adminOkBtn);
  adminRow.appendChild(adminCancelBtn);
  adminRow.style.display = "none";

  // 口令错误等内联提示（就在输入框下方，红色，避免“对错无提示”）
  const adminErr = document.createElement("div");
  adminErr.className = "kb-admin-error";

  // 入库进度面板（SSE 驱动：口令校验 → AI 撰写 → 切片向量化 → 落盘）
  const panel = buildIngestPanel();
  panel.el.style.display = "none";

  const ui = {
    actions,
    adminRow,
    adminInput,
    adminErr,
    adoptBtn,
    panel,
  };

  const submitAdmin = () => {
    const token = adminInput.value.trim();
    if (!token) {
      adminInput.focus();
      return;
    }
    adoptCandidate(card, cand, token, ui);
  };

  actions.appendChild(adoptBtn);
  actions.appendChild(dismissBtn);
  card.appendChild(actions);
  card.appendChild(adminRow);
  card.appendChild(adminErr);
  card.appendChild(panel.el);

  adoptBtn.addEventListener("click", () => {
    // 已缓存过口令则直接提交；否则展开内联输入框
    if (getAdminToken()) {
      adoptCandidate(card, cand, getAdminToken(), ui);
      return;
    }
    adminErr.textContent = "";
    actions.style.display = "none";
    adminRow.style.display = "flex";
    adminInput.focus();
  });
  adminOkBtn.addEventListener("click", submitAdmin);
  adminCancelBtn.addEventListener("click", () => {
    adminInput.value = "";
    adminErr.textContent = "";
    adminRow.style.display = "none";
    actions.style.display = "flex";
  });
  adminInput.addEventListener("keydown", (e) => {
    if (e.key === "Enter") submitAdmin();
  });
  dismissBtn.addEventListener("click", () => card.remove());

  return card;
}

// ---------- 入库进度面板 ----------

function getAdminToken() {
  return localStorage.getItem(ADMIN_TOKEN_KEY) || "";
}

// 阶段 key → 中文文案
const INGEST_STAGES = [
  ["auth", "管理员口令校验"],
  ["organize", "AI 整理攻略文档"],
  ["embed", "切片并向量化"],
  ["save", "写入知识库"],
];

// 构造进度面板；返回控制器，供 adoptCandidate 驱动
function buildIngestPanel() {
  const el = document.createElement("div");
  el.className = "kb-progress";

  const head = document.createElement("div");
  head.className = "kb-progress-head";
  const title = document.createElement("span");
  title.textContent = "入库进度";
  const hint = document.createElement("span");
  hint.className = "kb-progress-hint";
  hint.textContent = "AI 整理约需 1-3 分钟，可继续提问";
  const timer = document.createElement("span");
  timer.className = "kb-timer";
  timer.textContent = "00:00";
  head.appendChild(title);
  head.appendChild(hint);
  head.appendChild(timer);

  const bar = document.createElement("div");
  bar.className = "kb-bar";
  const barFill = document.createElement("div");
  barFill.className = "kb-bar-fill";
  bar.appendChild(barFill);

  const stageList = document.createElement("ul");
  stageList.className = "kb-stages";
  const stages = {};
  INGEST_STAGES.forEach(([key, label]) => {
    const li = document.createElement("li");
    li.className = "kb-stage";
    li.dataset.stage = key;
    const dot = document.createElement("span");
    dot.className = "kb-stage-dot";
    const text = document.createElement("span");
    text.className = "kb-stage-label";
    text.textContent = label;
    const sub = document.createElement("em");
    sub.className = "kb-stage-sub";
    li.appendChild(dot);
    li.appendChild(text);
    li.appendChild(sub);
    stageList.appendChild(li);
    stages[key] = { li, sub };
  });

  // AI 撰写的实时预览（原始 Markdown 文本逐块追加）
  const docWrap = document.createElement("details");
  docWrap.className = "kb-doc";
  docWrap.open = true;
  const docSummary = document.createElement("summary");
  docSummary.textContent = "AI 正在撰写的攻略（实时预览）";
  const docBody = document.createElement("div");
  docBody.className = "kb-doc-body";
  docWrap.appendChild(docSummary);
  docWrap.appendChild(docBody);

  const errBox = document.createElement("div");
  errBox.className = "kb-progress-error";

  const retryBtn = document.createElement("button");
  retryBtn.type = "button";
  retryBtn.className = "kb-btn kb-btn-primary kb-retry";
  retryBtn.textContent = "重试";
  retryBtn.style.display = "none";

  el.appendChild(head);
  el.appendChild(bar);
  el.appendChild(stageList);
  el.appendChild(docWrap);
  el.appendChild(errBox);
  el.appendChild(retryBtn);

  let timerId = null;
  let startTs = 0;
  let activeStage = "";

  const fmt = (sec) => {
    const m = String(Math.floor(sec / 60)).padStart(2, "0");
    const s = String(sec % 60).padStart(2, "0");
    return m + ":" + s;
  };

  const startTimer = () => {
    startTs = Date.now();
    timer.textContent = "00:00";
    timerId = setInterval(() => {
      timer.textContent = fmt(Math.floor((Date.now() - startTs) / 1000));
    }, 1000);
  };
  const stopTimer = () => {
    if (timerId) clearInterval(timerId);
    timerId = null;
  };

  const setStage = (key, state, subText) => {
    const item = stages[key];
    if (!item) return;
    item.li.classList.remove("active", "done", "fail");
    if (state) item.li.classList.add(state);
    item.sub.textContent = subText || "";
    if (state === "active") activeStage = key;
  };

  const reset = () => {
    stopTimer();
    timer.textContent = "00:00";
    errBox.textContent = "";
    retryBtn.style.display = "none";
    docBody.textContent = "";
    docWrap.style.display = "";
    barFill.className = "kb-bar-fill";
    barFill.style.width = "";
    Object.keys(stages).forEach((k) => {
      stages[k].li.classList.remove("active", "done", "fail");
      stages[k].sub.textContent = "";
    });
    activeStage = "";
  };

  const setIndeterminate = () => {
    barFill.className = "kb-bar-fill kb-bar-indeterminate";
    barFill.style.width = "";
  };
  const setPercent = (pct) => {
    barFill.className = "kb-bar-fill";
    barFill.style.width = Math.max(0, Math.min(100, pct)) + "%";
  };

  const appendDoc = (delta) => {
    // 防御性截断：预览只用于看进度，不保留超长 DOM
    if (docBody.textContent.length > 12000) return;
    docBody.appendChild(document.createTextNode(delta));
    docBody.scrollTop = docBody.scrollHeight;
  };

  const failStage = (msg, withRetry) => {
    stopTimer();
    if (activeStage) stages[activeStage].li.classList.add("fail");
    errBox.textContent = msg;
    if (withRetry) retryBtn.style.display = "";
  };

  return {
    el,
    retryBtn,
    reset,
    startTimer,
    stopTimer,
    setStage,
    setIndeterminate,
    setPercent,
    appendDoc,
    failStage,
  };
}

// 采纳入库：口令对错通过 SSE 的 ready/403 即时反馈，全程展示真实入库阶段进度。
// 仅在 ready 后才把口令缓存到 localStorage。
async function adoptCandidate(card, cand, adminToken, ui) {
  if (!adminToken) return;
  const { actions, adminRow, adminInput, adminErr, adoptBtn, panel } = ui;

  adoptBtn.disabled = true;
  adminErr.textContent = "";
  actions.style.display = "none";
  adminRow.style.display = "none";

  // 打开进度面板并复位
  panel.el.style.display = "";
  panel.reset();
  panel.startTimer();
  panel.setStage("auth", "active", "正在校验…");
  panel.setIndeterminate();

  // 403 等 SSE 建立前的失败：回到口令输入态（或按钮态），让用户立刻知道对错
  const recoverInput = (msg) => {
    panel.stopTimer();
    panel.el.style.display = "none";
    adoptBtn.disabled = false;
    adminInput.value = "";
    adminErr.textContent = msg;
    adminRow.style.display = "flex";
    adminInput.focus();
  };
  const recoverActions = (msg) => {
    panel.stopTimer();
    panel.el.style.display = "none";
    adoptBtn.disabled = false;
    actions.style.display = "flex";
    panel.failStage(msg, false);
    // 面板被隐藏时把错误直接显示在按钮旁
    adminErr.textContent = msg;
  };

  let resp;
  try {
    resp = await fetch("/v1/knowledge/ingest", {
      method: "POST",
      headers: authHeaders({
        "Content-Type": "application/json",
        "X-Admin-Token": adminToken,
      }),
      body: JSON.stringify({
        query: cand.query || "",
        summary: cand.summary || "",
        sources: cand.sources || [],
      }),
    });
  } catch (e) {
    recoverActions("网络异常，无法连接服务器，请稍后重试");
    return;
  }

  // 鉴权/参数类错误仍以 JSON 返回（SSE 头尚未建立）
  if (resp.status === 403 || !resp.ok || !resp.body) {
    let msg = "入库失败，请稍后再试";
    try {
      const j = await resp.json();
      if (j.error) msg = j.error;
    } catch (_) {}
    if (resp.status === 403) {
      localStorage.removeItem(ADMIN_TOKEN_KEY);
      recoverInput("管理员口令错误，请重新输入");
    } else if (resp.status === 503) {
      recoverActions("服务端未开启入库（未配置 ADMIN_TOKEN）");
    } else {
      recoverActions(msg);
    }
    return;
  }

  // ---- 口令正确：进入 SSE 进度流 ----
  panel.setStage("auth", "done");
  panel.setStage("organize", "active", "等待 AI 响应…");
  localStorage.setItem(ADMIN_TOKEN_KEY, adminToken);

  // “重试”复用当前卡片与候选，重新走一遍（已缓存口令则免输入）
  panel.retryBtn.onclick = () =>
    adoptCandidate(card, cand, getAdminToken() || adminToken, ui);

  const reader = resp.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let errMsg = "";
  let result = null;

  const handleBlock = (block) => {
    let event = "";
    let data = "";
    for (const line of block.split("\n")) {
      if (line.startsWith("event:")) event = line.slice(6).trim();
      else if (line.startsWith("data:")) data = line.slice(5).trim();
    }
    if (!event) return;
    let p = null;
    try {
      p = data ? JSON.parse(data) : {};
    } catch (_) {
      return;
    }
    if (event === "ready") {
      // 已在收到响应头时处理
    } else if (event === "doc") {
      if (p.delta) panel.appendDoc(p.delta);
    } else if (event === "progress") {
      switch (p.stage) {
        case "organizing":
          panel.setStage("organize", "active", "正在整理资料…");
          break;
        case "writing":
          if (p.delta) panel.appendDoc(p.delta);
          break;
        case "chunking":
          panel.setStage("organize", "done");
          panel.setStage("embed", "active", "正在切片…");
          break;
        case "embedding": {
          panel.setStage("organize", "done");
          panel.setStage("embed", "active");
          if (p.total > 0) {
            panel.setPercent(Math.round((p.done / p.total) * 100));
            stagesSetEmbed(panel, p.done, p.total);
            if (p.done >= p.total) {
              panel.setStage("embed", "done");
              panel.setStage("save", "active", "正在落盘…");
            }
          }
          break;
        }
      }
    } else if (event === "done") {
      result = p;
    } else if (event === "error") {
      errMsg = p.error || "入库失败，请稍后再试";
    }
  };

  try {
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      let sep;
      while ((sep = buffer.indexOf("\n\n")) >= 0) {
        handleBlock(buffer.slice(0, sep));
        buffer = buffer.slice(sep + 2);
      }
    }
    if (buffer.trim()) handleBlock(buffer);
  } catch (e) {
    panel.failStage("网络中断，入库结果未知，请稍后重试", true);
    return;
  }

  if (errMsg) {
    panel.failStage(errMsg, true);
    return;
  }
  if (!result) {
    panel.failStage("服务器未返回结果，请重试", true);
    return;
  }

  // ---- 成功：卡片替换为已收录状态 ----
  panel.setStage("save", "done");
  panel.setPercent(100);
  panel.stopTimer();
  const source = result.source || "新文档";
  const chunks = result.chunks != null ? result.chunks : 0;
  // 让用户看清“写入知识库 ✓”后再切绿
  setTimeout(() => {
    card.classList.add("kb-card-done");
    card.innerHTML = "";
    const ok = document.createElement("div");
    ok.className = "kb-card-head";
    ok.textContent = "✅ 已确认收录到知识库";
    const meta = document.createElement("div");
    meta.className = "kb-card-query";
    meta.textContent = source + "（" + chunks + " 个切片），之后提问即可直接检索到";
    card.appendChild(ok);
    card.appendChild(meta);
    scrollToBottom();
  }, 500);
}

// 更新向量化阶段副标题（小工具，保持 handleBlock 可读）
function stagesSetEmbed(panel, done, total) {
  panel.setStage("embed", "active", "向量化 " + done + "/" + total);
}

// ---------- 交互 ----------

loginForm.addEventListener("submit", async (e) => {
  e.preventDefault();
  const token = tokenInput.value.trim();
  if (!token) return;
  loginBtn.disabled = true;
  loginError.textContent = "校验中…";
  try {
    if (await verifyToken(token)) {
      localStorage.setItem(TOKEN_KEY, token);
      loginBtn.disabled = false;
      enterChat();
    } else {
      loginError.textContent = "访问码不正确，请重试";
    }
  } catch (err) {
    loginError.textContent = "无法连接服务器，请稍后再试";
  } finally {
    loginBtn.disabled = false;
  }
});

async function send() {
  const text = inputEl.value.trim();
  const images = pendingImages.slice();
  if ((!text && !images.length) || busy) return;

  addMessage("user", text, images);
  inputEl.value = "";
  inputEl.style.height = "auto";
  pendingImages = [];
  renderImagePreview();
  setBusy(true);

  currentBubble = addMessage("assistant", "");
  showTyping(currentBubble);
  answerText = "";
  const candidates = [];

  try {
    await streamChat(
      ensureSessionId(),
      text,
      images,
      (chunk) => {
        answerText += chunk;
        currentBubble.innerHTML = escapeHtml(answerText).replace(/\n/g, "<br>");
        scrollToBottom();
      },
      (cand) => {
        // 同一次搜索可能重复推送，按 搜索词+摘要 去重
        const dup = candidates.some(
          (c) => c.query === cand.query && c.summary === cand.summary
        );
        if (!dup) candidates.push(cand);
      }
    );
    if (!answerText) {
      currentBubble.textContent = "（没有收到回复内容，请再试一次）";
    }
    // 回答渲染完后再插入候选知识卡片，避免流式 innerHTML 重绘把卡片冲掉
    candidates.forEach((cand) => {
      currentBubble.appendChild(buildCandidateCard(cand));
    });
    if (candidates.length) scrollToBottom();
  } catch (e) {
    currentBubble.classList.add("error");
    currentBubble.textContent = "⚠️ " + (e && e.message ? e.message : String(e));
  } finally {
    currentBubble = null;
    answerText = "";
    setBusy(false);
    inputEl.focus();
  }
}

sendBtn.addEventListener("click", send);

// 图片附件按钮 + 隐藏的文件选择框
attachBtn.addEventListener("click", () => {
  if (!busy) imageInput.click();
});
imageInput.addEventListener("change", async () => {
  if (imageInput.files && imageInput.files.length) {
    await addImageFiles(Array.from(imageInput.files));
  }
  imageInput.value = ""; // 允许再次选择同一个文件
});

// 支持直接在输入框粘贴截图（微信/QQ 截图后 Ctrl+V 即可发送）
inputEl.addEventListener("paste", (e) => {
  const items = e.clipboardData && e.clipboardData.items;
  if (!items) return;
  const files = [];
  for (const item of items) {
    if (item.kind === "file" && item.type.startsWith("image/")) {
      const f = item.getAsFile();
      if (f) files.push(f);
    }
  }
  if (files.length) {
    e.preventDefault();
    addImageFiles(files);
  }
});

inputEl.addEventListener("keydown", (e) => {
  if (e.key === "Enter" && !e.shiftKey) {
    e.preventDefault();
    send();
  }
});

inputEl.addEventListener("input", () => {
  inputEl.style.height = "auto";
  inputEl.style.height = Math.min(inputEl.scrollHeight, 96) + "px";
});

// 清空对话：同时清掉服务端会话上下文，并本地换新会话ID
clearBtn.addEventListener("click", async () => {
  await resetSession();
  localStorage.removeItem(SESSION_KEY);
  ensureSessionId();
  messagesEl.innerHTML = "";
  addMessage(
    "assistant",
    "对话已清空，我们重新开始吧～\n可以问我：价格、密码、钥匙卡、战备配装等。"
  );
  inputEl.focus();
});

// 退出登录：删访问码回到登录页（下次进入需重新输入）
logoutBtn.addEventListener("click", () => {
  localStorage.removeItem(TOKEN_KEY);
  tokenInput.value = "";
  showLogin();
});

// ---------- 启动：已保存访问码则先校验，通过直接进聊天 ----------
(async function init() {
  if (getToken()) {
    try {
      if (await verifyToken(getToken())) {
        enterChat();
        return;
      }
      localStorage.removeItem(TOKEN_KEY);
    } catch (e) {
      // 网络不通时也先进界面，发消息时会给出具体错误，避免登录页直接卡死
      enterChat();
      return;
    }
  }
  showLogin();
})();
