import { copyFileSync, existsSync, mkdirSync, renameSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { downloadFile } from '../packages/cortico-world-desktop-pet/src/runtime/download.ts';
import { sha256File } from '../packages/cortico-world-desktop-pet/src/runtime/store.ts';

const MODEL = 'csukuangfj/vits-melo-tts-zh_en/resolve/a0d5c6a264c0ef92d70d8661d8cc502d79627cd6';
const MIRRORS = ['https://huggingface.co', 'https://hf-mirror.com'];
const FILES = {
  'model.int8.onnx': 'f085f5079e05f039b800aeb542f5253c26a303211b0c6465d0d9387977855a63',
  'lexicon.txt': '7236884b02435ac5d10cf69b4be40a61b45aa676b5300f0e412f185748fee528',
  'tokens.txt': 'd18664a7e12bd7ea1022ddaf951e534e136815016c5a809d6b64156bffb4369d',
  'date.fst': 'eb8aa079ae3cb81d8f4404992f39d61a0cb990947512b5b8d1e54d1f6980e718',
  'number.fst': '743f402181fcfebf76cc2f0546b71fa26476e626fbe4e460fb7b4c3a7a8bd5bd',
  'phone.fst': '1ac2b6fa56b1442320c4de7db08353bab8963a2b57f365eebcdd3a2d3562f8d7',
  LICENSE: '88a50e5a02bbc2a5c2f084dc19da751aa97b1690f5fda76cd8005c8634d1ca70',
};

async function fetchModelFile(name: string, path: string, hash: string): Promise<void> {
  if (existsSync(path) && await sha256File(path) === hash) return;
  const pending = path + '.partial';
  const errors: unknown[] = [];
  for (const mirror of MIRRORS) {
    try {
      await downloadFile(`${mirror}/${MODEL}/${name}`, pending, { signal: AbortSignal.timeout(90_000) });
      if (await sha256File(pending) !== hash) throw new Error('SHA-256 mismatch');
      renameSync(pending, path);
      return;
    } catch (error) {
      rmSync(pending, { force: true });
      errors.push(error);
    }
  }
  throw new AggregateError(errors, `Could not prepare MeloTTS file ${name}`);
}

export async function prepareMeloTts(cache: string, target: string): Promise<void> {
  mkdirSync(target, { recursive: true });
  await Promise.all(Object.entries(FILES).map(async ([name, hash]) => {
    const path = join(cache, name);
    await fetchModelFile(name, path, hash);
    copyFileSync(path, join(target, name));
  }));
}
