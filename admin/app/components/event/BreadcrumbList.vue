<script setup lang="ts">
import type { BreadcrumbView } from '~/utils/event'

// Что было перед ошибкой: клики, переходы, запросы, консоль — со временем относительно ошибки
const props = defineProps<{ breadcrumbs: BreadcrumbView[], errorAt: string }>()
const { t } = useI18n()

const filter = ref('')
const showAll = ref(false)
const LIMIT = 30

const filtered = computed(() => {
  const q = filter.value.trim().toLowerCase()
  const list = q
    ? props.breadcrumbs.filter(b => `${b.category} ${b.message} ${b.data.map(([k, v]) => `${k} ${v}`).join(' ')}`.toLowerCase().includes(q))
    : props.breadcrumbs
  return [...list].reverse()
})
const shown = computed(() => (showAll.value ? filtered.value : filtered.value.slice(0, LIMIT)))

function icon(b: BreadcrumbView): string {
  if (b.category.startsWith('ui.'))
    return 'i-lucide-mouse-pointer-click'
  if (b.category === 'navigation')
    return 'i-lucide-route'
  if (b.category === 'fetch' || b.category === 'xhr' || b.type === 'http')
    return 'i-lucide-arrow-left-right'
  if (b.category === 'console')
    return 'i-lucide-terminal'
  if (b.type === 'error' || b.level === 'error')
    return 'i-lucide-octagon-alert'
  return 'i-lucide-dot'
}

function offset(b: BreadcrumbView): string {
  const s = secondsBefore(b.timestamp, props.errorAt)
  if (s === null)
    return ''
  const abs = Math.abs(s)
  return abs < 0.05 ? t('event.crumb.atError') : t('event.crumb.before', { s: new Intl.NumberFormat(intlTag.value, { maximumFractionDigits: 1 }).format(abs) })
}
</script>

<template>
  <div class="space-y-2">
    <UInput v-model="filter" icon="i-lucide-filter" size="sm" :placeholder="t('event.crumb.filter')" class="w-64 max-w-full" />
    <div class="overflow-hidden rounded-md border border-default text-xs">
      <div v-for="(b, i) in shown" :key="i" class="flex gap-3 border-b border-default px-3 py-1.5 last:border-b-0" :class="b.level === 'error' ? 'bg-error/5' : ''">
        <UIcon :name="icon(b)" class="mt-0.5 size-3.5 shrink-0 text-muted" />
        <div class="min-w-0 flex-1">
          <div class="flex flex-wrap items-baseline gap-x-2">
            <span class="font-medium text-highlighted">{{ b.category || b.type }}</span>
            <span v-if="b.message" class="break-all">{{ b.message }}</span>
          </div>
          <div v-if="b.data.length" class="mt-0.5 flex flex-wrap gap-x-3 font-mono text-dimmed">
            <span v-for="[k, v] in b.data" :key="k" class="break-all">{{ k }}: {{ v }}</span>
          </div>
        </div>
        <span class="shrink-0 whitespace-nowrap text-dimmed" :title="b.timestamp ? formatDateTime(b.timestamp) : ''">{{ offset(b) }}</span>
      </div>
      <div v-if="!shown.length" class="px-3 py-3 text-center text-muted">{{ t('event.crumb.empty') }}</div>
    </div>
    <UButton v-if="filtered.length > LIMIT && !showAll" size="xs" variant="link" :label="t('event.crumb.showAll', filtered.length)" @click="showAll = true" />
  </div>
</template>
