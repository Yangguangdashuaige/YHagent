import type { en } from './strings.ts';

export const S: Partial<typeof en> = {
  // advanced/features/prompts/index.ts
  button: "Очистить разговор и начать заново",
  title: "Очистить текущий разговор и начать заново?",
  body: "Сохраняет правки на этой странице, очищает контекст текущего разговора Coo и начинает заново с обновлённым системным промптом. Очистку нельзя отменить. Memory и личность сохраняются.",
  clearing: "Начинается новый разговор…",
  cleared: "Новый разговор начат",
  // release.ts
  updateShort: (version: string) => `Доступна v${version}`,
  updateDownloadingShort: (version: string) => `Загрузка v${version}`,
  updateReadyShort: (version: string) => `v${version} готова`,
  updateFailedShort: (version: string) => `Скачать v${version}`,
  repoHint: 'Открыть проект 小小轩 на GitHub',
  update: (latest: string) => `Вышел 小小轩 ${latest}: скачайте обновление`,
  updateDownloading: (version: string) => `小小轩 ${version} загружается в фоне`,
  updateReady: (version: string) => `小小轩 ${version} загружен и установится при выходе`,
  updateFailed: (version: string) => `小小轩 ${version} не загрузился: скачать вручную`,
};
