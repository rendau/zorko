<script setup lang="ts">
import type { ProjectMain } from '~/types/project'

// Организация и её проекты; менять может только админ
const { t } = useI18n()
const auth = useAuth()
const org = useOrg()
const { loading: removing, run } = useApiAction()

onMounted(() => {
  void org.load()
})

const orgModal = ref(false)
const orgEditing = ref(false)
const projectModal = ref(false)
const projectEditing = ref<ProjectMain | null>(null)
const confirmOrg = ref(false)
const confirmProject = ref<ProjectMain | null>(null)

function createOrg() {
  orgEditing.value = false
  orgModal.value = true
}

function editOrg() {
  orgEditing.value = true
  orgModal.value = true
}

async function orgSaved(id: string) {
  await org.load(true)
  await org.select(id)
}

function createProject() {
  projectEditing.value = null
  projectModal.value = true
}

function editProject(p: ProjectMain) {
  projectEditing.value = p
  projectModal.value = true
}

async function removeOrg() {
  const id = org.currentId.value
  if (await run(() => orgApi.remove(id), { success: t('common.deleted') }) !== undefined) {
    confirmOrg.value = false
    org.currentId.value = ''
    await org.load(true)
  }
}

async function removeProject() {
  const p = confirmProject.value
  if (p && await run(() => projectApi.remove(p.id), { success: t('common.deleted') }) !== undefined) {
    confirmProject.value = null
    await org.loadProjects()
  }
}
</script>

<template>
  <UDashboardPanel id="settings">
    <template #header>
      <UDashboardNavbar :title="t('settings.title')">
        <template #leading>
          <UDashboardSidebarCollapse />
        </template>
        <template #right>
          <UButton v-if="auth.isAdmin.value" icon="i-lucide-plus" color="neutral" variant="outline" :label="t('settings.org.create')" @click="createOrg" />
        </template>
      </UDashboardNavbar>
    </template>

    <template #body>
      <EmptyState v-if="!org.orgs.value.length" icon="i-lucide-building-2" :title="t('settings.org.empty')" :description="auth.isAdmin.value ? t('settings.org.emptyAdmin') : t('settings.org.emptyMember')">
        <UButton v-if="auth.isAdmin.value" icon="i-lucide-plus" :label="t('settings.org.create')" @click="createOrg" />
      </EmptyState>

      <div v-else-if="org.current.value" class="space-y-6">
        <div class="flex flex-wrap items-center gap-3">
          <UIcon name="i-lucide-building-2" class="size-5 text-muted" />
          <h2 class="text-base font-semibold text-highlighted">{{ org.current.value.name || org.current.value.slug }}</h2>
          <span class="font-mono text-xs text-dimmed">{{ org.current.value.slug }}</span>
          <template v-if="auth.isAdmin.value">
            <UButton size="xs" color="neutral" variant="ghost" icon="i-lucide-pencil" :label="t('common.edit')" @click="editOrg" />
            <UButton size="xs" color="error" variant="ghost" icon="i-lucide-trash-2" :label="t('common.delete')" @click="confirmOrg = true" />
          </template>
        </div>

        <div class="flex items-center justify-between gap-2">
          <h3 class="text-sm font-semibold text-highlighted">{{ t('settings.project.title') }}</h3>
          <UButton v-if="auth.isAdmin.value" icon="i-lucide-plus" size="sm" :label="t('settings.project.create')" @click="createProject" />
        </div>

        <EmptyState v-if="!org.projects.value.length" icon="i-lucide-folder" :title="t('settings.project.empty')" />

        <div v-else class="overflow-x-auto rounded-lg border border-default">
          <table class="w-full min-w-[560px] text-sm">
            <thead class="bg-elevated/50 text-left text-xs text-muted">
              <tr>
                <th class="px-4 py-2 font-medium">{{ t('settings.field.name') }}</th>
                <th class="px-4 py-2 font-medium">{{ t('settings.field.slug') }}</th>
                <th class="px-4 py-2 font-medium">{{ t('settings.field.platform') }}</th>
                <th class="px-4 py-2" />
              </tr>
            </thead>
            <tbody class="divide-y divide-default">
              <tr v-for="p in org.projects.value" :key="p.id">
                <td class="px-4 py-2">
                  <NuxtLink :to="`/settings/projects/${p.id}`" class="font-medium text-highlighted hover:text-primary">{{ p.name || p.slug }}</NuxtLink>
                </td>
                <td class="px-4 py-2 font-mono text-xs">{{ p.slug }}</td>
                <td class="px-4 py-2">{{ p.platform }}</td>
                <td class="px-4 py-2 text-right whitespace-nowrap">
                  <UButton size="xs" color="neutral" variant="ghost" icon="i-lucide-key-round" :label="t('settings.project.keys')" :to="`/settings/projects/${p.id}`" />
                  <template v-if="auth.isAdmin.value">
                    <UButton size="xs" color="neutral" variant="ghost" icon="i-lucide-pencil" :aria-label="t('common.edit')" @click="editProject(p)" />
                    <UButton size="xs" color="error" variant="ghost" icon="i-lucide-trash-2" :aria-label="t('common.delete')" @click="confirmProject = p" />
                  </template>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <SettingsOrgModal v-model:open="orgModal" :org="orgEditing ? org.current.value : null" @saved="orgSaved" />
      <SettingsProjectModal v-model:open="projectModal" :project="projectEditing" :org-id="org.currentId.value" @saved="org.loadProjects()" />
      <ConfirmModal
        v-model:open="confirmOrg"
        :title="t('settings.org.deleteTitle')"
        :description="t('settings.org.deleteDescription')"
        :loading="removing"
        @confirm="removeOrg"
      />
      <ConfirmModal
        :open="!!confirmProject"
        :title="t('settings.project.deleteTitle')"
        :description="t('settings.project.deleteDescription')"
        :loading="removing"
        @update:open="v => { if (!v) confirmProject = null }"
        @confirm="removeProject"
      />
    </template>
  </UDashboardPanel>
</template>
