// electron/main.js
//
// YHagent-Pet 桌宠客户端主进程（Electron）。
//
// 窗口机制移植自 dsh-pet 的 electron-helper（MIT，行为复刻非整文件搬运）：
//   - 透明无边框置顶窗（transparent/frame:false/alwaysOnTop/skipTaskbar/hasShadow:false）
//   - 60ms 光标轮询 vs 宠物命中盒 → setIgnoreMouseEvents(forward) 全窗穿透除命中区
//   - 置顶看门狗 + 后台渲染节流关闭 + 右键默认菜单屏蔽
// 几何采用 Coopanion 带状窗规范（主屏工作区整宽 × 固定高，离底边固定间隙）。
//
// 对话不经过主进程：渲染进程 fetch 直连服务端 /v1/chat SSE。
// 主进程只负责窗口/几何/穿透/热键（koffi GetAsyncKeyState）/SAPI 听写子进程。
const { app, BrowserWindow, screen, ipcMain } = require('electron');
const path = require('path');
const fs = require('fs');
const { spawn } = require('child_process');

// ---------- 配置（.env：exe 同目录优先，其次工作目录；无 BOM 编码） ----------

const CFG_DEFAULTS = {
  REMOTE_BASE_URL: '',
  REMOTE_TOKEN: '',
  PET_BAND_HEIGHT: 260, // 带状窗高（DIP；Electron 全程逻辑像素）
  PET_BOTTOM_GAP: 3,    // 窗口底边离工作区底边间隙（DIP）
  PTT_ENABLED: 1,        // 按住说话（默认左 Alt）
  PTT_KEY_VK: 164,       // PTT 虚拟键码（164=左 Alt）
  ASR_AUTO_SEND: 1,      // 听写结束自动发送
};

function parseEnvFile(text) {
  const out = {};
  for (const raw of text.split(/\r?\n/)) {
    const line = raw.trim();
    if (!line || line.startsWith('#')) continue;
    const eq = line.indexOf('=');
    if (eq <= 0) continue;
    const key = line.slice(0, eq).trim();
    let val = line.slice(eq + 1).trim();
    if ((val.startsWith('"') && val.endsWith('"')) || (val.startsWith("'") && val.endsWith("'"))) {
      val = val.slice(1, -1);
    }
    out[key] = val;
  }
  return out;
}

function loadConfig() {
  const cfg = { ...CFG_DEFAULTS };
  const paths = [
    path.join(path.dirname(process.execPath), '.env'), // 打包后 exe 同目录
    path.join(process.cwd(), '.env'),                  // 开发态 client/
  ];
  for (const p of paths) {
    try {
      if (fs.existsSync(p)) Object.assign(cfg, parseEnvFile(fs.readFileSync(p, 'utf8')));
    } catch { /* 可观测性失败不阻断启动 */ }
  }
  for (const k of ['PET_BAND_HEIGHT', 'PET_BOTTOM_GAP', 'PTT_KEY_VK']) {
    cfg[k] = Number.parseInt(cfg[k], 10) || CFG_DEFAULTS[k];
  }
  for (const k of ['PTT_ENABLED', 'ASR_AUTO_SEND']) {
    cfg[k] = cfg[k] === '1' || String(cfg[k]).toLowerCase() === 'true' ? 1 : 0;
  }
  if (cfg.PET_BAND_HEIGHT < 120) cfg.PET_BAND_HEIGHT = 120;
  if (cfg.PET_BOTTOM_GAP < 0) cfg.PET_BOTTOM_GAP = 0;
  return cfg;
}

const cfg = loadConfig();

// ---------- 带状窗几何（Coopanion 规范：地面=工作区底边-间隙） ----------

function bandBounds() {
  const wa = screen.getPrimaryDisplay().workArea; // DIP
  return {
    x: wa.x,
    y: wa.y + wa.height - cfg.PET_BOTTOM_GAP - cfg.PET_BAND_HEIGHT,
    width: wa.width,
    height: cfg.PET_BAND_HEIGHT,
  };
}

// ---------- 窗口 ----------

let win = null;

function createWindow() {
  win = new BrowserWindow({
    ...bandBounds(),
    show: false,
    useContentSize: true,
    enableLargerThanScreen: true,
    transparent: true,
    frame: false,
    alwaysOnTop: true,
    skipTaskbar: true,
    resizable: false,
    hasShadow: false,
    // 可聚焦：输入框/菜单需要焦点；show 用 showInactive 不抢其它应用焦点
    focusable: true,
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false,
      spellcheck: false,
    },
  });
  win.setAlwaysOnTop(true, 'screen-saver');
  win.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true });
  // 被遮挡的窗口默认暂停渲染（表现成"消失"）；关闭后台节流保证持续渲染
  win.webContents.setBackgroundThrottling(false);
  // 屏蔽 Electron 默认右键菜单：右键菜单由渲染进程自绘
  win.webContents.on('context-menu', (event) => event.preventDefault());
  win.loadFile(path.join(__dirname, '..', 'renderer', 'index.html'));
  win.once('ready-to-show', () => win.showInactive());
  win.on('closed', () => { win = null; });

  // 置顶看门狗：被全屏游戏压下去时自动顶回（dsh-pet 同款兜底）
  const watchdog = setInterval(() => { if (win) win.moveTop(); }, 3000);
  watchdog.unref?.();
  // 分辨率/任务栏变化 → 重放带状几何
  const relayout = () => { if (win) win.setBounds(bandBounds()); };
  screen.on('display-metrics-changed', relayout);
  screen.on('display-removed', relayout);
}

// ---------- 穿透轮询（dsh-pet 机制：60ms 光标 vs 渲染进程上报的命中盒） ----------

let hitBox = null;    // { x, y, w, h } 相对窗口内容区（DIP=CSS px 1:1）
let ignoring = false;

function pointerPump() {
  const t = setInterval(() => {
    if (!win) return;
    let hit = false;
    if (hitBox && hitBox.w > 0 && hitBox.h > 0) {
      const b = win.getContentBounds();
      const pt = screen.getCursorScreenPoint();
      hit = pt.x >= b.x + hitBox.x && pt.x < b.x + hitBox.x + hitBox.w &&
            pt.y >= b.y + hitBox.y && pt.y < b.y + hitBox.y + hitBox.h;
    }
    const shouldIgnore = !hit;
    if (shouldIgnore !== ignoring) {
      // forward:true 让穿透期间 mousemove 仍转发给页面（悬停即停依赖它）
      win.setIgnoreMouseEvents(shouldIgnore, { forward: true });
      ignoring = shouldIgnore;
    }
  }, 60);
  t.unref?.();
}

// ---------- SAPI 听写子进程（PowerShell System.Speech，协议同 sapi.ps1） ----------

let sapiProc = null;

function emitAsr(evt) { if (win) win.webContents.send('pet:asr', evt); }

function sapiStart() {
  if (sapiProc) return;
  const script = path.join(__dirname, 'sapi.ps1');
  const proc = spawn('powershell.exe', ['-NoProfile', '-STA', '-ExecutionPolicy', 'Bypass', '-File', script], {
    windowsHide: true, // 不弹蓝色 PowerShell 窗
    stdio: ['ignore', 'pipe', 'ignore'],
  });
  sapiProc = proc;
  let buf = '';
  proc.stdout.setEncoding('utf8');
  proc.stdout.on('data', (chunk) => {
    buf += chunk;
    let idx;
    while ((idx = buf.indexOf('\n')) >= 0) {
      const line = buf.slice(0, idx).trim();
      buf = buf.slice(idx + 1);
      if (!line) continue;
      try { emitAsr(JSON.parse(line)); } catch { /* 非 JSON 行忽略 */ }
    }
  });
  proc.on('exit', () => { if (sapiProc === proc) sapiProc = null; });
}

function sapiStop() {
  if (!sapiProc) return;
  const proc = sapiProc;
  sapiProc = null;
  // 说完一句引擎自己退出；仍在说话时给 600ms 自然收尾再强杀
  const killTimer = setTimeout(() => { try { proc.kill(); } catch {} }, 600);
  killTimer.unref?.();
  proc.once('exit', () => clearTimeout(killTimer));
}

// ---------- PTT 热键轮询（koffi 调 GetAsyncKeyState；不可用时静默降级） ----------

let pttTimer = null;
let pttPolling = false;

function startPttPolling() {
  if (pttPolling) return true;
  let GetAsyncKeyState;
  try {
    const koffi = require('koffi');
    const user32 = koffi.load('user32.dll');
    GetAsyncKeyState = user32.func('short __stdcall GetAsyncKeyState(int vKey)');
  } catch (e) {
    console.warn('[pet] koffi 不可用，按键说话已禁用：', e.message);
    return false;
  }
  let prev = (GetAsyncKeyState(cfg.PTT_KEY_VK) & 0x8000) !== 0; // 基线，防启动瞬间的按下沿
  pttTimer = setInterval(() => {
    const down = (GetAsyncKeyState(cfg.PTT_KEY_VK) & 0x8000) !== 0;
    if (down !== prev) {
      prev = down;
      if (win) win.webContents.send('pet:ptt', down);
    }
  }, 30);
  pttTimer.unref?.();
  pttPolling = true;
  return true;
}

function stopPttPolling() {
  if (pttTimer) clearInterval(pttTimer);
  pttTimer = null;
  pttPolling = false;
}

// ---------- IPC 面（渲染进程经 preload 调用） ----------

ipcMain.handle('pet:get-config', () => cfg);
ipcMain.on('pet:hitbox', (_e, box) => { hitBox = box; });
ipcMain.on('pet:listen-start', () => sapiStart());
ipcMain.on('pet:listen-stop', () => sapiStop());
ipcMain.on('pet:set-ptt', (_e, on) => { if (on) startPttPolling(); else stopPttPolling(); });
ipcMain.on('pet:set-always-on-top', (_e, on) => { if (win) win.setAlwaysOnTop(!!on, 'screen-saver'); });
ipcMain.on('pet:quit', () => app.quit());

// ---------- 启动 ----------

app.whenReady().then(() => {
  createWindow();
  pointerPump();
  if (cfg.PTT_ENABLED) startPttPolling();
});

app.on('window-all-closed', () => app.quit());
