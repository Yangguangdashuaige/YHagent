// electron/preload.js
// 渲染进程桥：唯一通道 petBridge（contextIsolation 隔离，零 node 能力暴露）。
// 约定与 Go 版 window.go.main.App 对齐，渲染层业务代码只换绑定点。
const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('pet', {
  // 配置（含 REMOTE_BASE_URL/REMOTE_TOKEN/PTT/ASR_AUTO_SEND）
  getConfig: () => ipcRenderer.invoke('pet:get-config'),
  // 命中盒上报（相对窗口内容区 CSS px；Electron 下 DIP 与 CSS px 1:1，无需 DPI 换算）
  setHitBox: (x, y, w, h) => ipcRenderer.send('pet:hitbox', { x, y, w, h }),
  // SAPI 听写
  listenStart: () => ipcRenderer.send('pet:listen-start'),
  listenStop: () => ipcRenderer.send('pet:listen-stop'),
  // 按键说话轮询开关（设置面板运行时切换）
  setPTT: (on) => ipcRenderer.send('pet:set-ptt', on),
  // 窗口与生命周期
  setAlwaysOnTop: (on) => ipcRenderer.send('pet:set-always-on-top', on),
  quit: () => ipcRenderer.send('pet:quit'),
  // 主进程 → 渲染进程事件
  onAsr: (cb) => ipcRenderer.on('pet:asr', (_e, evt) => cb(evt)),
  onPtt: (cb) => ipcRenderer.on('pet:ptt', (_e, down) => cb(down)),
});
