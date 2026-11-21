import { existsSync } from 'node:fs';
import { createRequire } from 'node:module';
import { join } from 'node:path';

type VoiceModel = {
  generateAsync(input: { text: string; sid: number; speed: number; enableExternalBuffer: false }): Promise<{ samples: Float32Array; sampleRate: number }>;
};

const require = createRequire(import.meta.url);

export function createCuteVoice(root = process.env.YHPET_TTS_ROOT): ((text: string) => Promise<Buffer | null>) | undefined {
  if (!root) return undefined;
  const vits = { model: join(root, 'model.int8.onnx'), lexicon: join(root, 'lexicon.txt'), tokens: join(root, 'tokens.txt') };
  if (!Object.values(vits).every((file) => existsSync(file))) return undefined;
  const ruleFsts = ['date.fst', 'number.fst', 'phone.fst'].map((name) => join(root, name)).filter(existsSync).join(',');
  let model: Promise<VoiceModel> | undefined;
  let busy = false;

  return async (text) => {
    const spoken = text.replace(/https?:\/\/[^\s]+/gi, '').trim();
    if (!spoken || Array.from(spoken).length > 110 || busy) return null;
    busy = true;
    try {
      const { OfflineTts } = require('sherpa-onnx-node') as {
        OfflineTts: { createAsync(config: unknown): Promise<VoiceModel> };
      };
      model ??= OfflineTts.createAsync({ model: { vits, numThreads: 2, provider: 'cpu' }, ruleFsts, maxNumSentences: 2 });
      // Electron only accepts JavaScript-owned ArrayBuffers.
      const audio = await (await model).generateAsync({ text: spoken, sid: 0, speed: 0.95, enableExternalBuffer: false });
      return audio.samples.length ? wave(audio.samples, Math.round(audio.sampleRate * 1.1)) : null;
    } finally {
      busy = false;
    }
  };
}

function wave(samples: Float32Array, rate: number): Buffer {
  const length = samples.length * 2;
  const buffer = Buffer.alloc(44 + length);
  buffer.write('RIFF', 0);
  buffer.writeUInt32LE(36 + length, 4);
  buffer.write('WAVEfmt ', 8);
  buffer.writeUInt32LE(16, 16);
  buffer.writeUInt16LE(1, 20);
  buffer.writeUInt16LE(1, 22);
  buffer.writeUInt32LE(rate, 24);
  buffer.writeUInt32LE(rate * 2, 28);
  buffer.writeUInt16LE(2, 32);
  buffer.writeUInt16LE(16, 34);
  buffer.write('data', 36);
  buffer.writeUInt32LE(length, 40);
  for (let i = 0; i < samples.length; i++) {
    buffer.writeInt16LE(Math.round(Math.max(-1, Math.min(1, samples[i]!)) * 32767), 44 + i * 2);
  }
  return buffer;
}
