// Рантайм-конфиг интерфейса. В проде Go-сервер отдаёт /config.js (window.__APP_CONFIG__ из
// ADMIN_API_BASE_URL и AUTH_*) до загрузки бандла; в деве — runtimeConfig (.env → NUXT_PUBLIC_*).

// Способ входа (AUTH_MODE, ADR-019): local — свои учётки; oidc — интерфейс сам входит через
// OpenID-провайдера и шлёт его токен; proxy — вход делает шлюз перед API, запросы идут с его куками
export type AuthMode = 'local' | 'oidc' | 'proxy'

export interface AuthConfig {
  mode: AuthMode
  oidcIssuer?: string
  oidcClientId?: string
  oidcScopes?: string
  // адреса шлюза; {url} заменяется адресом возврата
  proxyLoginUrl?: string
  proxyRefreshUrl?: string
  proxyLogoutUrl?: string
}

interface AppConfig {
  apiBaseUrl?: string
  auth?: AuthConfig
  // адрес MCP-сервера для AI-агентов (ADR-020); пусто — MCP на этой установке выключен
  mcpUrl?: string
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

// useMcpUrl — адрес MCP-сервера; пусто — выключен (пункт «AI-агенты» в меню не показывается)
export function useMcpUrl(): string {
  return window.__APP_CONFIG__?.mcpUrl || String(useRuntimeConfig().public.mcpUrl ?? '')
}

export function useAuthConfig(): AuthConfig {
  const fromWindow = window.__APP_CONFIG__?.auth
  if (fromWindow?.mode)
    return fromWindow
  const pub = useRuntimeConfig().public
  return {
    mode: (String(pub.authMode || 'local') as AuthMode),
    oidcIssuer: String(pub.oidcIssuer ?? ''),
    oidcClientId: String(pub.oidcClientId ?? ''),
    oidcScopes: String(pub.oidcScopes ?? ''),
    proxyLoginUrl: String(pub.authProxyLoginUrl ?? ''),
    proxyRefreshUrl: String(pub.authProxyRefreshUrl ?? ''),
    proxyLogoutUrl: String(pub.authProxyLogoutUrl ?? ''),
  }
}

// вход через внешний провайдер: пароля, первого админа и правки логина нет
export function isExternalAuth(): boolean {
  return useAuthConfig().mode !== 'local'
}

// appUrl — полный адрес страницы интерфейса (с учётом baseURL) — для возврата после входа и выхода
export function appUrl(path: string): string {
  const base = useRuntimeConfig().app.baseURL || '/'
  return new URL(base.replace(/\/$/, '') + path, window.location.origin).href
}
