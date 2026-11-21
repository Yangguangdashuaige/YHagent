import { createServer } from 'node:http';
import { once } from 'node:events';
import { JSDOM } from 'jsdom';
import { expect, it, vi } from 'vitest';
import { LiveClient } from 'cortico/worlds/bilibili/client.ts';
import { BilibiliWorld } from 'cortico/worlds/bilibili/world.ts';
import { ConsolePageLoader } from '../console/advanced/console-pages/loader.ts';
import { createConsoleUi } from '../console/base/ui/index.ts';

async function mount(invoke, start) {
  const dom = new JSDOM();
  const { document, AbortController } = dom.window;
  const root = document.createElement('div');
  const life = new AbortController();
  const importModule = vi.fn();
  const loader = new ConsolePageLoader({ importModule, styleHost: document.head, createLink: () => document.createElement('link') });
  const panel = await loader.resolvePanel('world:bilibili', 'login');
  expect(importModule).not.toHaveBeenCalled();
  const memo = { get: (_key, fallback) => fallback, set() {} };
  const ui = createConsoleUi({ memo, signal: life.signal, overlayHost: document.body });
  let poll;
  await panel.mount({ ui, root, language: 'zh', signal: life.signal, invoke, interval(fn) { poll = fn; } });
  await vi.waitFor(() => expect(root.querySelector('button').textContent).toBe(start));
  return { root, poll, window: dom.window, dispose() { life.abort(); dom.window.close(); } };
}

it('renders the Bilibili QR, reports phone approval, saves the credential, and reconnects the room', async () => {
  const pageUrl = 'https://account.bilibili.com/h5/account-h5/auth/scan-web?qrcode_key=fixture-key';
  const cookie = 'mock%2Ctime%2Csignature*21';
  let passportCode = 86090;
  const server = createServer((req, res) => {
    const url = new URL(req.url, 'http://127.0.0.1');
    if (url.pathname.endsWith('/qrcode/generate')) {
      res.end(JSON.stringify({ code: 0, data: { url: pageUrl, qrcode_key: 'fixture-key' } }));
    } else if (url.pathname.endsWith('/qrcode/poll') && url.searchParams.get('qrcode_key') === 'fixture-key') {
      if (passportCode === 0) res.setHeader('Set-Cookie', [`SESSDATA=${cookie}; Path=/`, 'DedeUserID=42; Path=/']);
      res.end(JSON.stringify({ code: 0, data: { code: passportCode } }));
    } else {
      res.writeHead(404).end();
    }
  });
  server.listen(0, '127.0.0.1');
  await once(server, 'listening');
  const saved = [];
  const connected = [];
  const connect = vi.spyOn(LiveClient.prototype, 'start').mockImplementation(async function () {
    connected.push(this.opts.sessdata);
    this.opts.onStatus({ ...this.current(), phase: 'connected', selfUid: this.opts.sessdata ? 42 : 0 });
  });
  const disconnect = vi.spyOn(LiveClient.prototype, 'stop').mockResolvedValue();
  const mockLog = { info() {}, warn() {}, debug() {}, error() {}, child: () => mockLog };
  const world = new BilibiliWorld({
    roomId: 6, passportBase: `http://127.0.0.1:${server.address().port}`, onSessdata: (value) => saved.push(value),
  });
  let view;
  try {
    await world.start({ log: mockLog, pushCandidate: async () => [] });
    const rpc = world.webUi().invoke;
    view = await mount((method) => rpc('login', method, []), '生成登录二维码');
    const open = vi.spyOn(view.window, 'open').mockImplementation(() => null);
    vi.stubGlobal('window', view.window);
    const { root, poll } = view;
    root.querySelector('button').click();
    const image = root.querySelector('img');
    await vi.waitFor(() => expect(image.src).toMatch(/^data:image\/gif;base64,/));
    expect(root.querySelector('a').href).toBe(pageUrl);
    expect(open).not.toHaveBeenCalled();
    await vi.waitFor(async () => {
      await poll();
      expect(root.querySelector('[role=status]').textContent).toContain('已扫码');
    }, { timeout: 5000, interval: 40 });
    passportCode = 0;
    await vi.waitFor(async () => {
      await poll();
      expect(root.querySelector('[role=status]').textContent).toContain('登录成功（UID 42）');
      expect(root.textContent).toContain('已用 UID 42 接入');
    }, { timeout: 5000, interval: 40 });
    expect(image.hidden).toBe(true);
    expect(image.hasAttribute('src')).toBe(false);
    expect(root.querySelector('a').hasAttribute('href')).toBe(false);
    expect(saved).toEqual([cookie]);
    expect(connected).toEqual(['', cookie]);
    expect(disconnect).toHaveBeenCalledTimes(1);
  } finally {
    view?.dispose();
    await world.stop();
    vi.unstubAllGlobals();
    connect.mockRestore();
    disconnect.mockRestore();
    const closed = once(server, 'close');
    server.closeAllConnections();
    server.close();
    await closed;
  }
}, 15000);

it('keeps a confirmation link outside the Bilibili sign-in service out of the panel', async () => {
  const idle = { phase: 'idle', url: null, uid: null, error: null, configured: false, selfUid: 0 };
  const link = 'https://account.bilibili.com.example.test/h5/account-h5/auth/scan-web?qrcode_key=fixture-key';
  const view = await mount(async (method) => method === 'start' ? { ...idle, phase: 'waiting', url: link } : idle, '生成登录二维码');
  try {
    view.root.querySelector('button').click();
    await vi.waitFor(() => expect(view.root.querySelector('[role=status]').textContent).toContain('二维码地址无效'));
    expect(view.root.querySelector('img').hasAttribute('src')).toBe(false);
    expect(view.root.querySelector('a').hasAttribute('href')).toBe(false);
  } finally {
    view.dispose();
  }
});
