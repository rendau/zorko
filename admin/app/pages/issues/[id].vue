<script setup lang="ts">
import type { EventMain, EventSummary } from '~/types/event'
import type { IssueGetRep, IssueStatPoint, Period, TagDist } from '~/types/issue'

// Карточка проблемы: шапка, график, теги, событие с листанием и таблица сохранённых событий
const { t } = useI18n()
const route = useRoute()
const org = useOrg()
const id = computed(() => String(route.params.id))

const { state: filters } = useQueryFilters({
  period: '14d',
  env: '',
  tab: 'event',
  event: 'latest',
  eq: '',
  epage: '0',
})

const issue = ref<IssueGetRep | null>(null)
const points = ref<IssueStatPoint[]>([])
const tags = ref<TagDist[]>([])
const event = ref<EventMain | null>(null)
const error = ref('')
const eventError = ref('')
const loading = ref(false)
const eventLoading = ref(false)

const interval = computed<'hour' | 'day'>(() => (['1h', '24h'].includes(filters.period) ? 'hour' : 'day'))

async function loadIssue() {
  loading.value = true
  error.value = ''
  try {
    const period = filters.period as Period
    const [i, s, tg] = await Promise.all([
      issueApi.get(id.value, period, filters.env),
      issueApi.stat(id.value, period, interval.value, filters.env),
      issueApi.tags(id.value, period, filters.env),
    ])
    issue.value = i
    points.value = s.points
    tags.value = tg.tags
  }
  catch (e) {
    error.value = apiErrorMessage(e)
  }
  finally {
    loading.value = false
  }
}

async function loadEvent() {
  eventLoading.value = true
  eventError.value = ''
  try {
    event.value = await eventApi.get(id.value, filters.event)
  }
  catch (e) {
    event.value = null
    eventError.value = apiErrorCode(e) === 'object_not_found' ? t('issue.eventGone') : apiErrorMessage(e)
  }
  finally {
    eventLoading.value = false
  }
}

watch(() => [id.value, filters.period, filters.env], loadIssue, { immediate: true })
watch(() => [id.value, filters.event], loadEvent, { immediate: true })

const periodItems = computed(() => (['1h', '24h', '7d', '14d', '30d'] as Period[]).map(p => ({ label: t(`period.${p}`), value: p as string })))

const tabs = computed(() => [
  { label: t('issue.tab.event'), value: 'event', icon: 'i-lucide-file-search' },
  { label: t('issue.tab.events'), value: 'events', icon: 'i-lucide-list' },
])

// клик по тегу — к таблице событий с этим условием
function filterByTag(key: string, value: string) {
  filters.eq = addCondition(filters.eq, key, value)
  filters.epage = '0'
  filters.tab = 'events'
}

// таблица событий
const EVENT_PAGE = 50
const events = ref<EventSummary[]>([])
const eventsTotal = ref(0)
const eventsError = ref('')
const eventsLoading = ref(false)

async function loadEvents() {
  if (filters.tab !== 'events')
    return
  eventsLoading.value = true
  eventsError.value = ''
  try {
    const rep = await eventApi.list(id.value, filters.eq, Number(filters.epage) || 0, EVENT_PAGE)
    events.value = rep.results
    eventsTotal.value = Number(rep.pagination_info.total_count)
  }
  catch (e) {
    eventsError.value = apiErrorMessage(e)
  }
  finally {
    eventsLoading.value = false
  }
}
watch(() => [id.value, filters.tab, filters.eq, filters.epage], loadEvents, { immediate: true })

const eventsPage = computed({
  get: () => (Number(filters.epage) || 0) + 1,
  set: (v: number) => {
    filters.epage = String(v - 1)
  },
})

function openEvent(eventId: string) {
  filters.event = eventId
  filters.tab = 'event'
}

watch(() => filters.eq, () => {
  filters.epage = '0'
})

const searchScope = computed(() => ({
  orgId: org.currentId.value,
  projectIds: issue.value ? [issue.value.issue.project_id] : [],
  environment: '',
  period: '30d' as Period,
}))
</script>

<template>
  <UDashboardPanel id="issue">
    <template #header>
      <UDashboardNavbar>
        <template #leading>
          <UDashboardSidebarCollapse />
          <UButton icon="i-lucide-arrow-left" color="neutral" variant="ghost" :to="{ path: '/issues', query: { period: filters.period, env: filters.env || undefined } }" :aria-label="t('common.back')" />
        </template>
        <template #title>
          <span class="truncate">{{ issue?.issue.title ?? t('issue.title') }}</span>
        </template>
        <template #right>
          <USelect v-model="filters.period" :items="periodItems" icon="i-lucide-calendar-clock" size="sm" class="w-40" />
          <UButton icon="i-lucide-refresh-cw" color="neutral" variant="ghost" :loading="loading" :aria-label="t('common.refresh')" @click="loadIssue(); loadEvent()" />
        </template>
      </UDashboardNavbar>
    </template>

    <template #body>
      <LoadAlert :title="t('issue.loadError')" :error="error" @retry="loadIssue" />

      <div v-if="issue" class="space-y-6">
        <header class="space-y-2">
          <div class="flex flex-wrap items-center gap-2">
            <LevelBadge :level="issue.issue.level" />
            <UBadge color="neutral" variant="outline" size="sm" icon="i-lucide-folder">{{ org.projectName(issue.issue.project_id) }}</UBadge>
            <UBadge v-if="issue.issue.platform" color="neutral" variant="outline" size="sm">{{ issue.issue.platform }}</UBadge>
          </div>
          <h1 class="break-words text-lg font-semibold text-highlighted">
            {{ issue.issue.type || issue.issue.title }}<span v-if="issue.issue.type && issue.issue.value" class="ms-2 font-normal text-default">{{ issue.issue.value }}</span>
          </h1>
          <p v-if="issue.issue.culprit" class="break-all font-mono text-sm text-muted">{{ issue.issue.culprit }}</p>
        </header>

        <div class="grid grid-cols-2 gap-3 md:grid-cols-4">
          <UCard :ui="{ body: 'p-3 sm:p-3' }">
            <div class="text-xs text-muted">{{ t('issue.stat.events', { period: t(`period.${filters.period}`) }) }}</div>
            <div class="text-xl font-semibold tabular-nums text-highlighted">{{ formatNumber(issue.events) }}</div>
            <div class="text-xs text-dimmed">{{ t('issue.stat.total', { n: formatNumber(issue.issue.times_seen) }) }}</div>
          </UCard>
          <UCard :ui="{ body: 'p-3 sm:p-3' }">
            <div class="text-xs text-muted">{{ t('issue.stat.users', { period: t(`period.${filters.period}`) }) }}</div>
            <div class="text-xl font-semibold tabular-nums text-highlighted">{{ formatNumber(issue.users) }}</div>
          </UCard>
          <UCard :ui="{ body: 'p-3 sm:p-3' }">
            <div class="text-xs text-muted">{{ t('issues.firstSeen') }}</div>
            <div class="text-sm font-medium text-highlighted"><RelativeTime :value="issue.issue.first_seen" /></div>
            <div v-if="issue.issue.first_release" class="truncate text-xs text-dimmed">{{ t('issue.release', { r: issue.issue.first_release }) }}</div>
          </UCard>
          <UCard :ui="{ body: 'p-3 sm:p-3' }">
            <div class="text-xs text-muted">{{ t('issues.lastSeen') }}</div>
            <div class="text-sm font-medium text-highlighted"><RelativeTime :value="issue.issue.last_seen" /></div>
            <div v-if="issue.issue.last_release" class="truncate text-xs text-dimmed">{{ t('issue.release', { r: issue.issue.last_release }) }}</div>
          </UCard>
        </div>

        <UCard :ui="{ body: 'p-3 sm:p-4' }">
          <BarChart :points="points" :interval="interval" />
        </UCard>

        <div class="grid gap-6 lg:grid-cols-[1fr_18rem]">
          <div class="min-w-0 space-y-4">
            <UTabs v-model="filters.tab" :items="tabs" :content="false" variant="link" />

            <template v-if="filters.tab === 'event'">
              <div class="flex flex-wrap items-center gap-2">
                <UButton size="sm" color="neutral" variant="outline" icon="i-lucide-chevrons-left" :label="t('issue.nav.oldest')" @click="filters.event = 'oldest'" />
                <UButton size="sm" color="neutral" variant="outline" icon="i-lucide-chevron-left" :label="t('issue.nav.older')" :disabled="!event?.prev_event_id" @click="event && (filters.event = event.prev_event_id)" />
                <UButton size="sm" color="neutral" variant="outline" trailing-icon="i-lucide-chevron-right" :label="t('issue.nav.newer')" :disabled="!event?.next_event_id" @click="event && (filters.event = event.next_event_id)" />
                <UButton size="sm" color="neutral" variant="outline" trailing-icon="i-lucide-chevrons-right" :label="t('issue.nav.latest')" @click="filters.event = 'latest'" />
                <span v-if="event" class="text-xs text-muted">
                  <span class="font-mono">{{ event.summary.event_id.slice(0, 8) }}</span> · {{ formatDateTime(event.summary.timestamp) }}
                  <template v-if="event.summary.release"> · {{ event.summary.release }}</template>
                </span>
              </div>
              <LoadAlert :title="t('issue.eventLoadError')" :error="eventError" @retry="loadEvent" />
              <USkeleton v-if="eventLoading && !event" class="h-64 w-full" />
              <EventDetail v-if="event" :event="event" @tag="filterByTag" />
            </template>

            <template v-else>
              <SearchBar v-model="filters.eq" :scope="searchScope" />
              <p class="text-xs text-muted">{{ t('issue.events.note') }}</p>
              <LoadAlert :title="t('issue.eventsLoadError')" :error="eventsError" @retry="loadEvents" />
              <div class="overflow-x-auto rounded-lg border border-default">
                <table class="w-full min-w-[640px] text-xs">
                  <thead class="bg-elevated/50 text-left text-muted">
                    <tr>
                      <th class="px-3 py-2 font-medium">{{ t('issue.events.col.time') }}</th>
                      <th class="px-3 py-2 font-medium">{{ t('issue.events.col.release') }}</th>
                      <th class="px-3 py-2 font-medium">{{ t('issue.events.col.browser') }}</th>
                      <th class="px-3 py-2 font-medium">{{ t('issue.events.col.os') }}</th>
                      <th class="px-3 py-2 font-medium">{{ t('issue.events.col.user') }}</th>
                      <th class="px-3 py-2 font-medium">{{ t('issue.events.col.url') }}</th>
                    </tr>
                  </thead>
                  <tbody class="divide-y divide-default">
                    <tr v-for="e in events" :key="e.event_id" class="cursor-pointer hover:bg-elevated/50" @click="openEvent(e.event_id)">
                      <td class="whitespace-nowrap px-3 py-1.5">{{ formatDateTime(e.timestamp) }}</td>
                      <td class="px-3 py-1.5">{{ e.release }}</td>
                      <td class="px-3 py-1.5">{{ e.browser }}</td>
                      <td class="px-3 py-1.5">{{ e.os }}</td>
                      <td class="max-w-40 truncate px-3 py-1.5">{{ e.user_id || e.user_email || e.user_ip }}</td>
                      <td class="max-w-64 truncate px-3 py-1.5 font-mono" :title="e.url">{{ e.url }}</td>
                    </tr>
                    <tr v-if="!events.length && !eventsLoading">
                      <td colspan="6" class="px-3 py-4 text-center text-muted">{{ t('issue.events.empty') }}</td>
                    </tr>
                  </tbody>
                </table>
              </div>
              <div v-if="eventsTotal > EVENT_PAGE" class="flex justify-center">
                <UPagination v-model:page="eventsPage" :total="eventsTotal" :items-per-page="EVENT_PAGE" />
              </div>
            </template>
          </div>

          <aside class="min-w-0">
            <h3 class="mb-3 text-sm font-semibold text-highlighted">{{ t('issue.tags') }}</h3>
            <TagBars v-if="tags.length" :tags="tags" @select="filterByTag" />
            <p v-else class="text-xs text-muted">{{ t('issue.noTags') }}</p>
          </aside>
        </div>
      </div>
      <USkeleton v-else-if="loading" class="h-96 w-full" />
    </template>
  </UDashboardPanel>
</template>
