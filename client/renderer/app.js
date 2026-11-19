// renderer/app.js —— 桌宠前端逻辑（Electron 渲染进程）
//
// 通道两条：
//   1. window.pet.*（preload 桥）：命中盒/听写/PTT/置顶/退出 → 主进程
//   2. fetch 直连服务端：对话 SSE（/v1/chat）、会话管理（/v1/session*）
//
// UI 逻辑（动画状态机/气泡/拖拽戳摸/设置面板）继承自 Wails 版，
// 断线续传协议与服务端 WAL 对齐（trace 事件 trace_id 即续传凭据）。

const SESSION_KEY = "yh_session_id";
const SESSIONS_KEY = "yh_sessions";
const BUBBLE_FADE_MS = 12000;         // 气泡无更新后淡出
const SLEEP_AFTER_MS = 5 * 60 * 1000; // 无交互入睡

const petEl = document.getElementById("pet");
const videoA = document.getElementById("petA");
const videoB = document.getElementById("petB");
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

// 宠物尺寸随视口自适应（CSS --pet-h），运行期从元素实测
function petW() { return petEl.offsetWidth || 150; }
function petH() { return petEl.offsetHeight || 150; }

// ---------- 配置（主进程 .env） ----------

let cfg = { REMOTE_BASE_URL: "", REMOTE_TOKEN: "", ASR_AUTO_SEND: 1, PTT_ENABLED: 1 };
let cfgErr = "";
window.pet && window.pet.getConfig().then(c => {
  cfg = c;
  if (!c.REMOTE_BASE_URL || !c.REMOTE_TOKEN) {
    cfgErr = "REMOTE_BASE_URL / REMOTE_TOKEN 未配置，请在 exe 同目录 .env 填写后重启";
  }
  bootBubble();
});

// ---------- 动画（双 video 交替消抖） ----------

const ANIM = {
  idle: { loop: true }, walk: { loop: true }, happy: { loop: true },
  think: { loop: true }, work: { loop: true }, talk: { loop: true },
  drag: { loop: true }, sleep: { loop: true },
  celebrate: { loop: false }, startled: { loop: false },
  poke1: { loop: false }, poke2: { loop: false }, poke3: { loop: false },
};

let frontVideo = videoA;
let backVideo = videoB;
let currentAnim = null;

function setAnim(name) {
  if (currentAnim === name) return;
  currentAnim = name;
  const cfgA = ANIM[name];
  if (!cfgA) return;
  const v = backVideo;
  v.loop = cfgA.loop;
  v.src = "assets/pet/" + name + ".webm";
  v.onloadeddata = () => {
    v.classList.add("is-front");
    frontVideo.classList.remove("is-front");
    frontVideo.onended = null;
    [frontVideo, backVideo] = [v, frontVideo];
    if (!cfgA.loop) {
      v.onended = () => { v.onended = null; setState("idle"); };
    }
    reportHitBoxSoon();
  };
}

// ---------- 状态机 ----------

let state = "boot";
let walkTimer = null, sleepTimer = null;
let walkDir = 1;

function setState(next) {
  if (state === next) return;
  state = next;
  petEl.classList.toggle("flying", next === "drag");
  switch (next) {
    case "idle": setAnim("idle"); scheduleIdle(); break;
    case "walk": setAnim("walk"); break;
    case "think": setAnim("think"); break;
    case "talk": setAnim("talk"); break;
    case "work": setAnim("work"); break;
    case "celebrate": setAnim("celebrate"); break;
    case "startled": setAnim("startled"); break;
    case "drag": setAnim("drag"); break;
    case "sleep": setAnim("sleep"); break;
    case "poke": {
      const n = 1 + Math.floor(Math.random() * 3);
      setAnim("poke" + n); break;
    }
  }
}

// 悬停即停：鼠标放在宠物身上时暂停走动（方便点击；移开恢复）
let hoverPet = false;
petEl.addEventListener("pointerenter", () => { hoverPet = true; });
petEl.addEventListener("pointerleave", () => { hoverPet = false; });

// 行走 tick：不做随机漫游，只朝语音/文字指令指定的目标走，到达即停（驻守模式）
let targetX = null;
setInterval(() => {
  if (state !== "walk" || targetX == null || hoverPet) return;
  const d = targetX - petX();
  const step = 4;
  if (Math.abs(d) <= step) { targetX = null; setState("idle"); return; }
  walkDir = d > 0 ? 1 : -1;
  setPetX(petX() + walkDir * step);
  petEl.style.transform = walkDir < 0 ? "scaleX(-1)" : "";
}, 16);

// 语音/文字位置指令解析："去左边/最右边/回中间/往左一点/挪到右边"等，
// 命中即本地执行移动（不发给服务端消耗对话轮次）
function parseMoveCmd(t) {
  if (!t || !/(左|右|中)/.test(t) || !/(去|到|走|回|靠|挪|待|呆)/.test(t)) return null;
  const w = window.innerWidth, pw = petW();
  if (/最左|最左边/.test(t)) return 0;
  if (/最右|最右边/.test(t)) return w - pw;
  if (/中间|居中|回家|回来/.test(t)) return (w - pw) / 2;
  if (/往左|向左|左一点|左边去|去左/.test(t)) return Math.max(0, petX() - w * 0.25);
  if (/往右|向右|右一点|右边去|去右/.test(t)) return Math.min(w - pw, petX() + w * 0.25);
  return null;
}

// 指派宠物走向目标位置（到达自动停下驻守）
function movePetTo(x) {
  hideInput();
  wakeUp();
  targetX = Math.max(0, Math.min(window.innerWidth - petW(), x));
  if (Math.abs(targetX - petX()) < 4) {
    setState("idle");
    showBubble("轩就在这儿呀～", "");
    return;
  }
  setState("walk");
  showBubble("轩这就过去～", "");
}

function scheduleIdle() {
  clearTimeout(walkTimer);
  clearTimeout(sleepTimer);
  // 驻守模式：不再随机漫游（walkTimer 保留作未来扩展），仅长时间无交互入睡
  if (state !== "idle") return;
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
  // 气泡/输入面板跟随宠物（拖拽与走动时都不脱节）
  if (!bubbleEl.classList.contains("hidden")) positionBubble();
  if (!inputPanel.classList.contains("hidden")) positionInput();
}

// ---------- 命中盒上报（主进程穿透轮询的数据源；Electron 下 DIP 与 CSS px 1:1） ----------

let hitTimer = null;
function reportHitBoxSoon() {
  if (hitTimer) return;
  hitTimer = setTimeout(() => { hitTimer = null; reportHitBox(); }, 100);
}
function reportHitBox() {
  const inputOpen = !inputPanel.classList.contains("hidden");
  const settingsOpen = !settingsPanel.classList.contains("hidden");
  // 输入/设置面板打开期间整窗可交互——"点空白关闭面板"的体验前提
  if (inputOpen || settingsOpen) {
    window.pet && window.pet.setHitBox(0, 0, window.innerWidth, window.innerHeight);
    return;
  }
  const r = petEl.getBoundingClientRect();
  let x = r.left - 10, right = r.right + 10, y = r.top - 10, bottom = r.bottom + 10;
  if (!menuEl.classList.contains("hidden")) {
    const mr = menuEl.getBoundingClientRect();
    x = Math.min(x, mr.left); right = Math.max(right, mr.right);
    y = Math.min(y, mr.top); bottom = Math.max(bottom, mr.bottom);
  }
  window.pet && window.pet.setHitBox(x, y, right - x, bottom - y);
}

// 点面板外任意处 = 关闭面板（面板打开期间整窗可交互，事件能到达页面）。
// 打开后 600ms 为保护期：双击开面板的第二击若没点中移动中的宠物，不应把刚弹出的面板又关掉。
let panelOpenedAt = 0;
window.addEventListener("pointerdown", e => {
  const inputOpen = !inputPanel.classList.contains("hidden");
  const settingsOpen = !settingsPanel.classList.contains("hidden");
  if (!inputOpen && !settingsOpen) return;
  if (Date.now() - panelOpenedAt < 600) return;
  const t = e.target;
  if (inputPanel.contains(t) || settingsPanel.contains(t) ||
      petEl.contains(t) || menuEl.contains(t)) return;
  if (inputOpen) hideInput();
  else closeSettings();
});

// ---------- 气泡 ----------

let fadeTimer = null;
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
  let left = cx < window.innerWidth / 2 ? petX() + petW() * 0.4 : cx - bw - petW() * 0.4;
  left = Math.max(8, Math.min(window.innerWidth - bw - 8, left));
  bubbleEl.style.left = left + "px";
  const tail = bubbleEl.querySelector(".bubble-tail");
  const tx = Math.max(18, Math.min(bw - 32, cx - left - 7));
  tail.style.left = tx + "px";
}

// ---------- 轻量 Markdown ----------

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
      if (cells.every(c => /^[\s:-]*$/.test(c))) continue;
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

// ---------- 会话 API（fetch 直连服务端） ----------

function api(path, opts) {
  const o = opts || {};
  o.headers = Object.assign({ "Authorization": "Bearer " + cfg.REMOTE_TOKEN }, o.headers || {});
  if (o.body && !o.raw) {
    o.headers["Content-Type"] = "application/json";
    o.body = JSON.stringify(o.body);
  }
  return fetch(cfg.REMOTE_BASE_URL + path, o);
}

async function apiNewSession() {
  const r = await api("/v1/session/new", { method: "POST" });
  if (!r.ok) throw new Error("HTTP " + r.status);
  const env = await r.json();
  return env.session_id;
}
async function apiListSessions(sids) {
  if (!sids.length) return [];
  const r = await api("/v1/sessions?ids=" + encodeURIComponent(sids.join(",")));
  if (!r.ok) throw new Error("HTTP " + r.status);
  const env = await r.json();
  return env.sessions || [];
}
async function apiDeleteSession(sid) {
  const r = await api("/v1/session?session_id=" + encodeURIComponent(sid), { method: "DELETE" });
  if (!r.ok) throw new Error("HTTP " + r.status);
}
async function apiHistory(sid) {
  const r = await api("/v1/session/history?session_id=" + encodeURIComponent(sid));
  if (!r.ok) throw new Error("HTTP " + r.status);
  const env = await r.json();
  return env.messages || [];
}
function apiReset(sid) {
  api("/v1/session/reset", { method: "POST", body: { session_id: sid } }).catch(() => {});
}

// ---------- SSE 直连（fetch + ReadableStream 手动解析，断线凭 WAL 续传） ----------

const RESUME_MAX_RETRY = 5;
const RESUME_BACKOFF = [800, 1500, 3000, 5000, 8000];
const sleep = (ms) => new Promise(r => setTimeout(r, ms));

// 解析一段 SSE 流并回调事件；返回 {turnID, got} 供续传
async function streamOnce(sessionID, userText, resume, cb) {
  const payload = resume
    ? { session_id: sessionID, message: "", images: null, resume_turn: resume.turnID, resume_after: resume.got }
    : { session_id: sessionID, message: userText, images: null, resume_turn: "", resume_after: 0 };
  const resp = await fetch(cfg.REMOTE_BASE_URL + "/v1/chat", {
    method: "POST",
    headers: { "Content-Type": "application/json", "Authorization": "Bearer " + cfg.REMOTE_TOKEN, "Accept": "text/event-stream" },
    body: JSON.stringify(payload),
  });
  if (resp.status === 401) throw new Error("远端服务鉴权失败，请检查 REMOTE_TOKEN");
  if (!resp.ok) {
    const bodyText = await resp.text().catch(() => "");
    try { throw new Error(JSON.parse(bodyText).error || "HTTP " + resp.status); }
    catch (e) { throw e instanceof SyntaxError ? new Error("HTTP " + resp.status) : e; }
  }

  const st = { turnID: resume ? resume.turnID : "", got: resume ? resume.got : 0 };
  const reader = resp.body.getReader();
  const decoder = new TextDecoder();
  let buf = "";
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    buf += decoder.decode(value, { stream: true });
    let idx;
    while ((idx = buf.indexOf("\n\n")) >= 0) {
      const block = buf.slice(0, idx);
      buf = buf.slice(idx + 2);
      // 解析事件块：event: X\ndata: Y（: 开头的行是心跳，忽略）
      let event = "", data = "";
      for (const line of block.split("\n")) {
        if (line.startsWith(":")) continue;
        if (line.startsWith("event:")) event = line.slice(6).trim();
        else if (line.startsWith("data:")) data += line.slice(5).trim();
      }
      if (!event) continue;
      let parsed = null;
      if (data) { try { parsed = JSON.parse(data); } catch { parsed = data; } }
      switch (event) {
        case "chunk":
          st.got++;
          cb.onChunk(typeof parsed === "string" ? parsed : data);
          break;
        case "thought":
          if (parsed && parsed.text) st.got++;
          cb.onThought && cb.onThought(parsed && parsed.text);
          break;
        case "trace":
          if (parsed && parsed.trace_id) st.turnID = parsed.trace_id;
          break;
        case "clarify":
          cb.onClarify && cb.onClarify(parsed || {});
          break;
        case "interrupted":
          cb.onInterrupted && cb.onInterrupted(parsed || {});
          return st; // 中断不重试，上层渲染「重新生成」
        case "error":
          throw new Error(typeof parsed === "string" ? parsed : (parsed && parsed.error) || "服务端错误");
        case "done":
          cb.onDone && cb.onDone();
          return st;
        case "queue":
        case "candidate":
        case "tool":
        case "round":
        case "plan":
          break; // 桌宠气泡不呈现执行过程细节，忽略
      }
    }
  }
  throw new Error("连接中断");
}

// 带断线续传的对话（trace.turnID 为凭据，chunk+thought 计数续传对齐）
async function chatWithSession(sessionID, userText, cb) {
  let attempt = 0, turnID = "", got = 0;
  for (;;) {
    let st;
    try {
      st = await streamOnce(sessionID, userText, attempt > 0 ? { turnID, got } : null, cb);
    } catch (e) {
      if (e.message === "连接中断" && turnID && attempt < RESUME_MAX_RETRY) {
        attempt++;
        await sleep(RESUME_BACKOFF[Math.min(attempt - 1, RESUME_BACKOFF.length - 1)]);
        continue;
      }
      throw e;
    }
    turnID = st.turnID; got = st.got;
    return;
  }
}

// ---------- 会话与对话（UI 层） ----------

let sessionId = localStorage.getItem(SESSION_KEY);
let busy = false;
let answerText = "";

function savedSids() {
  try { return JSON.parse(localStorage.getItem(SESSIONS_KEY) || "[]"); } catch { return []; }
}
function rememberSid(sid) {
  if (!sid) return;
  const list = savedSids().filter(s => s !== sid);
  list.unshift(sid);
  localStorage.setItem(SESSIONS_KEY, JSON.stringify(list.slice(0, 30)));
}

function ensureSession() {
  if (sessionId) { rememberSid(sessionId); return; }
  apiNewSession().then(id => {
    if (id) {
      sessionId = id;
      localStorage.setItem(SESSION_KEY, id);
      rememberSid(id);
    }
  }).catch(() => {});
}

function send(text) {
  text = (text || "").trim();
  if (!text || busy) { if (busy) showBubble("轩还在回答上一个问题哦…", "thinking"); return; }
  if (cfgErr) { setState("startled"); showBubble('<span class="err">' + escapeHtml(cfgErr) + "</span>", "err", true); return; }
  // 位置指令本地消化（语音/文字"去左边/回中间"等），不进服务端对话
  const dest = parseMoveCmd(text);
  if (dest != null) { movePetTo(dest); return; }
  hideInput();
  busy = true; answerText = "";
  setState("think");
  showBubble('<span class="thinking">让我想想…</span>', "thinking");

  chatWithSession(sessionId || "default", text, {
    onChunk: (c) => {
      if (state === "think") setState("talk");
      answerText += c;
      showBubble(mdRender(answerText), "");
    },
    onClarify: (p) => {
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
    },
    onInterrupted: (p) => {
      busy = false;
      setState("idle");
      showBubble("（回答被中断，已生成内容如上；重新问一次就好）", "thinking");
    },
    onDone: () => {
      busy = false;
      setState("celebrate");
      setTimeout(() => { if (state === "celebrate") setState("idle"); scheduleIdle(); }, 2200);
      showBubble(mdRender(answerText), "");
    },
  }).catch(err => {
    busy = false;
    setState("startled");
    showBubble('<span class="err">' + escapeHtml(String(err && err.message || err)) + "</span>", "err", true);
  });
}

// ---------- PTT 听写编排（主进程 SAPI 子进程 + koffi 热键） ----------

let held = false;
let utter = "";

window.pet && window.pet.onPtt(down => { down ? beginListening() : endListening(); });
window.pet && window.pet.onAsr(evt => {
  switch (evt.type) {
    case "partial":
      showBubble('<span class="listening">🎤 ' + escapeHtml(evt.text || "") + "</span>", "listening");
      break;
    case "final":
      if (utter) utter += "，";
      utter += evt.text || "";
      showBubble('<span class="listening">🎤 ' + escapeHtml(utter) + "</span>", "listening");
      break;
    case "done":
      // 一句说完仍按住 → 300ms 后自动续听下一句（多句 PTT 语义）
      if (held && (evt.reason === "completed" || evt.reason === "timeout" || evt.reason === "rejected")) {
        setTimeout(() => { if (held) window.pet && window.pet.listenStart(); }, 300);
      }
      break;
  }
});

function beginListening() {
  if (held) return;
  held = true;
  utter = "";
  micDot.classList.remove("hidden");
  if (!busy) setState("work");
  showBubble('<span class="listening">🎤 在听，请讲…</span>', "listening");
  window.pet && window.pet.listenStart();
}

function endListening() {
  if (!held) return;
  held = false;
  window.pet && window.pet.listenStop();
  micDot.classList.add("hidden");
  const text = utter.trim();
  if (!text) { showBubble("（没听清，再试一次？）", "thinking"); return; }
  if (cfg.ASR_AUTO_SEND) { send(text); return; }
  showInput(text);
}

// ---------- 输入面板 ----------

function showInput(text) {
  settingsPanel.classList.add("hidden");
  inputPanel.classList.remove("hidden");
  panelOpenedAt = Date.now();
  if (state === "walk") setState("idle");
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
  if (inputEl.composing) return;
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

// Esc 全局兜底：关闭最上层的面板
window.addEventListener("keydown", e => {
  if (e.key === "Escape") {
    if (!menuEl.classList.contains("hidden")) menuEl.classList.add("hidden");
    else if (!settingsPanel.classList.contains("hidden")) closeSettings();
    else if (!inputPanel.classList.contains("hidden")) hideInput();
  }
});

// ---------- 设置面板 ----------

const tabHistoryBtn = document.getElementById("tabHistory");
const tabSettingsBtn = document.getElementById("tabSettings");
const pageHistoryEl = document.getElementById("pageHistory");
const pageSettingsEl = document.getElementById("pageSettings");

function openSettings() {
  menuEl.classList.add("hidden");
  hideInput();
  settingsPanel.classList.remove("hidden");
  panelOpenedAt = Date.now();
  renderSessions();
  initOptionToggles();
  reportHitBox();
}
function closeSettings() {
  settingsPanel.classList.add("hidden");
  historyViewEl.classList.add("hidden");
  sessionListEl.style.display = "";
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

function fmtTime(ts) {
  if (!ts) return "";
  const d = new Date(ts * 1000);
  const now = Date.now() / 1000;
  if (now - ts < 86400) return d.getHours() + ":" + String(d.getMinutes()).padStart(2, "0");
  return (d.getMonth() + 1) + "/" + d.getDate();
}

function renderSessions() {
  const sids = savedSids();
  if (!sids.length) {
    sessionListEl.innerHTML = '<li><span class="s-title" style="color:#8a94a6">还没有对话记录～</span></li>';
    return;
  }
  apiListSessions(sids).then(list => {
    if (!list || !list.length) {
      sessionListEl.innerHTML = '<li><span class="s-title" style="color:#8a94a6">（服务端没有这些会话的记录，可能已过期清理）</span></li>';
      return;
    }
    list.sort((a, b) => (b.updated_at || 0) - (a.updated_at || 0));
    sessionListEl.innerHTML = "";
    for (const s of list) {
      const li = document.createElement("li");
      if (s.session_id === sessionId) li.classList.add("current");
      li.innerHTML = '<span class="s-title">' + escapeHtml(s.title || "（未命名对话）") + "</span>" +
        '<span class="s-meta">' + fmtTime(s.updated_at) + " · " + s.msg_count + '条</span>' +
        '<button class="s-del" title="切换到此对话" style="color:#7ec2ff;font-size:12px">切换</button>' +
        '<button class="s-del" title="删除此对话">删</button>';
      li.addEventListener("click", () => viewHistory(s.session_id, s.title));
      li.querySelectorAll("button")[0].addEventListener("click", e => { e.stopPropagation(); switchSession(s.session_id); });
      li.querySelectorAll("button")[1].addEventListener("click", e => {
        e.stopPropagation();
        apiDeleteSession(s.session_id).catch(() => {});
        localStorage.setItem(SESSIONS_KEY, JSON.stringify(savedSids().filter(x => x !== s.session_id)));
        if (s.session_id === sessionId) switchToNewSession();
        renderSessions();
      });
      sessionListEl.appendChild(li);
    }
  }).catch(() => {
    sessionListEl.innerHTML = '<li><span class="s-title" style="color:#8a94a6">（会话列表获取失败）</span></li>';
  });
}

function viewHistory(sid, title) {
  apiHistory(sid).then(msgs => {
    historyViewEl.classList.remove("hidden");
    sessionListEl.style.display = "none";
    let html = '<button class="hback">← 返回列表</button>';
    if (!msgs || !msgs.length) {
      html += '<div class="hmsg" style="color:#8a94a6">（没有可回看的消息）</div>';
    } else {
      html += '<div style="color:#8a94a6;font-size:11px;margin-bottom:4px">' + escapeHtml(title || "") + " 共 " + msgs.length + " 条</div>";
      for (const m of msgs) {
        html += '<div class="hmsg ' + (m.role === "user" ? "user" : "assistant") + '">' +
                (m.role === "user" ? "问：" : "答：") + escapeHtml(m.content) + "</div>";
      }
    }
    historyViewEl.innerHTML = html;
    historyViewEl.querySelector(".hback").addEventListener("click", () => {
      historyViewEl.classList.add("hidden");
      sessionListEl.style.display = "";
    });
  }).catch(() => {
    showBubble('<span class="err">会话历史获取失败</span>', "err", true);
  });
}

function switchSession(sid) {
  if (busy) { showBubble("轩正在回答中，稍等再切换～", "thinking"); return; }
  sessionId = sid;
  localStorage.setItem(SESSION_KEY, sessionId);
  showBubble("已切换到该对话～", "");
  renderSessions();
}

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

// 设置页开关：置顶 / PTT / 自动发送
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
    window.pet && window.pet.setAlwaysOnTop(pinTop);
  });
  optPTTEl.checked = !!cfg.PTT_ENABLED;
  optPTTEl.addEventListener("change", () => window.pet && window.pet.setPTT(optPTTEl.checked));
  optAutoSendEl.checked = !!cfg.ASR_AUTO_SEND;
  optAutoSendEl.addEventListener("change", () => { cfg.ASR_AUTO_SEND = optAutoSendEl.checked ? 1 : 0; });
}

// ---------- 拖拽 / 戳摸 ----------

let dragging = false, dragMoved = false, downAt = 0, downX = 0, downY = 0, grabDX = 0, grabDY = 0;

petEl.addEventListener("pointerdown", e => {
  if (e.button === 2) return;
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
    if (Math.hypot(e.clientX - downX, e.clientY - downY) < 5) return;
    dragMoved = true;
    setState("drag");
  }
  setPetX(e.clientX - grabDX);
  const y = Math.max(0, Math.min(window.innerHeight - petH(), e.clientY - grabDY));
  petEl.style.bottom = (window.innerHeight - petH() - y) + "px";
});
petEl.addEventListener("pointerup", e => {
  if (!dragging) return;
  dragging = false;
  if (state === "drag") {
    petEl.classList.add("dropping");
    petEl.style.bottom = "0px";
    targetX = null; // 手动放下后取消未完成的行走指令，就地驻守
    setState("happy");
    setTimeout(() => {
      petEl.classList.remove("dropping");
      setState("idle");
    }, 480);
    return;
  }
  if (Date.now() - downAt < 350 && !dragMoved) poke();
});

function poke() {
  if (!inputPanel.classList.contains("hidden")) { hideInput(); return; }
  if (!settingsPanel.classList.contains("hidden")) { closeSettings(); return; }
  if (busy) { showBubble("轩正在回答中…", "thinking"); return; }
  setState("poke");
  const msgs = ["轩？", "干嘛戳我～", "想问什么就戳出输入框吧", "轩在呢！", "轻点轻点…"];
  showBubble(msgs[Math.floor(Math.random() * msgs.length)], "");
  setTimeout(() => { if (state === "poke") setState("idle"); }, 1600);
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
  reportHitBox();
});
document.addEventListener("pointerdown", e => {
  if (!menuEl.classList.contains("hidden") && !menuEl.contains(e.target)) {
    menuEl.classList.add("hidden");
    reportHitBox();
  }
});
menuEl.addEventListener("click", e => {
  const act = e.target.dataset && e.target.dataset.act;
  menuEl.classList.add("hidden");
  reportHitBox();
  if (act === "input") showInput();
  else if (act === "new") switchToNewSession();
  else if (act === "settings") openSettings();
  else if (act === "quit") window.pet && window.pet.quit();
});

let pinTop = localStorage.getItem("yh_pin_top") !== "0";

// ---------- 启动 ----------

window.addEventListener("resize", () => {
  setPetX(petX());
  positionBubble();
  positionInput();
  reportHitBox();
});

function bootBubble() {
  if (cfgErr) {
    setState("startled");
    showBubble('<span class="err">' + escapeHtml(cfgErr) + "</span>", "err", true);
    return;
  }
  showBubble("指挥官好，我是小小轩～<br>戳两下我提问，<b>按住左 Alt 说话</b>；说「去左边 / 去右边 / 回中间」指挥我挪窝；右键有菜单。", "");
}

setPetX(Math.max(0, window.innerWidth / 2 - petW() / 2));
setState("idle");
ensureSession();
if (!pinTop) window.pet && window.pet.setAlwaysOnTop(false);
bootBubble(); // 配置未到时先出欢迎语；getConfig 返回后再覆盖
