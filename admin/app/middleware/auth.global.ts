// Вход обязателен везде, кроме /login, /login/callback и /setup; пока людей нет — только /setup
// (первый админ, только AUTH_MODE=local: при внешнем входе первого админа даёт провайдер)
export default defineNuxtRouteMiddleware(async (to) => {
  const auth = useAuth()

  if (to.path === '/login/callback')
    return

  if (isExternalAuth()) {
    if (to.path === '/setup')
      return navigateTo('/login')
  }
  else {
    if (auth.bootstrapRequired.value === null) {
      try {
        await auth.checkBootstrap()
      }
      catch {
        // API недоступен — страница покажет свою ошибку
        auth.bootstrapRequired.value = false
      }
    }

    if (auth.bootstrapRequired.value)
      return to.path === '/setup' ? undefined : navigateTo('/setup')
    if (to.path === '/setup')
      return navigateTo('/login')
  }

  await auth.ensureUser()
  if (!auth.user.value)
    return to.path === '/login' ? undefined : navigateTo({ path: '/login', query: { redirect: to.fullPath } })
  if (to.path === '/login')
    return navigateTo('/issues')

  if (to.path.startsWith('/users') && !auth.isAdmin.value)
    return navigateTo('/issues')
})
