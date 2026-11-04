<script setup lang="ts">
import type { IssueScope } from '~/api/issue.api'
import type { IssueListItem, IssueSort, Period } from '~/types/issue'

// Список проблем (как Issues в Sentry): фильтры и поиск — в адресе страницы, ссылку можно переслать
const { t } = useI18n()
const org = useOrg()

const PAGE_SIZE = 25
const { state: filters, reset, dirty } = useQueryFilters({
  project: [] as string[],
  env: '',
  period: '14d',
  q: '',
  sort: 'last_seen',
  page: '0',
})

const scope = computed<IssueScope>(() => ({
  orgId: org.currentId.value,
  projectIds: filters.project,
  environment: filters.env,
  period: filters.period as Period,
}))

const items = ref<IssueListItem[]>([])
const total = ref(0)
const truncated = ref(false)
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
    const rep = await issueApi.list(scope.value, filters.q, filters.sort as IssueSort, Number(filters.page) || 0, PAGE_SIZE, controller.signal)
    items.value = rep.results
    total.value = Number(rep.pagination_info.total_count)
    truncated.value = rep.truncated
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

watch(() => [org.currentId.value, filters.project.join(','), filters.env, filters.period, filters.q, filters.sort, filters.page], load, { immediate: true })

// новый фильтр — с первой страницы
watch(() => [filters.project.join(','), filters.env, filters.period, filters.q, filters.sort, org.currentId.value], () => {
  filters.page = '0'
})

const environments = ref<string[]>([])
watch(() => [org.currentId.value, filters.period], async () => {
  if (!org.currentId.value)
    return
  try {
    environments.value = (await tagApi.values({ ...scope.value, environment: '' }, 'environment', '')).values
  }
  catch {
    environments.value = []
  }
}, { immediate: true })

const projectItems = computed(() => org.projects.value.map(p => ({ label: p.name || p.slug, value: p.id })))
// у пункта выбора не может быть пустого значения — «все окружения» помечены служебным
const ALL_ENVIRONMENTS = '__all'
const envModel = computed({
  get: () => filters.env || ALL_ENVIRONMENTS,
  set: (v: string) => {
    filters.env = v === ALL_ENVIRONMENTS ? '' : v
  },
})
const envItems = computed(() => [{ label: t('issues.filter.allEnvironments'), value: ALL_ENVIRONMENTS }, ...environments.value.map(e => ({ label: e, value: e }))])
const periodItems = computed(() => (['1h', '24h', '7d', '14d', '30d'] as Period[]).map(p => ({ label: t(`period.${p}`), value: p as string })))
const sortItems = computed(() => (['last_seen', 'first_seen', 'events', 'users'] as IssueSort[]).map(s => ({ label: t(`issues.sort.${s}`), value: s as string })))

const page = computed({
  get: () => (Number(filters.page) || 0) + 1,
  set: (v: number) => {
    filters.page = String(v - 1)
  },
})
</script>

<template>
  <UDashboardPanel id="issues">
    <template #header>
      <UDashboardNavbar :title="t('issues.title')">
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
          <USelectMenu
            v-model="filters.project"
            :items="projectItems"
            value-key="value"
            multiple
            icon="i-lucide-folder"
            :placeholder="t('issues.filter.allProjects')"
            class="w-52"
          />
          <USelect v-model="envModel" :items="envItems" icon="i-lucide-server" class="w-52" />
          <USelect v-model="filters.period" :items="periodItems" icon="i-lucide-calendar-clock" class="w-40" />
          <USelect v-model="filters.sort" :items="sortItems" icon="i-lucide-arrow-down-wide-narrow" class="w-48" />
          <UButton v-if="dirty" color="neutral" variant="link" :label="t('common.reset')" @click="reset" />
        </div>

        <SearchBar v-if="org.currentId.value" v-model="filters.q" :scope="scope" />

        <LoadAlert :title="t('issues.loadError')" :error="error" @retry="load" />

        <UAlert v-if="truncated" color="warning" variant="subtle" icon="i-lucide-triangle-alert" :description="t('issues.truncated')" />

        <EmptyState
          v-if="!org.orgs.value.length && !loading"
          icon="i-lucide-building-2"
          :title="t('issues.empty.noOrg')"
        >
          <UButton to="/settings" :label="t('issues.empty.toSettings')" variant="soft" />
        </EmptyState>

        <EmptyState
          v-else-if="!items.length && !loading && !error"
          icon="i-lucide-party-popper"
          :title="filters.q || filters.env || filters.project.length ? t('issues.empty.filtered') : t('issues.empty.title')"
          :description="filters.q || filters.env || filters.project.length ? '' : t('issues.empty.description')"
        >
          <UButton v-if="!filters.q && !filters.env && !filters.project.length" to="/settings" :label="t('issues.empty.connect')" variant="soft" />
        </EmptyState>

        <div v-if="items.length" class="overflow-x-auto rounded-lg border border-default">
          <div class="min-w-[720px] divide-y divide-default">
            <div class="grid grid-cols-[1fr_9rem_5rem_5rem] items-center gap-4 bg-elevated/50 px-4 py-2 text-xs font-medium text-muted">
              <span>{{ t('issues.column.issue') }}</span>
              <span>{{ t('issues.column.graph') }}</span>
              <span class="text-right">{{ t('issues.column.events') }}</span>
              <span class="text-right">{{ t('issues.column.users') }}</span>
            </div>
            <NuxtLink
              v-for="item in items"
              :key="item.issue.id"
              :to="{ path: `/issues/${item.issue.id}`, query: { period: filters.period, env: filters.env || undefined } }"
              class="grid grid-cols-[1fr_9rem_5rem_5rem] items-center gap-4 px-4 py-2.5 hover:bg-elevated/50"
            >
              <div class="flex min-w-0 gap-3">
                <span class="mt-1 h-8 w-1 shrink-0 rounded-full" :class="levelBarClass(item.issue.level)" />
                <div class="min-w-0">
                  <div class="truncate text-sm">
                    <span class="font-semibold text-highlighted">{{ item.issue.type || item.issue.title }}</span>
                    <span v-if="item.issue.type && item.issue.value" class="ms-1.5 text-default">{{ item.issue.value }}</span>
                  </div>
                  <div class="mt-0.5 flex flex-wrap items-center gap-x-3 gap-y-0.5 text-xs text-muted">
                    <span v-if="item.issue.culprit" class="truncate font-mono">{{ item.issue.culprit }}</span>
                    <span class="inline-flex items-center gap-1"><UIcon name="i-lucide-folder" class="size-3" />{{ org.projectName(item.issue.project_id) }}</span>
                    <span class="inline-flex items-center gap-1" :title="t('issues.lastSeen')"><UIcon name="i-lucide-clock" class="size-3" /><RelativeTime :value="item.issue.last_seen" /></span>
                    <span class="inline-flex items-center gap-1" :title="t('issues.firstSeen')"><UIcon name="i-lucide-sprout" class="size-3" /><RelativeTime :value="item.issue.first_seen" /></span>
                  </div>
                </div>
              </div>
              <SparkBars :values="item.sparkline" />
              <span class="text-right text-sm tabular-nums text-highlighted">{{ compactNumber(item.events) }}</span>
              <span class="text-right text-sm tabular-nums">{{ compactNumber(item.users) }}</span>
            </NuxtLink>
          </div>
        </div>

        <div v-if="total > PAGE_SIZE" class="flex justify-center">
          <UPagination v-model:page="page" :total="total" :items-per-page="PAGE_SIZE" />
        </div>
      </div>
    </template>
  </UDashboardPanel>
</template>
