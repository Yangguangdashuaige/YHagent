import qrcode from 'qrcode-generator';
import type { WebUiPanel } from '../base/protocol.ts';

interface LoginState {
  phase: 'idle' | 'waiting' | 'scanned' | 'done' | 'expired' | 'failed';
  url: string | null;
  uid: number | null;
  error: string | null;
  configured: boolean;
  selfUid: number;
}

const zh = {
  title: '登录 B 站',
  description: '使用手机上的哔哩哔哩 App 扫码，并在手机上确认登录。',
  start: '生成登录二维码',
  restart: '重新扫码登录',
  cancel: '取消登录',
  openPage: '在已登录的浏览器中打开确认页 ↗',
  qrLabel: '哔哩哔哩登录二维码',
  credential: '登录凭证',
  saved: '已保存在本机',
  missing: '未登录',
  identity: '直播间身份',
  signedIn: (uid: number) => `已用 UID ${uid} 接入`,
  anonymous: '尚未以登录身份接入',
  waiting: '等待手机 App 扫码',
  scanned: '已扫码，请在手机上确认',
  expired: '二维码已过期，请重新生成',
  done: (uid: number | null) => `登录成功${uid ? `（UID ${uid}）` : ''}，凭证已保存。已设置房间时会自动尝试重连。`,
  failed: (error: string) => `B 站登录失败。${error}`,
  stateFailed: (error: string) => `暂时无法读取登录状态。${error}`,
  invalidPage: 'B 站返回的二维码地址无效。请取消后重试。',
};

const en: typeof zh = {
  title: 'Sign in to Bilibili',
  description: 'Scan with the Bilibili mobile app and confirm the sign-in on your phone.',
  start: 'Generate sign-in QR code',
  restart: 'Sign in again',
  cancel: 'Cancel sign-in',
  openPage: 'Open confirmation page in a signed-in browser ↗',
  qrLabel: 'Bilibili sign-in QR code',
  credential: 'Credential',
  saved: 'Saved on this device',
  missing: 'Signed out',
  identity: 'Live room identity',
  signedIn: (uid) => `Connected as UID ${uid}`,
  anonymous: 'Not connected with an account yet',
  waiting: 'Waiting for the mobile app to scan',
  scanned: 'Scanned. Confirm the sign-in on your phone.',
  expired: 'The QR code expired. Generate a new one.',
  done: (uid) => `Signed in${uid ? ` (UID ${uid})` : ''}. The credential is saved. A configured live room will reconnect automatically.`,
  failed: (error) => `Bilibili sign-in failed. ${error}`,
  stateFailed: (error) => `Could not read the sign-in status. ${error}`,
  invalidPage: 'Bilibili returned an invalid QR code URL. Cancel sign-in and try again.',
};

function confirmationLink(value: string | null): string | null {
  if (!value) return null;
  try {
    const url = new URL(value);
    const path = url.pathname.replace(/\/$/, '');
    const link = `${url.origin}${path}`;
    if ((link === 'https://account.bilibili.com/h5/account-h5/auth/scan-web'
        || link === 'https://passport.bilibili.com/h5/scan-web')
      && !url.username && !url.password && !url.hash && url.searchParams.get('qrcode_key')) return url.href;
  } catch { /* A malformed response should never become a link or QR code. */ }
  return null;
}

const errorMessage = (error: unknown) => error instanceof Error ? error.message : String(error);

export const bilibiliLoginPanel: WebUiPanel = {
  mount(ctx) {
    const { ui, root, signal } = ctx;
    const t = ctx.language.startsWith('zh') ? zh : en;
    const card = ui.sheet({ title: t.title, desc: t.description });
    root.append(card.el);

    const error = ui.msgline();
    error.className = 'msgline bad';
    error.hidden = true;
    const identity = ui.h('div');
    const progress = ui.h('p');
    progress.setAttribute('role', 'status');
    progress.hidden = true;

    const image = ui.h('img');
    image.alt = t.qrLabel;
    image.className = 'bilibili-login-qr';
    image.hidden = true;
    const start = ui.button(t.start, { variant: 'primary', onClick: () => void change('start') });
    const cancel = ui.button(t.cancel, { onClick: () => void change('cancel') });
    const browser = ui.h('a', 'btn', t.openPage);
    browser.target = '_blank';
    browser.rel = 'noopener noreferrer';
    browser.hidden = cancel.hidden = true;
    const actions = ui.actions();
    actions.append(start, cancel, browser);
    card.body.append(error, identity, progress, image, actions);

    let showing: string | null = null;
    let changing = false;
    let checking = false;
    let statusProblem = false;
    let revision = 0;

    function draw(state: LoginState): void {
      const pending = state.phase === 'waiting' || state.phase === 'scanned';
      const link = pending ? confirmationLink(state.url) : null;
      if (link !== showing) {
        if (link) {
          const qr = qrcode(0, 'M');
          qr.addData(link);
          qr.make();
          image.width = image.height = (qr.getModuleCount() + 8) * 4;
          image.src = qr.createDataURL(4, 16);
          browser.href = link;
        } else {
          image.removeAttribute('src');
          browser.removeAttribute('href');
        }
        showing = link;
      }
      image.hidden = browser.hidden = !link;
      start.hidden = pending;
      start.textContent = state.configured ? t.restart : t.start;
      cancel.hidden = !pending;
      identity.replaceChildren(ui.kv([
        { k: t.credential, v: ui.pill(state.configured ? t.saved : t.missing, state.configured ? 'on' : 'off') },
        { k: t.identity, v: ui.pill(state.selfUid ? t.signedIn(state.selfUid) : t.anonymous, state.selfUid ? 'on' : 'plain') },
      ]));
      const messages = { idle: '', waiting: t.waiting, scanned: t.scanned, expired: t.expired,
        done: state.phase === 'done' ? t.done(state.uid) : '', failed: t.failed(state.error ?? '') };
      const text = pending && !link ? t.invalidPage : messages[state.phase];
      if (progress.textContent !== text) progress.textContent = text;
      progress.hidden = !text;
    }

    function problem(text: string): void {
      error.textContent = text;
      error.hidden = !text;
    }

    async function change(method: 'start' | 'cancel'): Promise<void> {
      if (changing) return;
      changing = start.disabled = cancel.disabled = true;
      statusProblem = false;
      revision++;
      try {
        const next = await ctx.invoke<LoginState>(method);
        if (!signal.aborted) { draw(next); problem(''); }
      } catch (err) {
        if (!signal.aborted) problem(t.failed(errorMessage(err)));
      } finally {
        changing = start.disabled = cancel.disabled = false;
      }
    }

    async function poll(): Promise<void> {
      if (changing || checking) return;
      checking = true;
      const version = revision;
      try {
        const next = await ctx.invoke<LoginState>('state');
        if (signal.aborted || version !== revision) return;
        draw(next);
        if (statusProblem) {
          problem('');
          statusProblem = false;
        }
      } catch (err) {
        if (!signal.aborted && version === revision && !error.textContent) {
          problem(t.stateFailed(errorMessage(err)));
          statusProblem = true;
        }
      } finally {
        checking = false;
      }
    }

    ctx.interval(() => void poll(), 1_500);
    void poll();
  },
};
