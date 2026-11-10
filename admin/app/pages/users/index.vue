<script setup lang="ts">
import type { SelectMenuItem } from '@nuxt/ui'
import type { OrgMain } from '~/types/org'
import type { ProjectMain } from '~/types/project'
import type { UsrMain } from '~/types/usr'

// Люди (только админ): добавить со случайным паролем, роль, проекты участника, отключить,
// сбросить пароль, удалить. «Сохранить и скопировать» кладёт в буфер адрес, логин и пароль.
// При внешнем входе (ADR-019) пароля нет, роль админа даёт провайдер, а человек заводится сам при
// первом входе без проектов; заранее заведённый по логину привяжется при входе.
const { t } = useI18n()
const external = isExternalAuth()
const auth = useAuth()
const toast = useToast()
const { loading, run } = useApiAction()

const items = ref<UsrMain[]>([])
const orgs = ref<OrgMain[]>([])
const projects = ref<ProjectMain[]>([])
const error = ref('')

async function load() {
  error.value = ''
  try {
    const [u, o, p] = await Promise.all([
      usrApi.list({ page: 0, page_size: 100 }),
      orgApi.list(),
      projectApi.listAll(),
    ])
    items.value = u.results
    orgs.value = o.results
    projects.value = p.results
  }
  catch (e) {
    error.value = apiErrorMessage(e)
  }
}
onMounted(load)

const projectById = computed(() => new Map(projects.value.map(p => [p.id, p])))

function projectLabel(id: string): string {
  const p = projectById.value.get(id)
  return p ? (p.name || p.slug) : t('users.projects.deleted', { id })
}

// подпись в таблице: «Все проекты», «Нет проектов» или имена через запятую
function projectsSummary(u: UsrMain): string {
  if (u.is_admin || u.all_projects)
    return t('users.projects.all')
  if (!u.project_ids?.length)
    return t('users.projects.none')
  return u.project_ids.map(projectLabel).join(', ')
}

function projectsMuted(u: UsrMain): boolean {
  return u.is_admin || !!u.all_projects || !u.project_ids?.length
}

const modal = ref(false)
const editing = ref<UsrMain | null>(null)
const name = ref('')
const username = ref('')
const password = ref('')
// сгенерированный пароль показан открыто: админ должен видеть, что передаёт
const passwordRevealed = ref(false)
const isAdmin = ref(false)
const active = ref(true)
const projectIds = ref<string[]>([])
const allProjects = ref(false)
const confirm = ref<UsrMain | null>(null)

// пункты выбора по организациям; проекты, удалённые после назначения, видны отдельно — иначе они
// исчезли бы из списка без следа
const projectItems = computed<SelectMenuItem[][]>(() => {
  const groups = orgs.value
    .map(o => [
      { type: 'label' as const, label: o.name || o.slug } as SelectMenuItem,
      ...projects.value
        .filter(p => p.org_id === o.id)
        .map(p => ({ label: p.name || p.slug, value: p.id })),
    ])
    .filter(g => g.length > 1)
  const stale = projectIds.value.filter(id => !projectById.value.has(id))
  if (stale.length)
    groups.push(stale.map(id => ({ label: t('users.projects.deleted', { id }), value: id })))
  return groups
})

function openCreate() {
  editing.value = null
  name.value = ''
  username.value = ''
  password.value = ''
  passwordRevealed.value = false
  isAdmin.value = false
  active.value = true
  projectIds.value = []
  allProjects.value = false
  modal.value = true
}

function openEdit(u: UsrMain) {
  editing.value = u
  name.value = u.name
  username.value = u.username
  password.value = ''
  passwordRevealed.value = false
  isAdmin.value = u.is_admin
  active.value = u.active
  projectIds.value = [...(u.project_ids ?? [])]
  allProjects.value = !!u.all_projects
  modal.value = true
}

function fillGeneratedPassword() {
  password.value = generatePassword()
  passwordRevealed.value = true
}

watch(password, (v) => {
  // стёрли пароль — поле снова скрытое
  if (!v)
    passwordRevealed.value = false
})

const isSelf = computed(() => !!editing.value && editing.value.id === auth.user.value?.id)
const canSave = computed(() => !!username.value.trim() && (external || !!editing.value || !!password.value))
// логин привязанного человека приходит от провайдера
const usernameLocked = computed(() => external && !!editing.value?.external)
const hasStale = computed(() => projectIds.value.some(id => !projectById.value.has(id)))

async function save(): Promise<boolean> {
  const common = {
    name: name.value.trim(),
    active: active.value,
    all_projects: allProjects.value,
    ...(usernameLocked.value ? {} : { username: username.value.trim() }),
    ...(external ? {} : { is_admin: isAdmin.value }),
    ...(password.value && !external ? { password: password.value } : {}),
  }
  const ok = await run(async () => {
    if (editing.value)
      await usrApi.update(editing.value.id, { ...common, project_ids: { values: projectIds.value } })
    else
      await usrApi.create({ ...common, project_ids: projectIds.value })
    return true
  }, { success: t('common.saved') })
  if (ok) {
    modal.value = false
    await load()
  }
  return !!ok
}

// адрес интерфейса, логин и пароль одним сообщением — чтобы переслать в мессенджер
function credentialsMessage(): string {
  const siteUrl = new URL(useRuntimeConfig().app.baseURL || '/', window.location.origin).href
  return [
    t('users.copy.site', { url: siteUrl }),
    t('users.copy.username', { username: username.value.trim().toLowerCase() }),
    t('users.copy.password', { password: password.value }),
  ].join('\n')
}

// Буфер пишется до запроса: после закрытия окна пароля уже не увидеть, поэтому человек не
// сохраняется с паролем, который не удалось скопировать.
async function saveAndCopy() {
  try {
    await navigator.clipboard.writeText(credentialsMessage())
  }
  catch {
    toast.add({ title: t('users.copy.failed'), color: 'error' })
    return
  }
  if (await save())
    toast.add({ title: t('users.copy.done'), color: 'success' })
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
        <table class="w-full min-w-[640px] text-sm">
          <thead class="bg-elevated/50 text-left text-xs text-muted">
            <tr>
              <th class="px-4 py-2 font-medium">{{ t('usr.field.name') }}</th>
              <th class="px-4 py-2 font-medium">{{ t('usr.field.username') }}</th>
              <th class="px-4 py-2 font-medium">{{ t('users.role') }}</th>
              <th class="px-4 py-2 font-medium">{{ t('users.projects.title') }}</th>
              <th class="px-4 py-2 font-medium">{{ t('users.status') }}</th>
              <th class="px-4 py-2" />
            </tr>
          </thead>
          <tbody class="divide-y divide-default">
            <tr v-for="u in items" :key="u.id" :class="u.active ? '' : 'opacity-60'">
              <td class="px-4 py-2 text-highlighted">{{ u.name }}</td>
              <td class="px-4 py-2 font-mono text-xs">
                {{ u.username }}
                <UBadge v-if="u.external" color="neutral" variant="outline" size="sm" class="ms-1 font-sans">{{ t('users.external') }}</UBadge>
              </td>
              <td class="px-4 py-2">
                <UBadge :color="u.is_admin ? 'primary' : 'neutral'" variant="subtle" size="sm">{{ u.is_admin ? t('usr.role.admin') : t('usr.role.member') }}</UBadge>
              </td>
              <td class="max-w-64 truncate px-4 py-2" :class="projectsMuted(u) ? 'text-muted' : ''" :title="projectsSummary(u)">
                {{ projectsSummary(u) }}
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
            <UFormField
              :label="t('usr.field.username')"
              :hint="external ? undefined : t('usr.usernameHint')"
              :description="usernameLocked ? t('users.externalUsername') : external && !editing ? t('users.externalPrecreate') : undefined"
            >
              <UInput v-model="username" autocomplete="off" class="w-full font-mono" :disabled="usernameLocked" />
            </UFormField>
            <UFormField
              v-if="!external"
              :label="editing ? t('users.newPassword') : t('users.tempPassword')"
              :hint="t('usr.passwordHint')"
              :description="password ? t('users.copy.hint') : undefined"
            >
              <UFieldGroup class="w-full">
                <UInput
                  v-model="password"
                  :type="passwordRevealed ? 'text' : 'password'"
                  autocomplete="new-password"
                  class="w-full"
                  :class="passwordRevealed ? 'font-mono' : ''"
                />
                <UButton color="neutral" variant="outline" icon="i-lucide-dices" :label="t('users.generate')" @click="fillGeneratedPassword" />
              </UFieldGroup>
            </UFormField>
            <UFormField :label="t('usr.role.admin')" :description="external ? t('users.externalAdmin') : isSelf ? t('users.selfHint') : t('users.adminHint')">
              <USwitch v-model="isAdmin" :disabled="isSelf || external" />
            </UFormField>
            <UFormField :label="t('users.projects.all')" :description="isAdmin ? t('users.projects.adminHint') : t('users.projects.allHint')">
              <USwitch v-model="allProjects" :disabled="isAdmin" />
            </UFormField>
            <UFormField
              :label="t('users.projects.title')"
              :description="isAdmin || allProjects ? undefined : t('users.projects.hint')"
              :error="!isAdmin && !allProjects && hasStale ? t('users.projects.staleHint') : undefined"
            >
              <USelectMenu
                v-model="projectIds"
                :items="projectItems"
                value-key="value"
                multiple
                :disabled="isAdmin || allProjects"
                icon="i-lucide-folder"
                :placeholder="isAdmin || allProjects ? t('users.projects.all') : t('users.projects.none')"
                class="w-full"
              />
            </UFormField>
            <UFormField :label="t('users.active')">
              <USwitch v-model="active" :disabled="isSelf" />
            </UFormField>
          </form>
        </template>
        <template #footer>
          <div class="flex w-full justify-end gap-2">
            <UButton color="neutral" variant="ghost" :label="t('common.cancel')" @click="modal = false" />
            <UButton
              type="submit"
              form="usr-form"
              :color="password ? 'neutral' : 'primary'"
              :variant="password ? 'outline' : 'solid'"
              :label="t('common.save')"
              :loading="loading"
              :disabled="!canSave"
            />
            <UButton
              v-if="password"
              icon="i-lucide-copy"
              :label="t('users.copy.button')"
              :loading="loading"
              :disabled="!canSave"
              @click="saveAndCopy"
            />
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
