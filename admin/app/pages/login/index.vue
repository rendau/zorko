<script setup lang="ts">
definePageMeta({ layout: 'auth' })

const { t } = useI18n()
const auth = useAuth()
const route = useRoute()

const username = ref('')
const password = ref('')
const loading = ref(false)
const error = ref('')

async function submit() {
  loading.value = true
  error.value = ''
  try {
    await auth.login(username.value, password.value)
    const redirect = typeof route.query.redirect === 'string' && route.query.redirect.startsWith('/') ? route.query.redirect : '/issues'
    await navigateTo(redirect)
  }
  catch (e) {
    error.value = apiErrorMessage(e)
  }
  finally {
    loading.value = false
  }
}
</script>

<template>
  <form class="space-y-4" @submit.prevent="submit">
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
