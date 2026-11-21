import type { en } from './strings.ts';

export const S: Partial<typeof en> = {
  // advanced/features/prompts/index.ts
  button: "Borrar conversación y reiniciar",
  title: "¿Borrar la conversación actual y reiniciar?",
  body: "Guarda los cambios de esta página, borra el contexto de la conversación actual de Coo y reinicia con el prompt del sistema actualizado. El borrado no se puede deshacer. Memory y la personalidad se conservan.",
  clearing: "Iniciando una nueva conversación…",
  cleared: "Nueva conversación iniciada",
  // release.ts
  updateShort: (version: string) => `v${version} disponible`,
  updateDownloadingShort: (version: string) => `Descargando v${version}`,
  updateReadyShort: (version: string) => `v${version} lista`,
  updateFailedShort: (version: string) => `Descargar v${version}`,
  repoHint: 'Abrir el proyecto 小小轩 en GitHub',
  update: (latest: string) => `Salió 小小轩 ${latest}: descarga la actualización`,
  updateDownloading: (version: string) => `小小轩 ${version} se está descargando en segundo plano`,
  updateReady: (version: string) => `小小轩 ${version} ya se descargó y se instala al salir`,
  updateFailed: (version: string) => `小小轩 ${version} no se descargó: descárgalo a mano`,
};
