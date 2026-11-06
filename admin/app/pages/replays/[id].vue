<script setup lang="ts">
import type { ReplayItem, RrwebEvent } from '~/utils/replay'
import type { ReplayGetRep } from '~/types/replay'

// Запись сессии: проигрыватель, шкала с ошибками, вкладки консоли, запросов и действий.
// ?project= — проект записи (номер записи придумывает браузер, доступ — по проекту);
// ?at= — время ошибки: проигрыватель встаёт за 5 с до неё.
const { t } = useI18n()
const route = useRoute()
const org = useOrg()
const id = computed(() => String(route.params.id))
const projectId = computed(() => String(route.query.project ?? ''))
const at = computed(() => (typeof route.query.at === 'string' ? Date.parse(route.query.at) : Number.NaN))

const rep = ref<ReplayGetRep | null>(null)
const events = ref<RrwebEvent[]>([])
const items = ref<ReplayItem[]>([])
const error = ref('')
const notFound = ref(false)
const loading = ref(false)
const loaded = ref(0)
const missing = ref(0)
const current = ref(0)
const player = ref<{ goto: (ms: number) => void } | null>(null)

const SEGMENT_CONCURRENCY = 6

let controller: AbortController | null = null
async function load() {
  controller?.abort()
  controller = new AbortController()
  const signal = controller.signal
  loading.value = true
  error.value = ''
  notFound.value = false
  events.value = []
  items.value = []
  loaded.value = 0
  missing.value = 0
  try {
    rep.value = await replayApi.get(projectId.value, id.value)
    const numbers = Array.from({ length: rep.value.summary.max_segment + 1 }, (_, i) => i)
    const parts = await mapLimit(numbers, SEGMENT_CONCURRENCY, async (n) => {
      try {
        const buf = await replayApi.segment(projectId.value, id.value, n, signal)
        return await decodeSegment(buf)
      }
      catch (e) {
        if ((e as Error).name === 'AbortError')
          throw e
        // сегмент не дошёл или уже убран — запись проигрывается без него
        missing.value++
        return []
      }
      finally {
        loaded.value++
      }
    })
    const all = parts.flat().sort((a, b) => a.timestamp - b.timestamp)
    items.value = replayItems(all)
    events.value = all
  }
  catch (e) {
    if ((e as Error).name === 'AbortError')
      return
    if (apiErrorCode(e) === 'replay_not_found')
      notFound.value = true
    else
      error.value = apiErrorMessage(e)
  }
  finally {
    loading.value = false
  }
}
watch(() => [id.value, projectId.value], load, { immediate: true })
onMounted(() => {
  void org.load()
})

const start = computed(() => events.value[0]?.timestamp ?? 0)
const playable = computed(() => events.value.some(e => e.type === RRWEB_FULL_SNAPSHOT) && events.value.length > 1)
const startAt = computed(() => (Number.isFinite(at.value) && start.value ? Math.max(at.value - start.value - 5000, 0) : 0))
const total = computed(() => (rep.value ? rep.value.summary.max_segment + 1 : 0))

function seek(offsetMs: number) {
  player.value?.goto(offsetMs)
}

const summary = computed(() => rep.value?.summary ?? null)
const user = computed(() => summary.value ? (summary.value.user_email || summary.value.user_id || summary.value.user_ip) : '')
</script>

<template>
  <UDashboardPanel id="replay">
    <template #header>
      <UDashboardNavbar>
        <template #leading>
          <UDashboardSidebarCollapse />
          <UButton icon="i-lucide-arrow-left" color="neutral" variant="ghost" :aria-label="t('common.back')" @click="$router.back()" />
        </template>
        <template #title>
          <span class="truncate">{{ t('replay.title') }}</span>
        </template>
        <template #right>
          <UButton icon="i-lucide-refresh-cw" color="neutral" variant="ghost" :loading="loading" :aria-label="t('common.refresh')" @click="load" />
        </template>
      </UDashboardNavbar>
    </template>

    <template #body>
      <LoadAlert :title="t('replay.loadError')" :error="error" @retry="load" />

      <EmptyState v-if="notFound" icon="i-lucide-video-off" :title="t('replay.notArrived.title')">
        <ul class="mx-auto max-w-lg list-disc space-y-1 ps-5 text-start text-sm text-muted">
          <li>{{ t('replay.notArrived.mode') }}</li>
          <li>{{ t('replay.notArrived.limit') }}</li>
          <li>{{ t('replay.notArrived.closed') }}</li>
          <li>{{ t('replay.notArrived.csp') }}</li>
          <li>{{ t('replay.notArrived.expired') }}</li>
        </ul>
      </EmptyState>

      <div v-if="summary" class="space-y-4">
        <div class="flex flex-wrap items-center gap-2 text-xs">
          <UBadge color="neutral" variant="outline" size="sm" icon="i-lucide-folder">{{ org.projectName(summary.project_id) }}</UBadge>
          <UBadge color="neutral" variant="outline" size="sm" :icon="summary.replay_type === 'session' ? 'i-lucide-film' : 'i-lucide-bug-play'">
            {{ summary.replay_type === 'session' ? t('replay.type.session') : t('replay.type.buffer') }}
          </UBadge>
          <span class="text-muted">{{ formatDateTime(summary.started_at) }} · {{ formatOffset(Number(summary.duration_ms)) }}</span>
          <span v-if="summary.browser || summary.os" class="text-muted">· {{ [summary.browser, summary.os].filter(Boolean).join(' · ') }}</span>
          <span v-if="summary.release" class="text-muted">· {{ summary.release }}</span>
          <span v-if="summary.environment" class="text-muted">· {{ summary.environment }}</span>
          <span class="text-muted">· {{ user || t('replay.anonymous') }}</span>
        </div>
        <p v-if="summary.url" class="break-all font-mono text-xs text-muted">{{ summary.url }}</p>

        <div v-if="loading && total" class="max-w-md space-y-1">
          <UProgress :model-value="loaded" :max="total" size="sm" />
          <p class="text-xs text-muted">{{ t('replay.loading', { n: loaded, total }) }}</p>
        </div>

        <UAlert v-if="!loading && missing" color="warning" variant="subtle" icon="i-lucide-triangle-alert" :description="t('replay.missing', { n: missing, total })" />
        <UAlert v-if="!loading && events.length && !playable" color="warning" variant="subtle" icon="i-lucide-triangle-alert" :description="t('replay.noSnapshot')" />

        <div v-if="!loading && playable" class="grid gap-4 xl:grid-cols-[1fr_24rem]">
          <ReplayPlayer ref="player" :events="events" :start-at="startAt" @time="v => current = v" />
          <ReplayItems :items="items" :errors="rep?.errors ?? []" :start="start" :current="current" @seek="seek" />
        </div>
      </div>
      <USkeleton v-else-if="loading" class="h-96 w-full" />
    </template>
  </UDashboardPanel>
</template>
