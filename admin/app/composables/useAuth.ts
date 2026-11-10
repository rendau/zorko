import type { UsrMain } from '~/types/usr'

// Вошедший человек (ADR-005, ADR-019): токены — utils/auth.ts (local) и utils/externalAuth.ts
// (oidc, proxy), профиль — /usr/profile. Роль в интерфейсе только прячет кнопки, права проверяет сервер.
export function useAuth() {
  const user = useState<UsrMain | null>('auth:user', () => null)
  const bootstrapRequired = useState<boolean | null>('auth:bootstrap', () => null)
  // код отказа при проверке входа (usr_disabled — отключён в «Людях»); показывает страница входа
  const accessError = useState<string>('auth:accessError', () => '')

  const isAdmin = computed(() => !!user.value?.is_admin)

  async function checkBootstrap() {
    bootstrapRequired.value = (await usrApi.bootstrapStatus()).bootstrap_required
  }

  async function ensureUser() {
    if (user.value)
      return
    const mode = useAuthConfig().mode
    // local и oidc без токена — точно не вошёл; proxy — кук не видно, спрашиваем сервер
    if ((mode === 'local' && !getStoredJwt()) || (mode === 'oidc' && !await oidcManager().getUser().catch(() => null)))
      return
    try {
      user.value = await usrApi.profile({ authRedirect: false })
      accessError.value = ''
    }
    catch (e) {
      user.value = null
      accessError.value = apiErrorCode(e) === 'usr_disabled' ? 'usr_disabled' : ''
    }
  }

  async function login(username: string, password: string) {
    const pair = await usrApi.login(username, password)
    storeTokens(pair.jwt, pair.refresh_token)
    user.value = await usrApi.profile()
  }

  async function logout() {
    user.value = null
    if (isExternalAuth()) {
      // токены провайдера забывает сам выход: ему нужен ID-токен (id_token_hint), иначе провайдер
      // переспрашивает «точно выйти?»
      await externalLogout()
      return
    }
    clearTokens()
    await navigateTo('/login')
  }

  return { user, bootstrapRequired, accessError, isAdmin, checkBootstrap, ensureUser, login, logout }
}
