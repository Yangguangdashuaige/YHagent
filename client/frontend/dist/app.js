// 前端逻辑：只通过 Wails 绑定（window.go.main.App）与 Go 通信，走单一通道。
// 流式回答不依赖请求的 Promise，而是监听 chat:chunk / chat:done / chat:error 三个事件。

const SESSION_KEY = "yh_session_id";

let sessionId = localStorage.getItem(SESSION_KEY);
let busy = false;
let currentBubble = null;
let answerText = "";

const messagesEl = document.getElementById("messages");
const inputEl = document.getElementById("input");
const sendBtn = document.getElementById("sendBtn");
const pinBtn = document.getElementById("pinBtn");
const clearBtn = document.getElementById("clearBtn");
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
  messagesEl.scrollTop = messagesEl.scrollHeight;
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

function setBusy(b) {
  busy = b;
  sendBtn.disabled = b;
  inputEl.disabled = false;
}

// ---------- 后端事件 ----------

window.runtime.EventsOn("chat:chunk", (chunk) => {
  if (!currentBubble) return;
  answerText += chunk || "";
  currentBubble.innerHTML = escapeHtml(answerText).replace(/\n/g, "<br>");
  scrollToBottom();
});

window.runtime.EventsOn("chat:done", () => {
  if (currentBubble && !answerText) {
    currentBubble.textContent = "（没有收到回复内容，请再试一次）";
  }
  currentBubble = null;
  answerText = "";
  setBusy(false);
  inputEl.focus();
});

window.runtime.EventsOn("chat:error", (msg) => {
  if (currentBubble) {
    currentBubble.classList.add("error");
    currentBubble.textContent = "⚠️ " + (msg || "服务异常，请稍后再试");
  }
  currentBubble = null;
  answerText = "";
  setBusy(false);
  inputEl.focus();
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

  addMessage("user", text);
  inputEl.value = "";
  inputEl.style.height = "auto";
  setBusy(true);

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

// 清空对话：同时清掉后端会话上下文
clearBtn.addEventListener("click", async () => {
  await window.go.main.App.ResetSession(sessionId);
  messagesEl.innerHTML = "";
  addMessage(
    "assistant",
    "对话已清空，我们重新开始吧～\n可以问我：价格、密码、钥匙卡、战备配装等。"
  );
});

// 最小化 / 关闭：直接使用 Wails 注入的官方 window.runtime 窗口 API
minBtn.addEventListener("click", () => {
  window.runtime.WindowMinimise();
});
closeBtn.addEventListener("click", () => {
  window.runtime.Quit();
});

// 启动：确保会话ID就绪后聚焦输入框
ensureSession().then(() => inputEl.focus());
