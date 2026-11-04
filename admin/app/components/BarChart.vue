<script setup lang="ts">
import type { IssueStatPoint } from '~/types/issue'

// График событий по интервалам: рисуется в настоящей ширине контейнера, чтобы подписи не сжимались
const props = defineProps<{ points: IssueStatPoint[], interval: 'hour' | 'day' }>()
const { t } = useI18n()

const el = ref<HTMLElement | null>(null)
const width = useElementWidth(el)
const height = 140
const padBottom = 20
const padLeft = 36

const max = computed(() => Math.max(1, ...props.points.map(p => Number(p.count))))
const total = computed(() => props.points.reduce((s, p) => s + Number(p.count), 0))

const bars = computed(() => {
  const n = props.points.length
  if (!n || width.value <= padLeft)
    return []
  const step = (width.value - padLeft) / n
  return props.points.map((p, i) => {
    const count = Number(p.count)
    const h = count === 0 ? 0 : Math.max(2, (count / max.value) * (height - padBottom - 8))
    return { x: padLeft + i * step + step * 0.15, w: Math.max(1, step * 0.7), y: height - padBottom - h, h, count, ts: p.ts }
  })
})

// подписи оси — не чаще одной на 80 px
const labels = computed(() => {
  const n = bars.value.length
  if (!n)
    return []
  const every = Math.max(1, Math.ceil(n / Math.max(1, Math.floor((width.value - padLeft) / 80))))
  return bars.value
    .filter((_, i) => i % every === 0)
    .map(b => ({ x: b.x + b.w / 2, text: formatShortDate(new Date(b.ts), props.interval === 'hour') }))
})

const hover = ref<number | null>(null)
const hovered = computed(() => (hover.value === null ? null : bars.value[hover.value] ?? null))
</script>

<template>
  <div ref="el" class="relative w-full">
    <svg v-if="width" :width="width" :height="height" class="block" role="img" :aria-label="t('issue.chart.aria', total)">
      <text :x="padLeft - 6" y="12" text-anchor="end" class="fill-(--ui-text-dimmed) text-[10px]">{{ compactNumber(max) }}</text>
      <line :x1="padLeft" :x2="width" :y1="height - padBottom + 0.5" :y2="height - padBottom + 0.5" class="stroke-(--ui-border)" />
      <g v-for="(b, i) in bars" :key="i" @mouseenter="hover = i" @mouseleave="hover = null">
        <rect :x="b.x - 1" :y="0" :width="b.w + 2" :height="height - padBottom" fill="transparent" />
        <rect :x="b.x" :y="b.y" :width="b.w" :height="b.h" rx="1" :class="hover === i ? 'fill-primary' : 'fill-primary/60'" />
      </g>
      <text v-for="(l, i) in labels" :key="`l${i}`" :x="l.x" :y="height - 5" text-anchor="middle" class="fill-(--ui-text-dimmed) text-[10px]">{{ l.text }}</text>
    </svg>
    <div
      v-if="hovered"
      class="pointer-events-none absolute top-0 rounded-md border border-default bg-default px-2 py-1 text-xs shadow"
      :style="{ left: `${Math.max(0, Math.min(hovered.x, width - 150))}px` }"
    >
      <div class="text-muted">{{ formatShortDate(new Date(hovered.ts), interval === 'hour') }}</div>
      <div class="font-medium text-highlighted">{{ t('issue.eventsCount', hovered.count) }}</div>
    </div>
  </div>
</template>
