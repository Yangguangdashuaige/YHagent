import { rmSync } from 'node:fs';
import { FUNASR_MODEL, RuntimeStore, sha256File } from '../packages/cortico-world-desktop-pet/src/runtime/store.ts';

export async function prepareFunAsr(root: string): Promise<void> {
  const { funasr } = new RuntimeStore({ modelsDir: () => root, runtimesRoot: () => root, funasrModel: FUNASR_MODEL });
  if (funasr.state().phase === 'ready') {
    for (const artifact of FUNASR_MODEL.files) {
      const path = funasr.file(artifact.name);
      if (await sha256File(path) !== artifact.sha256) rmSync(path);
    }
  }
  if (funasr.state().phase !== 'ready') await funasr.install();
  const state = funasr.state();
  if (state.phase !== 'ready') throw new Error(state.detail ?? 'SenseVoiceSmall was not prepared');
}
