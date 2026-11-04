// Рантайм-конфиг интерфейса. В проде Go-сервер отдаёт /config.js (window.__APP_CONFIG__ из
// ADMIN_API_BASE_URL) до загрузки бандла; в деве — runtimeConfig (.env → NUXT_PUBLIC_*).

interface AppConfig {
  apiBaseUrl?: string
}

declare global {
  interface Window {
    __APP_CONFIG__?: AppConfig
  }
}

export function useApiBaseUrl(): string {
  const fromWindow = window.__APP_CONFIG__?.apiBaseUrl
  if (fromWindow)
    return fromWindow
  return String(useRuntimeConfig().public.apiBaseUrl ?? '')
}
