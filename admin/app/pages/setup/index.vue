<script setup lang="ts">
// Первый запуск: людей ещё нет — создаём администратора и сразу входим
definePageMeta({ layout: 'auth' })

const { t } = useI18n()
const auth = useAuth()

const name = ref('')
const username = ref('')
const password = ref('')
const loading = ref(false)
const error = ref('')

async function submit() {
  loading.value = true
  error.value = ''
  try {
    await usrApi.create({ name: name.value, username: username.value, password: password.value })
    auth.bootstrapRequired.value = false
    await auth.login(username.value, password.value)
    await navigateTo('/settings')
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
    <div>
      <h1 class="text-base font-semibold text-highlighted">{{ t('setup.title') }}</h1>
      <p class="mt-1 text-sm text-muted">{{ t('setup.description') }}</p>
    </div>
    <UFormField :label="t('usr.field.name')">
      <UInput v-model="name" class="w-full" />
    </UFormField>
    <UFormField :label="t('usr.field.username')" :hint="t('usr.usernameHint')">
      <UInput v-model="username" autocomplete="username" class="w-full" />
    </UFormField>
    <UFormField :label="t('usr.field.password')" :hint="t('usr.passwordHint')">
      <UInput v-model="password" type="password" autocomplete="new-password" class="w-full" />
    </UFormField>
    <UAlert v-if="error" color="error" variant="subtle" :description="error" />
    <UButton type="submit" block :label="t('setup.submit')" :loading="loading" :disabled="!username || !password" />
  </form>
</template>
