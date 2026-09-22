// 网页版前端逻辑：
// 与桌面客户端共用同一套服务端接口（/v1/chat SSE、/v1/session/reset），
// 区别仅在于通信方式：Wails 绑定 -> 浏览器 fetch。
// 浏览器原生 EventSource 不支持 POST 和自定义鉴权头，因此用 fetch + ReadableStream
// 手动解析 SSE（按 \n\n 切分事件块，: 开头的行是服务端心跳，直接忽略）。

const TOKEN_KEY = "yh_token";
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

// ---------- 会话状态 ----------
let busy = false;
let currentBubble = null;
let answerText = "";

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

function addMessage(role, text) {
  const wrap = document.createElement("div");
  wrap.className = "msg " + role;
  const bubble = document.createElement("div");
  bubble.className = "bubble";
  if (text) bubble.innerHTML = escapeHtml(text).replace(/\n/g, "<br>");
  wrap.appendChild(bubble);
  messagesEl.appendChild(bubble);
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

// 流式提问：onChunk 每收到一段回答文本被回调；出错时 throw Error（中文信息）
async function streamChat(sessionId, text, onChunk) {
  let resp;
  try {
    resp = await fetch("/v1/chat", {
      method: "POST",
      headers: authHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ session_id: sessionId, message: text }),
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
  if (!text || busy) return;

  addMessage("user", text);
  inputEl.value = "";
  inputEl.style.height = "auto";
  setBusy(true);

  currentBubble = addMessage("assistant", "");
  showTyping(currentBubble);
  answerText = "";

  try {
    await streamChat(ensureSessionId(), text, (chunk) => {
      answerText += chunk;
      currentBubble.innerHTML = escapeHtml(answerText).replace(/\n/g, "<br>");
      scrollToBottom();
    });
    if (!answerText) {
      currentBubble.textContent = "（没有收到回复内容，请再试一次）";
    }
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
