<script setup lang="ts">
import type { ReplayFilter, ReplaySummary } from '~/types/replay'

// Записи сессий организации: фильтры — в адресе страницы. Нужен прежде всего для режима
// «часть всех сессий»: там есть записи без ошибок; записи с ошибками видны и в карточке проблемы.
const { t } = useI18n()
const org = useOrg()

const PAGE_SIZE = 25
const { state: filters, reset, dirty } = useQueryFilters({
  project: [] as string[],
  q: '',
  errors: '',
  type: '',
  page: '0',
})

const filter = computed<ReplayFilter>(() => ({
  orgId: org.currentId.value,
  projectIds: filters.project,
  environment: '',
  q: filters.q,
  errors: filters.errors as ReplayFilter['errors'],
  type: filters.type as ReplayFilter['type'],
}))

const items = ref<ReplaySummary[]>([])
const total = ref(0)
const loading = ref(false)
const error = ref('')
const loadedAt = ref('')

let controller: AbortController | null = null
async function load() {
  if (!org.currentId.value)
    return
  controller?.abort()
  controller = new AbortController()
  loading.value = true
  error.value = ''
  try {
    const rep = await replayApi.list(filter.value, Number(filters.page) || 0, PAGE_SIZE, controller.signal)
    items.value = rep.results
    total.value = Number(rep.pagination_info.total_count)
    loadedAt.value = new Date().toISOString()
  }
  catch (e) {
    if ((e as Error).name === 'AbortError')
      return
    error.value = apiErrorMessage(e)
  }
  finally {
    loading.value = false
  }
}

watch(() => [org.currentId.value, filters.project.join(','), filters.q, filters.errors, filters.type, filters.page], load, { immediate: true })
watch(() => [org.currentId.value, filters.project.join(','), filters.q, filters.errors, filters.type], () => {
  filters.page = '0'
})

const projectItems = computed(() => org.projects.value.map(p => ({ label: p.name || p.slug, value: p.id })))
// у пункта выбора не может быть пустого значения — «любые» помечены служебным
const ANY = '__any'
const errorsModel = computed({
  get: () => filters.errors || ANY,
  set: (v: string) => {
    filters.errors = v === ANY ? '' : v
  },
})
const typeModel = computed({
  get: () => filters.type || ANY,
  set: (v: string) => {
    filters.type = v === ANY ? '' : v
  },
})
const errorsItems = computed(() => [
  { label: t('replay.filter.anyErrors'), value: ANY },
  { label: t('replay.filter.withErrors'), value: 'yes' },
  { label: t('replay.filter.withoutErrors'), value: 'no' },
])
const typeItems = computed(() => [
  { label: t('replay.filter.anyType'), value: ANY },
  { label: t('replay.type.buffer'), value: 'buffer' },
  { label: t('replay.type.session'), value: 'session' },
])

const q = ref(filters.q)
watch(() => filters.q, (v) => {
  q.value = v
})
function search() {
  filters.q = q.value.trim()
}

const page = computed({
  get: () => (Number(filters.page) || 0) + 1,
  set: (v: number) => {
    filters.page = String(v - 1)
  },
})
const filtered = computed(() => !!(filters.q || filters.errors || filters.type || filters.project.length))
</script>

<template>
  <UDashboardPanel id="replays">
    <template #header>
      <UDashboardNavbar :title="t('replay.listTitle')">
        <template #leading>
          <UDashboardSidebarCollapse />
        </template>
        <template #right>
          <span v-if="loadedAt" class="hidden text-xs text-dimmed sm:inline">{{ t('common.loadedAt', { time: formatTime(loadedAt) }) }}</span>
          <UButton icon="i-lucide-refresh-cw" color="neutral" variant="ghost" :loading="loading" :aria-label="t('common.refresh')" @click="load" />
        </template>
      </UDashboardNavbar>
    </template>

    <template #body>
      <div class="space-y-3">
        <div class="flex flex-wrap items-center gap-2">
          <USelectMenu v-model="filters.project" :items="projectItems" value-key="value" multiple icon="i-lucide-folder" :placeholder="t('issues.filter.allProjects')" class="w-52" />
          <USelect v-model="errorsModel" :items="errorsItems" icon="i-lucide-bug" class="w-48" />
          <USelect v-model="typeModel" :items="typeItems" icon="i-lucide-film" class="w-56" />
          <form class="flex min-w-64 flex-1 gap-2" @submit.prevent="search">
            <UInput v-model="q" icon="i-lucide-search" :placeholder="t('replay.filter.search')" class="flex-1" />
          </form>
          <UButton v-if="dirty" color="neutral" variant="link" :label="t('common.reset')" @click="reset" />
        </div>

        <LoadAlert :title="t('replay.listLoadError')" :error="error" @retry="load" />

        <EmptyState
          v-if="!items.length && !loading && !error"
          icon="i-lucide-video"
          :title="filtered ? t('replay.empty.filtered') : t('replay.empty.title')"
          :description="filtered ? '' : t('replay.empty.description')"
        >
          <UButton v-if="!filtered" to="/settings" :label="t('replay.empty.toSettings')" variant="soft" />
        </EmptyState>

        <ReplayTable v-if="items.length" :items="items" show-project />

        <div v-if="total > PAGE_SIZE" class="flex justify-center">
          <UPagination v-model:page="page" :total="total" :items-per-page="PAGE_SIZE" />
        </div>
      </div>
    </template>
  </UDashboardPanel>
</template>
