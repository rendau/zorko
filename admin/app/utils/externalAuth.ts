import { UserManager, WebStorageStateStore } from 'oidc-client-ts'

// Вход через внешний провайдер (ADR-019).
//
// oidc — интерфейс сам ведёт вход (Authorization Code + PKCE, public client), токены провайдера
// лежат в localStorage (как свои токены в режиме local — вход общий для вкладок), в API уходит
// access-токен. Продление — по refresh-токену (signinSilent).
//
// proxy — вход, куки и продление — у шлюза перед API; интерфейс знает только три адреса из
// AUTH_PROXY_*: войти, продлить (POST), выйти. Запросы к API идут с куками шлюза.

let manager: UserManager | null = null

export function oidcManager(): UserManager {
  if (!manager) {
    const c = useAuthConfig()
    manager = new UserManager({
      authority: c.oidcIssuer ?? '',
      client_id: c.oidcClientId ?? '',
      scope: c.oidcScopes || 'openid profile email',
      response_type: 'code',
      redirect_uri: appUrl('/login/callback'),
      post_logout_redirect_uri: appUrl('/login'),
      userStore: new WebStorageStateStore({ store: window.localStorage }),
      automaticSilentRenew: true,
    })
  }
  return manager
}

export async function oidcAccessToken(): Promise<string> {
  try {
    const user = await oidcManager().getUser()
    return user && !user.expired ? user.access_token : ''
  }
  catch {
    return ''
  }
}

// одно продление на все запросы, упавшие с 401 одновременно
let renewing: Promise<boolean> | null = null

export function oidcRenew(): Promise<boolean> {
  renewing ??= oidcManager().signinSilent()
    .then(user => !!user)
    .catch(() => false)
    .finally(() => {
      renewing = null
    })
  return renewing
}

// withReturnUrl — {url} в адресе шлюза заменяется адресом возврата
export function withReturnUrl(template: string, returnUrl: string): string {
  return template.replaceAll('{url}', encodeURIComponent(returnUrl))
}

export function proxyRefresh(): Promise<boolean> {
  const url = useAuthConfig().proxyRefreshUrl
  if (!url)
    return Promise.resolve(false)
  // POST без тела — «простой» запрос, предварительный OPTIONS не нужен
  renewing ??= fetch(url, { method: 'POST', credentials: 'include' })
    .then(r => r.ok)
    .catch(() => false)
    .finally(() => {
      renewing = null
    })
  return renewing
}

// externalLogin — уход на вход провайдера или шлюза; redirect — путь интерфейса после входа
export async function externalLogin(redirect: string) {
  const c = useAuthConfig()
  if (c.mode === 'oidc') {
    await oidcManager().signinRedirect({ state: { redirect } })
    return
  }
  if (c.proxyLoginUrl)
    window.location.href = withReturnUrl(c.proxyLoginUrl, appUrl(redirect))
}

export async function externalLogout() {
  const c = useAuthConfig()
  if (c.mode === 'oidc') {
    try {
      await oidcManager().signoutRedirect()
      return
    }
    catch {
      await oidcManager().removeUser()
    }
  }
  else if (c.proxyLogoutUrl) {
    window.location.href = withReturnUrl(c.proxyLogoutUrl, appUrl('/login'))
    return
  }
  await navigateTo('/login')
}

// loginErrorCode — код ошибки входа, с которым провайдер (error) или шлюз (auth_error) вернул на страницу
export function loginErrorCode(query: Record<string, unknown>): string {
  const v = query.auth_error ?? query.error
  return typeof v === 'string' ? v.slice(0, 64) : ''
}
