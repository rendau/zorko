<script setup lang="ts">
// Свой профиль: имя и пароль (смена пароля требует текущий). При внешнем входе (ADR-019) имя,
// логин и пароль — у провайдера: только просмотр.
const open = defineModel<boolean>('open', { required: true })
const { t } = useI18n()
const external = isExternalAuth()
const auth = useAuth()
const { loading, run } = useApiAction()

const name = ref('')
const currentPassword = ref('')
const password = ref('')

watch(open, (v) => {
  if (v) {
    name.value = auth.user.value?.name ?? ''
    currentPassword.value = ''
    password.value = ''
  }
})

async function save() {
  const ok = await run(() => usrApi.updateProfile({
    name: name.value,
    ...(password.value ? { password: password.value, current_password: currentPassword.value } : {}),
  }), { success: t('profile.saved') })
  if (ok !== undefined) {
    auth.user.value = await usrApi.profile()
    open.value = false
  }
}
</script>

<template>
  <UModal v-model:open="open" :title="t('profile.title')">
    <template #body>
      <div v-if="external" class="space-y-3 text-sm">
        <div><span class="text-muted">{{ t('usr.field.name') }}:</span> {{ auth.user.value?.name }}</div>
        <div><span class="text-muted">{{ t('usr.field.username') }}:</span> <span class="font-mono">{{ auth.user.value?.username }}</span></div>
        <UAlert color="neutral" variant="subtle" :description="t('profile.external')" />
      </div>
      <form v-else id="profile-form" class="space-y-4" @submit.prevent="save">
        <UFormField :label="t('usr.field.name')">
          <UInput v-model="name" class="w-full" />
        </UFormField>
        <UFormField :label="t('profile.currentPassword')">
          <UInput v-model="currentPassword" type="password" autocomplete="current-password" class="w-full" />
        </UFormField>
        <UFormField :label="t('profile.newPassword')" :hint="t('usr.passwordHint')">
          <UInput v-model="password" type="password" autocomplete="new-password" class="w-full" />
        </UFormField>
      </form>
    </template>
    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton color="neutral" variant="ghost" :label="external ? t('common.close') : t('common.cancel')" @click="open = false" />
        <UButton v-if="!external" type="submit" form="profile-form" :label="t('common.save')" :loading="loading" />
      </div>
    </template>
  </UModal>
</template>
