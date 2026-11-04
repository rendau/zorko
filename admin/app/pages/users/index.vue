<script setup lang="ts">
import type { UsrMain } from '~/types/usr'

// Люди (только админ): добавить с временным паролем, роль, отключить, сбросить пароль, удалить
const { t } = useI18n()
const auth = useAuth()
const { loading, run } = useApiAction()

const items = ref<UsrMain[]>([])
const error = ref('')

async function load() {
  error.value = ''
  try {
    items.value = (await usrApi.list({ page: 0, page_size: 100 })).results
  }
  catch (e) {
    error.value = apiErrorMessage(e)
  }
}
onMounted(load)

const modal = ref(false)
const editing = ref<UsrMain | null>(null)
const name = ref('')
const username = ref('')
const password = ref('')
const isAdmin = ref(false)
const active = ref(true)
const confirm = ref<UsrMain | null>(null)

function openCreate() {
  editing.value = null
  name.value = ''
  username.value = ''
  password.value = ''
  isAdmin.value = false
  active.value = true
  modal.value = true
}

function openEdit(u: UsrMain) {
  editing.value = u
  name.value = u.name
  username.value = u.username
  password.value = ''
  isAdmin.value = u.is_admin
  active.value = u.active
  modal.value = true
}

const isSelf = computed(() => !!editing.value && editing.value.id === auth.user.value?.id)

async function save() {
  const body = {
    name: name.value.trim(),
    username: username.value.trim(),
    is_admin: isAdmin.value,
    active: active.value,
    ...(password.value ? { password: password.value } : {}),
  }
  const ok = await run(async () => {
    if (editing.value)
      await usrApi.update(editing.value.id, body)
    else
      await usrApi.create(body)
    return true
  }, { success: t('common.saved') })
  if (ok) {
    modal.value = false
    await load()
  }
}

async function remove() {
  const u = confirm.value
  if (u && await run(() => usrApi.remove(u.id), { success: t('common.deleted') }) !== undefined) {
    confirm.value = null
    await load()
  }
}
</script>

<template>
  <UDashboardPanel id="users">
    <template #header>
      <UDashboardNavbar :title="t('users.title')">
        <template #leading>
          <UDashboardSidebarCollapse />
        </template>
        <template #right>
          <UButton icon="i-lucide-user-plus" :label="t('users.create')" @click="openCreate" />
        </template>
      </UDashboardNavbar>
    </template>

    <template #body>
      <LoadAlert :title="t('users.loadError')" :error="error" @retry="load" />
      <div class="overflow-x-auto rounded-lg border border-default">
        <table class="w-full min-w-[560px] text-sm">
          <thead class="bg-elevated/50 text-left text-xs text-muted">
            <tr>
              <th class="px-4 py-2 font-medium">{{ t('usr.field.name') }}</th>
              <th class="px-4 py-2 font-medium">{{ t('usr.field.username') }}</th>
              <th class="px-4 py-2 font-medium">{{ t('users.role') }}</th>
              <th class="px-4 py-2 font-medium">{{ t('users.status') }}</th>
              <th class="px-4 py-2" />
            </tr>
          </thead>
          <tbody class="divide-y divide-default">
            <tr v-for="u in items" :key="u.id" :class="u.active ? '' : 'opacity-60'">
              <td class="px-4 py-2 text-highlighted">{{ u.name }}</td>
              <td class="px-4 py-2 font-mono text-xs">{{ u.username }}</td>
              <td class="px-4 py-2">
                <UBadge :color="u.is_admin ? 'primary' : 'neutral'" variant="subtle" size="sm">{{ u.is_admin ? t('usr.role.admin') : t('usr.role.member') }}</UBadge>
              </td>
              <td class="px-4 py-2">{{ u.active ? t('users.active') : t('users.disabled') }}</td>
              <td class="px-4 py-2 text-right whitespace-nowrap">
                <UButton size="xs" color="neutral" variant="ghost" icon="i-lucide-pencil" :aria-label="t('common.edit')" @click="openEdit(u)" />
                <UButton v-if="u.id !== auth.user.value?.id" size="xs" color="error" variant="ghost" icon="i-lucide-trash-2" :aria-label="t('common.delete')" @click="confirm = u" />
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <UModal v-model:open="modal" :title="editing ? t('users.edit') : t('users.create')">
        <template #body>
          <form id="usr-form" class="space-y-4" @submit.prevent="save">
            <UFormField :label="t('usr.field.name')">
              <UInput v-model="name" class="w-full" autofocus />
            </UFormField>
            <UFormField :label="t('usr.field.username')" :hint="t('usr.usernameHint')">
              <UInput v-model="username" autocomplete="off" class="w-full font-mono" />
            </UFormField>
            <UFormField :label="editing ? t('users.newPassword') : t('users.tempPassword')" :hint="t('usr.passwordHint')">
              <UInput v-model="password" type="password" autocomplete="new-password" class="w-full" />
            </UFormField>
            <UFormField :label="t('usr.role.admin')" :description="isSelf ? t('users.selfHint') : t('users.adminHint')">
              <USwitch v-model="isAdmin" :disabled="isSelf" />
            </UFormField>
            <UFormField :label="t('users.active')">
              <USwitch v-model="active" :disabled="isSelf" />
            </UFormField>
          </form>
        </template>
        <template #footer>
          <div class="flex w-full justify-end gap-2">
            <UButton color="neutral" variant="ghost" :label="t('common.cancel')" @click="modal = false" />
            <UButton type="submit" form="usr-form" :label="t('common.save')" :loading="loading" :disabled="!username || (!editing && !password)" />
          </div>
        </template>
      </UModal>

      <ConfirmModal
        :open="!!confirm"
        :title="t('users.deleteTitle')"
        :description="t('users.deleteDescription')"
        :loading="loading"
        @update:open="v => { if (!v) confirm = null }"
        @confirm="remove"
      />
    </template>
  </UDashboardPanel>
</template>
