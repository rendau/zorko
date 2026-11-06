<script setup lang="ts">
import type { ProjectKeyMain, ProjectMain } from '~/types/project'

// Проект: ключи с DSN и готовым кодом подключения SDK
const { t } = useI18n()
const route = useRoute()
const auth = useAuth()
const org = useOrg()
const { loading: removing, run } = useApiAction()
const id = computed(() => String(route.params.id))

const project = ref<ProjectMain | null>(null)
const keys = ref<ProjectKeyMain[]>([])
const error = ref('')

async function load() {
  error.value = ''
  try {
    const [p, k] = await Promise.all([projectApi.get(id.value), projectKeyApi.list(id.value)])
    project.value = p
    keys.value = k.results
  }
  catch (e) {
    error.value = apiErrorMessage(e)
  }
}
watch(id, load, { immediate: true })

const keyModal = ref(false)
const keyEditing = ref<ProjectKeyMain | null>(null)
const confirmKey = ref<ProjectKeyMain | null>(null)

function createKey() {
  keyEditing.value = null
  keyModal.value = true
}

function editKey(k: ProjectKeyMain) {
  keyEditing.value = k
  keyModal.value = true
}

async function removeKey() {
  const k = confirmKey.value
  if (k && await run(() => projectKeyApi.remove(k.id), { success: t('common.deleted') }) !== undefined) {
    confirmKey.value = null
    await load()
  }
}

const snippetTab = ref('vue')
const firstDsn = computed(() => keys.value.find(k => k.active)?.dsn ?? keys.value[0]?.dsn ?? '')
// запись сессий в примере — по режиму проекта (сохранённому, а не выбранному в форме)
const snippets = computed(() => sdkSnippets(firstDsn.value, {
  mode: project.value?.replay_mode || 'off',
  rate: project.value?.replay_session_rate ?? 0,
}))
const snippetTabs = computed(() => [
  ...snippets.value.map(s => ({ label: s.label, value: s.id })),
  { label: t('settings.sdk.build'), value: 'build' },
])
const snippet = computed(() => snippets.value.find(s => s.id === snippetTab.value) ?? snippets.value[0])
const orgSlug = computed(() => org.orgs.value.find(o => o.id === project.value?.org_id)?.slug ?? '')
const builds = computed(() => buildSnippets(ingestUrl(firstDsn.value), orgSlug.value, project.value?.slug ?? ''))
const buildTab = ref('vite')
const buildTabs = computed(() => builds.value.map(b => ({ label: b.label, value: b.id })))
const build = computed(() => builds.value.find(b => b.id === buildTab.value) ?? builds.value[0]!)

// #maps из плашки карточки проблемы — прокрутить к картам
onMounted(() => {
  void org.load()
})

// #maps из плашки карточки проблемы — прокрутить к картам, когда страница отрисована
const unwatchHash = watch(project, async (p) => {
  if (!p || route.hash !== '#maps')
    return
  unwatchHash()
  await nextTick()
  document.getElementById('maps')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
})

watch(project, (p) => {
  if (p && snippets.value.some(s => s.id === p.platform))
    snippetTab.value = p.platform
  if (p?.platform === 'nuxt')
    buildTab.value = 'nuxt'
  if (p?.platform === 'javascript')
    snippetTab.value = 'browser'
})
</script>

<template>
  <UDashboardPanel id="project">
    <template #header>
      <UDashboardNavbar>
        <template #leading>
          <UDashboardSidebarCollapse />
          <UButton icon="i-lucide-arrow-left" color="neutral" variant="ghost" to="/settings" :aria-label="t('common.back')" />
        </template>
        <template #title>
          <span class="truncate">{{ project ? (project.name || project.slug) : t('settings.project.title') }}</span>
        </template>
        <template #right>
          <UButton v-if="auth.isAdmin.value" icon="i-lucide-plus" size="sm" :label="t('settings.key.create')" @click="createKey" />
        </template>
      </UDashboardNavbar>
    </template>

    <template #body>
      <LoadAlert :title="t('settings.project.loadError')" :error="error" @retry="load" />

      <div v-if="project" class="space-y-6">
        <section class="space-y-3">
          <h3 class="text-sm font-semibold text-highlighted">{{ t('settings.key.title') }}</h3>
          <EmptyState v-if="!keys.length" icon="i-lucide-key-round" :title="t('settings.key.empty')" />
          <UCard v-for="k in keys" :key="k.id" :ui="{ body: 'p-4 sm:p-4 space-y-3' }">
            <div class="flex flex-wrap items-center gap-2">
              <span class="font-medium text-highlighted">{{ k.label || t('settings.key.noLabel') }}</span>
              <UBadge :color="k.active ? 'success' : 'neutral'" variant="subtle" size="sm">{{ k.active ? t('settings.key.on') : t('settings.key.off') }}</UBadge>
              <span class="text-xs text-dimmed">{{ k.rate_limit_per_sec ? t('settings.key.limit', { n: k.rate_limit_per_sec }) : t('settings.key.noLimit') }}</span>
              <div v-if="auth.isAdmin.value" class="ms-auto flex gap-1">
                <UButton size="xs" color="neutral" variant="ghost" icon="i-lucide-pencil" :aria-label="t('common.edit')" @click="editKey(k)" />
                <UButton size="xs" color="error" variant="ghost" icon="i-lucide-trash-2" :aria-label="t('common.delete')" @click="confirmKey = k" />
              </div>
            </div>
            <div>
              <div class="mb-1 text-xs text-muted">DSN</div>
              <div class="flex items-center gap-1 rounded-md border border-default bg-elevated/40 px-2 py-1">
                <code class="min-w-0 flex-1 truncate font-mono text-xs">{{ k.dsn || t('settings.key.noDsn') }}</code>
                <CopyButton v-if="k.dsn" :text="k.dsn" />
              </div>
            </div>
            <div class="text-xs text-muted">
              {{ t('settings.key.origins') }}:
              <span v-if="k.allowed_origin.length" class="font-mono text-default">{{ k.allowed_origin.join(', ') }}</span>
              <span v-else>{{ t('settings.key.anyOrigin') }}</span>
            </div>
          </UCard>
        </section>

        <section v-if="firstDsn" class="space-y-3">
          <h3 class="text-sm font-semibold text-highlighted">{{ t('settings.sdk.title') }}</h3>
          <p class="text-xs text-muted">{{ t('settings.sdk.description') }}</p>
          <UTabs v-model="snippetTab" :items="snippetTabs" :content="false" variant="link" size="sm" />
          <div v-if="snippetTab === 'build'" class="space-y-2">
            <p class="text-xs text-muted">{{ t('settings.sdk.buildHint') }}</p>
            <UTabs v-model="buildTab" :items="buildTabs" :content="false" variant="pill" size="xs" class="w-fit" />
            <div v-for="cmd in build.install" :key="cmd" class="flex items-center gap-1 rounded-md border border-default bg-elevated/40 px-3 py-1.5">
              <code class="min-w-0 flex-1 truncate font-mono text-xs">{{ cmd }}</code>
              <CopyButton :text="cmd" />
            </div>
            <div class="relative">
              <pre class="overflow-x-auto rounded-md border border-default bg-elevated/40 p-3 font-mono text-xs">{{ build.code }}</pre>
              <CopyButton :text="build.code" class="absolute right-2 top-2" />
            </div>
            <p v-if="build.id === 'nuxt'" class="text-xs text-muted">{{ t('settings.sdk.buildNuxt') }}</p>
            <p class="text-xs text-muted">{{ t('settings.sdk.buildTrouble') }}</p>
            <template v-if="build.cli">
              <p class="text-xs text-muted">{{ t('settings.sdk.buildCli') }}</p>
              <div class="relative">
                <pre class="overflow-x-auto rounded-md border border-default bg-elevated/40 p-3 font-mono text-xs">{{ build.cli }}</pre>
                <CopyButton :text="build.cli" class="absolute right-2 top-2" />
              </div>
            </template>
          </div>
          <div v-else-if="snippet" class="space-y-2">
            <div v-for="cmd in snippet.install" :key="cmd" class="flex items-center gap-1 rounded-md border border-default bg-elevated/40 px-3 py-1.5">
              <code class="min-w-0 flex-1 truncate font-mono text-xs">{{ cmd }}</code>
              <CopyButton :text="cmd" />
            </div>
            <p v-if="snippet.id === 'nuxt'" class="text-xs text-muted">{{ t('settings.sdk.nuxtHint') }}</p>
            <div class="relative">
              <pre class="overflow-x-auto rounded-md border border-default bg-elevated/40 p-3 font-mono text-xs">{{ snippet.code }}</pre>
              <CopyButton :text="snippet.code" class="absolute right-2 top-2" />
            </div>
          </div>
        </section>

        <section id="replay" class="scroll-mt-4 space-y-3">
          <h3 class="text-sm font-semibold text-highlighted">{{ t('settings.replay.title') }}</h3>
          <p class="text-xs text-muted">{{ t('settings.replay.description') }}</p>
          <SettingsReplayForm :project="project" :is-admin="auth.isAdmin.value" @saved="load" />
        </section>

        <section id="maps" class="scroll-mt-4 space-y-3">
          <h3 class="text-sm font-semibold text-highlighted">{{ t('settings.maps.title') }}</h3>
          <p class="text-xs text-muted">{{ t('settings.maps.description') }}</p>
          <SettingsBundleList :project-id="id" :is-admin="auth.isAdmin.value" />
        </section>

        <section class="space-y-3">
          <h3 class="text-sm font-semibold text-highlighted">{{ t('settings.siteFetch.title') }}</h3>
          <p class="text-xs text-muted">{{ t('settings.siteFetch.description') }}</p>
          <SettingsSiteFetchForm :project="project" :is-admin="auth.isAdmin.value" @saved="load" />
        </section>
      </div>

      <SettingsKeyModal v-model:open="keyModal" :project-key="keyEditing" :project-id="id" @saved="load" />
      <ConfirmModal
        :open="!!confirmKey"
        :title="t('settings.key.deleteTitle')"
        :description="t('settings.key.deleteDescription')"
        :loading="removing"
        @update:open="v => { if (!v) confirmKey = null }"
        @confirm="removeKey"
      />
    </template>
  </UDashboardPanel>
</template>
