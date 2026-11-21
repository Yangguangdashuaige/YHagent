const SVG = 'http://www.w3.org/2000/svg';
const HIP = 174;
const OUTFITS = {
  field: { top: 25, accent: '#d3d58d', eyes: [[114, 81, 19], [138, 76, -16]], mouth: [128, 89] },
  night: { top: 22, accent: '#86e9eb', eyes: [[114, 78, 20], [137, 73, -17]], mouth: [128, 86] },
};

export async function createScout(_base, { scheme, kit, loadImage, asset, host }) {
  const [field, night] = await Promise.all([
    loadImage(asset('field.png')),
    loadImage(asset('night.png')),
  ]);
  const images = { field, night };
  const doc = host.root.ownerDocument;
  const colors = { z: OUTFITS.field.accent };
  let outfit = 'field';
  let pose;
  let gait = 0;

  function svg(tag, attributes = {}) {
    const node = doc.createElementNS(SVG, tag);
    for (const [name, value] of Object.entries(attributes)) node.setAttribute(name, String(value));
    return node;
  }

  function buildPose(parent) {
    const defs = svg('defs');
    const art = svg('g');

    function part(name, outline) {
      const id = 'recon-scout-' + name;
      const clip = svg('clipPath', { id });
      clip.append(svg('path', { d: outline }));
      defs.append(clip);
      const group = svg('g', { 'data-part': name });
      const image = svg('image', { x: 48, width: 160, height: 240, crossorigin: 'anonymous', 'clip-path': 'url(#' + id + ')' });
      group.append(image);
      art.append(group);
      return { group, image };
    }

    const left = part('left-leg', 'M55 166H128V267H55Z');
    const right = part('right-leg', 'M128 166H201V267H128Z');
    const torso = part('torso', 'M55 91H201V177H55Z');
    const head = part('head', 'M55 5H201V102H166Q145 99 128 105L104 106H55Z');

    const eyelids = [0, 1].map(() => {
      const lid = svg('g', { 'data-part': 'eyelid', opacity: 0 });
      lid.append(
        svg('path', { d: 'M-6.4 0Q-3 -5.5 4.2 -5L6.5 -2Q7 2 2 4.6Q-4.5 5.1 -6.4 0Z', fill: '#eeb292' }),
        svg('path', { d: 'M-6.3 0Q-.5 4 6.4 -1', stroke: '#322535', 'stroke-width': 1.15, fill: 'none', 'stroke-linecap': 'round' }),
      );
      return lid;
    });
    const mouth = svg('ellipse', { 'data-part': 'mouth', rx: 3.9, ry: 2.6, fill: '#57323c', opacity: 0 });
    const waves = svg('path', { d: 'M173 78q8 7 0 14M178 74q13 11 0 22M184 69q19 16 0 32', 'stroke-width': 2.8, fill: 'none', 'stroke-linecap': 'round' });
    const dots = [173, 183, 193].map((cx) => svg('circle', { cx, cy: 46, r: 2.4 }));
    const glint = svg('path', { d: 'M178 49v14m-7-7h14m3-20v8m-4-4h8', 'stroke-width': 2.8, fill: 'none', 'stroke-linecap': 'round' });
    head.group.append(...eyelids, mouth, waves, ...dots, glint);
    parent.append(defs, art);
    return { art, left, right, torso, head, eyelids, mouth, waves, dots, glint };
  }

  function setScheme(next) {
    outfit = next === 'night' ? 'night' : 'field';
    const { top, accent, mouth } = OUTFITS[outfit];
    colors.z = accent;
    if (!pose) return;
    const source = images[outfit].src;
    for (const part of [pose.left, pose.right, pose.torso, pose.head]) {
      part.image.setAttribute('href', source);
      part.image.setAttribute('y', top);
    }
    pose.mouth.setAttribute('cx', mouth[0]);
    pose.mouth.setAttribute('cy', mouth[1]);
    pose.waves.setAttribute('stroke', accent);
    pose.glint.setAttribute('stroke', accent);
    for (const dot of pose.dots) dot.setAttribute('fill', accent);
  }

  function drawFace(pose, face, frame) {
    const { eyes, mouth } = OUTFITS[outfit];
    const restingBlink = Math.max(0, 1 - Math.abs((frame.t + 1.7) % 3.4 - 0.14) / 0.14);
    const blink = Math.max(frame.blink, restingBlink, frame.mode === 'sleep' ? 1 : 0);
    for (const [index, lid] of pose.eyelids.entries()) {
      const [x, y, angle] = eyes[index];
      lid.setAttribute('transform', `translate(${x} ${y}) rotate(${angle}) scale(1 ${Math.max(0.04, blink)})`);
      lid.setAttribute('opacity', Math.min(1, blink * 1.7));
    }
    if (frame.talk > 0.06) {
      const open = 0.45 + frame.talk * (0.38 + 0.28 * Math.sin(frame.t * 24) ** 2);
      pose.mouth.setAttribute('transform', `translate(0 ${mouth[1] * (1 - open)}) scale(1 ${open})`);
      pose.mouth.setAttribute('opacity', Math.min(1, frame.talk * 2.5));
    } else {
      pose.mouth.setAttribute('opacity', 0);
    }
    pose.waves.setAttribute('opacity', face.listen ? 0.35 + 0.52 * Math.sin(frame.t * 8) ** 2 : 0);
    for (const [index, dot] of pose.dots.entries()) {
      dot.setAttribute('opacity', face.think ? 0.35 + 0.52 * Math.sin(frame.t * 5 - index) ** 2 : 0);
    }
    pose.glint.setAttribute('opacity', frame.face === 'happy' ? 0.55 + 0.4 * Math.sin(frame.t * 7) ** 2 : 0);
  }

  setScheme(scheme);
  return kit.createBody(host, {
    figure: {
      extent: [62, 21, 197, 256],
      hits: [[127, 61, 46], [129, 136, 40], [126, 195, 32], [126, 234, 32]],
      colors,
      setScheme,
      draw(parent, face, frame) {
        if (!pose) {
          pose = buildPose(parent);
          setScheme(outfit);
        }
        const moving = frame.mode === 'walk' || frame.mode === 'run';
        gait += ((moving ? 1 : 0) - gait) * 0.16;
        const phase = frame.t * (frame.mode === 'run' ? 18 : 12);
        const stride = Math.sin(phase) * gait;
        const weight = Math.sin(frame.t * 2.45);
        const bounce = Math.abs(stride) * 5;
        const squash = 1 - 0.16 * Math.min(1, Math.max(frame.sit, frame.drowse * 0.65));
        const tilt = weight * 3.5 + (frame.look?.[0] ?? 0) * 0.16 + (face.listen ? -5 : face.think ? 5 : 0);
        const rise = 1.4 * (1 + weight) + bounce;

        pose.art.setAttribute('transform', `translate(0 256) scale(1 ${squash}) translate(0 -256)`);
        pose.left.group.setAttribute('transform', `translate(0 ${-Math.max(0, stride) * 10}) rotate(${stride * 8} 111 ${HIP})`);
        pose.right.group.setAttribute('transform', `translate(0 ${Math.min(0, stride) * 10}) rotate(${-stride * 8} 145 ${HIP})`);
        pose.torso.group.setAttribute('transform', `translate(128 ${HIP}) scale(${1 + weight * 0.014} ${1 + rise / 85}) translate(-128 -${HIP})`);
        pose.head.group.setAttribute('transform', `translate(${stride * 2.8} ${-rise - frame.talk * 1.8}) rotate(${tilt} 128 99)`);
        drawFace(pose, face, frame);
      },
    },
  });
}
