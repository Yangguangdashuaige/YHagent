import { christmasHat } from '../base/ui/icons.ts';

export function sidebarBrand(doc: Document, signal: AbortSignal): HTMLDivElement {
  const lockup = doc.createElement('div');
  lockup.className = 'companion-lockup';
  lockup.setAttribute('role', 'img');
  lockup.setAttribute('aria-label', '小小轩');

  const prefix = doc.createElement('span');
  prefix.className = 'companion-brand-prefix';
  prefix.textContent = '小小';
  const mark = doc.createElement('span');
  mark.className = 'companion-brand-mark';
  mark.textContent = '轩';
  lockup.append(prefix, mark);

  const hat = doc.createElementNS('http://www.w3.org/2000/svg', 'svg');
  hat.classList.add('companion-brand-hat');
  hat.setAttribute('viewBox', '0 -45 180 128');
  hat.setAttribute('aria-hidden', 'true');
  hat.append(christmasHat(doc));

  const win = doc.defaultView!;
  let midnightTimer: ReturnType<typeof setTimeout> | undefined;
  const refresh = (): void => {
    clearTimeout(midnightTimer);
    const now = new Date();
    if (now.getMonth() === 11 && (now.getDate() === 24 || now.getDate() === 25)) mark.append(hat);
    else hat.remove();
    const midnight = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1);
    midnightTimer = setTimeout(refresh, midnight.getTime() - now.getTime());
  };
  refresh();
  win.addEventListener('focus', refresh, { signal });
  doc.addEventListener('visibilitychange', refresh, { signal });
  signal.addEventListener('abort', () => clearTimeout(midnightTimer), { once: true });
  return lockup;
}
