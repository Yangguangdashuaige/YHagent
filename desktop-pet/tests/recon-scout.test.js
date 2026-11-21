import { describe, expect, it } from 'vitest';
import { JSDOM } from 'jsdom';
import * as kit from '../packages/cortico-world-desktop-pet/web/kit/body.js';
import { createScout } from '../packages/cortico-world-desktop-pet/web/recon-scout/figure.js';

describe('recon scout animation', () => {
  it('moves the idle head, blinks, speaks, walks and keeps those motions after changing the outfit', async () => {
    const page = new JSDOM('<div id="stage"></div>');
    try {
      const stage = page.window.document.getElementById('stage');
      const base = new URL('http://127.0.0.1/web/recon-scout/');
      const body = await createScout(base, {
        scheme: 'field',
        kit,
        loadImage: async (url) => ({ src: url.href }),
        asset: (file) => new URL(file, base),
        host: {
          root: stage,
          bounds: () => ({ W: 800, H: 420, floorY: 400, S: 0.42 }),
          start: { x: 200, enter: false }, emit() {}, sound() {},
        },
      });
      body.set({ roam: 'off' });
      body.step(1 / 60);
      const part = (id) => stage.querySelector(`[data-part="${id}"]`);
      const head = part('head').getAttribute('transform');
      let blink = false;
      for (let i = 0; i < 210; i++) {
        body.step(1 / 60);
        blink ||= Number(part('eyelid').getAttribute('opacity')) > 0.8;
      }
      expect(part('head').getAttribute('transform')).not.toBe(head);
      expect(blink).toBe(true);
      body.talk();
      body.step(1 / 60);
      expect(Number(part('mouth').getAttribute('opacity'))).toBeGreaterThan(0);
      const leg = part('left-leg').getAttribute('transform');
      expect(body.walk(510, false, 7)).toBe(true);
      for (let i = 0; i < 45; i++) body.step(1 / 60);
      expect(part('left-leg').getAttribute('transform')).not.toBe(leg);
      body.setScheme('night');
      body.step(1 / 60);
      expect(part('head').querySelector('image').getAttribute('href')).toContain('night.png');
      body.dispose();
    } finally {
      page.window.close();
    }
  });
});
