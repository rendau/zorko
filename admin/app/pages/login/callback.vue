<script setup lang="ts">
definePageMeta({ layout: 'auth' })

// Возврат от OpenID-провайдера (AUTH_MODE=oidc): обмен кода на токены и переход туда, откуда ушли
const { t } = useI18n()
const route = useRoute()
const error = ref('')

onMounted(async () => {
  const code = loginErrorCode(route.query)
  if (code) {
    await navigateTo({ path: '/login', query: { error: code } })
    return
  }
  try {
    const user = await oidcManager().signinRedirectCallback()
    const state = user.state as { redirect?: string } | undefined
    const redirect = state?.redirect?.startsWith('/') ? state.redirect : '/issues'
    await navigateTo(redirect, { replace: true })
  }
  catch (e) {
    error.value = String(e)
  }
})
</script>

<template>
  <div class="space-y-4">
    <h1 class="text-base font-semibold text-highlighted">{{ t('login.title') }}</h1>
    <template v-if="error">
      <UAlert color="error" variant="subtle" :title="t('login.callbackFailed')" :description="error" />
      <UButton block :label="t('login.retry')" to="/login" />
    </template>
    <p v-else class="text-sm text-muted">{{ t('login.callback') }}</p>
  </div>
</template>
