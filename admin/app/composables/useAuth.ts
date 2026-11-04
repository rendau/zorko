import type { UsrMain } from '~/types/usr'

// Вошедший человек (ADR-005): токены — utils/auth.ts, профиль — /usr/profile.
// Роль в интерфейсе только прячет кнопки, права проверяет сервер.
export function useAuth() {
  const user = useState<UsrMain | null>('auth:user', () => null)
  const bootstrapRequired = useState<boolean | null>('auth:bootstrap', () => null)

  const isAdmin = computed(() => !!user.value?.is_admin)

  async function checkBootstrap() {
    bootstrapRequired.value = (await usrApi.bootstrapStatus()).bootstrap_required
  }

  async function ensureUser() {
    if (user.value || !getStoredJwt())
      return
    try {
      user.value = await usrApi.profile()
    }
    catch {
      user.value = null
    }
  }

  async function login(username: string, password: string) {
    const pair = await usrApi.login(username, password)
    storeTokens(pair.jwt, pair.refresh_token)
    user.value = await usrApi.profile()
  }

  async function logout() {
    clearTokens()
    user.value = null
    await navigateTo('/login')
  }

  return { user, bootstrapRequired, isAdmin, checkBootstrap, ensureUser, login, logout }
}
