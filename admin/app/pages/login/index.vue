<script setup lang="ts">
definePageMeta({ layout: 'auth' })

// Вход по AUTH_MODE (ADR-019): local — логин и пароль; oidc и proxy — кнопка ухода на вход
// провайдера или шлюза. Сам на вход не уводим: провайдер может отказать (нет доступа) и вернуть
// сюда — автопереход зациклился бы.
const { t } = useI18n()
const auth = useAuth()
const route = useRoute()
const external = isExternalAuth()

const username = ref('')
const password = ref('')
const loading = ref(false)
const error = ref('')

const redirect = computed(() => typeof route.query.redirect === 'string' && route.query.redirect.startsWith('/') ? route.query.redirect : '/issues')

// почему вход не состоялся: код провайдера/шлюза в адресе или отказ zorko
const externalError = computed(() => {
  if (auth.accessError.value === 'usr_disabled')
    return t('login.disabled')
  const code = loginErrorCode(route.query)
  if (!code)
    return ''
  return code === 'access_denied' ? t('login.accessDenied') : t('login.failed', { code })
})

async function submit() {
  loading.value = true
  error.value = ''
  try {
    await auth.login(username.value, password.value)
    await navigateTo(redirect.value)
  }
  catch (e) {
    error.value = apiErrorMessage(e)
  }
  finally {
    loading.value = false
  }
}

async function externalSubmit() {
  loading.value = true
  error.value = ''
  try {
    await externalLogin(redirect.value)
  }
  catch (e) {
    error.value = String(e)
    loading.value = false
  }
}
</script>

<template>
  <div v-if="external" class="space-y-4">
    <h1 class="text-base font-semibold text-highlighted">{{ t('login.title') }}</h1>
    <p class="text-sm text-muted">{{ t('login.externalHint') }}</p>
    <UAlert v-if="externalError" color="warning" variant="subtle" :description="externalError" />
    <UAlert v-if="error" color="error" variant="subtle" :description="error" />
    <UButton block icon="i-lucide-log-in" :label="t('login.external')" :loading="loading" @click="externalSubmit" />
  </div>
  <form v-else class="space-y-4" @submit.prevent="submit">
    <h1 class="text-base font-semibold text-highlighted">{{ t('login.title') }}</h1>
    <UFormField :label="t('usr.field.username')">
      <UInput v-model="username" autocomplete="username" autofocus class="w-full" />
    </UFormField>
    <UFormField :label="t('usr.field.password')">
      <UInput v-model="password" type="password" autocomplete="current-password" class="w-full" />
    </UFormField>
    <UAlert v-if="error" color="error" variant="subtle" :description="error" />
    <UButton type="submit" block :label="t('login.submit')" :loading="loading" :disabled="!username || !password" />
  </form>
</template>
