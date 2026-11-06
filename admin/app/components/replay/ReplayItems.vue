<script setup lang="ts">
import type { ReplayItem } from '~/utils/replay'
import type { ReplayErrorMark } from '~/types/replay'

// Вкладки рядом с проигрывателем: ошибки, консоль, запросы, действия. Щелчок по строке — переход
// к этому моменту записи; строки, ещё не наступившие на шкале, приглушены.
const props = defineProps<{
  items: ReplayItem[]
  errors: ReplayErrorMark[]
  start: number // мс Unix первого события записи
  current: number // мс от начала записи
}>()
const emit = defineEmits<{ seek: [offsetMs: number] }>()
const { t } = useI18n()

const tab = ref('errors')
const consoleItems = computed(() => props.items.filter(i => i.kind === 'console'))
const networkItems = computed(() => props.items.filter(i => i.kind === 'network'))
const actionItems = computed(() => props.items.filter(i => i.kind === 'click' || i.kind === 'navigation' || i.kind === 'input'))

const tabs = computed(() => [
  { label: t('replay.tab.errors', { n: props.errors.length }), value: 'errors' },
  { label: t('replay.tab.console', { n: consoleItems.value.length }), value: 'console' },
  { label: t('replay.tab.network', { n: networkItems.value.length }), value: 'network' },
  { label: t('replay.tab.actions', { n: actionItems.value.length }), value: 'actions' },
])

watch(() => props.errors.length, (n) => {
  if (!n && tab.value === 'errors')
    tab.value = 'actions'
}, { immediate: true })

function offset(at: number): number {
  return at - props.start
}

function future(at: number): boolean {
  return offset(at) > props.current
}

function levelColor(level: string): 'error' | 'warning' | 'neutral' {
  return level === 'error' ? 'error' : level === 'warning' || level === 'warn' ? 'warning' : 'neutral'
}

function statusColor(status: number): 'error' | 'warning' | 'success' | 'neutral' {
  if (!status)
    return 'neutral'
  return status >= 500 ? 'error' : status >= 400 ? 'warning' : 'success'
}

const ACTION_ICON: Record<string, string> = {
  click: 'i-lucide-mouse-pointer-click',
  navigation: 'i-lucide-navigation',
  input: 'i-lucide-text-cursor-input',
}
</script>

<template>
  <div class="flex min-h-0 flex-col gap-2">
    <UTabs v-model="tab" :items="tabs" :content="false" variant="link" size="sm" />
    <div class="max-h-[70vh] min-h-48 overflow-y-auto rounded-lg border border-default text-xs">
      <ul v-if="tab === 'errors'" class="divide-y divide-default">
        <li v-for="e in errors" :key="e.event_id" class="flex items-start gap-2 px-3 py-2" :class="{ 'opacity-50': future(Date.parse(e.timestamp)) }">
          <button type="button" class="shrink-0 font-mono tabular-nums text-muted hover:text-highlighted" @click="emit('seek', offset(Date.parse(e.timestamp)))">
            {{ formatOffset(offset(Date.parse(e.timestamp))) }}
          </button>
          <NuxtLink :to="`/issues/${e.issue_id}`" class="min-w-0 flex-1 break-words text-error hover:underline">{{ e.title }}</NuxtLink>
        </li>
        <li v-if="!errors.length" class="px-3 py-4 text-center text-muted">{{ t('replay.empty.errors') }}</li>
      </ul>

      <ul v-else-if="tab === 'console'" class="divide-y divide-default">
        <li v-for="(c, i) in consoleItems" :key="i" class="cursor-pointer px-3 py-2 hover:bg-elevated/50" :class="{ 'opacity-50': future(c.at) }" @click="emit('seek', offset(c.at))">
          <div class="flex items-center gap-2">
            <span class="shrink-0 font-mono tabular-nums text-muted">{{ formatOffset(offset(c.at)) }}</span>
            <UBadge :color="levelColor(c.level)" variant="subtle" size="sm">{{ c.level }}</UBadge>
          </div>
          <div class="mt-1 whitespace-pre-wrap break-all font-mono">{{ c.title }}</div>
          <div v-if="c.detail" class="mt-0.5 whitespace-pre-wrap break-all font-mono text-muted">{{ c.detail }}</div>
        </li>
        <li v-if="!consoleItems.length" class="px-3 py-4 text-center text-muted">{{ t('replay.empty.console') }}</li>
      </ul>

      <ul v-else-if="tab === 'network'" class="divide-y divide-default">
        <li v-for="(n, i) in networkItems" :key="i" class="cursor-pointer px-3 py-2 hover:bg-elevated/50" :class="{ 'opacity-50': future(n.at) }" @click="emit('seek', offset(n.at))">
          <div class="flex items-center gap-2">
            <span class="shrink-0 font-mono tabular-nums text-muted">{{ formatOffset(offset(n.at)) }}</span>
            <span class="font-mono font-medium">{{ n.method }}</span>
            <UBadge :color="statusColor(n.status)" variant="subtle" size="sm">{{ n.status || '—' }}</UBadge>
            <span class="ms-auto tabular-nums text-muted">{{ t('replay.ms', { n: formatNumber(n.durationMs) }) }}</span>
          </div>
          <div class="mt-1 break-all font-mono">{{ n.title }}</div>
        </li>
        <li v-if="!networkItems.length" class="px-3 py-4 text-center text-muted">{{ t('replay.empty.network') }}</li>
      </ul>

      <ul v-else class="divide-y divide-default">
        <li v-for="(a, i) in actionItems" :key="i" class="flex cursor-pointer items-start gap-2 px-3 py-2 hover:bg-elevated/50" :class="{ 'opacity-50': future(a.at) }" @click="emit('seek', offset(a.at))">
          <span class="shrink-0 font-mono tabular-nums text-muted">{{ formatOffset(offset(a.at)) }}</span>
          <UIcon :name="ACTION_ICON[a.kind] ?? 'i-lucide-dot'" class="mt-0.5 size-3.5 shrink-0 text-muted" />
          <div class="min-w-0">
            <div class="break-all font-mono">{{ a.title }}</div>
            <div v-if="a.detail" class="break-words text-muted">{{ a.detail }}</div>
          </div>
        </li>
        <li v-if="!actionItems.length" class="px-3 py-4 text-center text-muted">{{ t('replay.empty.actions') }}</li>
      </ul>
    </div>
  </div>
</template>
