<script setup lang="ts">
import type { RrwebEvent } from '~/utils/replay'
import Player from '@sentry-internal/rrweb-player'
import '@sentry-internal/rrweb-player/dist/style.css'

// Проигрыватель записи (ADR-015): rrweb-player из той же ветки rrweb, что пишет SDK Sentry.
// Страница проигрывается в рамке без скриптов (sandbox без allow-scripts — так создаёт rrweb).
// Панель управления — своя (в стиле интерфейса, с перетаскиванием по шкале и отметками ошибок):
// встроенная остаётся в разметке, но скрыта — через неё работают методы проигрывателя
// (play, goto, setSpeed), а её цикл присылает текущее время.
export interface ReplayMark {
  offset: number // мс от начала записи
  title: string
}

const props = defineProps<{ events: RrwebEvent[], startAt?: number, marks?: ReplayMark[] }>()
const emit = defineEmits<{ time: [offsetMs: number] }>()
const { t } = useI18n()

// то, чем пользуемся у компонента Svelte (Svelte 5, совместимый класс)
interface PlayerApi {
  $set: (props: Record<string, unknown>) => void
  $destroy: () => void
  addEventListener: (name: string, cb: (e: { payload: unknown }) => void) => void
  goto: (timeOffset: number, play?: boolean) => void
  play: () => void
  pause: () => void
  setSpeed: (speed: number) => void
  toggleSkipInactive: () => void
  getMetaData: () => { totalTime: number }
  triggerResize: () => void
}

const host = ref<HTMLElement | null>(null)
const width = useElementWidth(host)
const vp = computed(() => viewport(props.events))
const height = computed(() => Math.min(Math.round(width.value * vp.value.height / vp.value.width), Math.round(window.innerHeight * 0.65)))

const current = ref(0)
const total = ref(0)
const playing = ref(false)
const speed = ref(1)
const skipInactive = ref(true)
const SPEEDS = [1, 2, 4, 8]

let player: PlayerApi | null = null

function mount() {
  if (!host.value || width.value <= 0 || player)
    return
  player = new Player({
    target: host.value,
    props: {
      events: props.events as never,
      width: width.value,
      height: height.value,
      autoPlay: false,
      skipInactive: true,
      showController: true,
      mouseTail: false,
    },
  }) as unknown as PlayerApi
  total.value = player.getMetaData().totalTime
  player.addEventListener('ui-update-current-time', (e) => {
    if (typeof e.payload !== 'number')
      return
    current.value = e.payload
    emit('time', e.payload)
  })
  player.addEventListener('ui-update-player-state', (e) => {
    playing.value = e.payload === 'playing'
  })
  // без перехода (и на 0 тоже) проигрыватель не отрисовывает первый кадр до «играть»
  player.goto(Math.max(props.startAt ?? 0, 0), false)
  current.value = Math.max(props.startAt ?? 0, 0)
}

watch(width, (w) => {
  if (!player) {
    mount()
    return
  }
  player.$set({ width: w, height: height.value })
  player.triggerResize()
}, { flush: 'post' })

onBeforeUnmount(() => {
  player?.$destroy()
  player = null
})

function seek(offsetMs: number, play = playing.value) {
  const v = Math.min(Math.max(offsetMs, 0), total.value)
  current.value = v
  player?.goto(v, play)
}

function toggle() {
  if (!player)
    return
  if (playing.value) {
    player.pause()
    return
  }
  // запись доиграла — с начала
  if (current.value >= total.value - 50)
    seek(0, true)
  else
    player.play()
}

function setSpeed(v: number) {
  speed.value = v
  player?.setSpeed(v)
}

function setSkip(v: boolean) {
  if (v !== skipInactive.value) {
    skipInactive.value = v
    player?.toggleSkipInactive()
  }
}

// перетаскивание ползунка: переход на каждый кадр, не чаще раза в кадр отрисовки
let pending: number | null = null
let frame = 0
function onSlide(v: number | number[] | undefined) {
  const value = Array.isArray(v) ? v[0] : v
  if (typeof value !== 'number')
    return
  current.value = value
  pending = value
  if (frame)
    return
  frame = requestAnimationFrame(() => {
    frame = 0
    if (pending !== null)
      player?.goto(pending, playing.value)
    pending = null
  })
}

// пробел — играть/пауза, стрелки — ±5 с (когда фокус на проигрывателе)
function onKey(e: KeyboardEvent) {
  if (e.key === ' ') {
    e.preventDefault()
    toggle()
  }
  else if (e.key === 'ArrowLeft') {
    e.preventDefault()
    seek(current.value - 5000)
  }
  else if (e.key === 'ArrowRight') {
    e.preventDefault()
    seek(current.value + 5000)
  }
}

function markLeft(offset: number): string {
  return `${total.value ? Math.min(Math.max(offset / total.value, 0), 1) * 100 : 0}%`
}

defineExpose({
  // goto — перейти к моменту записи (мс от начала) и играть дальше
  goto: (offsetMs: number) => seek(offsetMs, true),
  pause: () => player?.pause(),
})
</script>

<template>
  <div class="min-w-0 space-y-3 rounded-lg outline-none focus-visible:ring-2 focus-visible:ring-primary" tabindex="0" @keydown="onKey">
    <div ref="host" class="replay-player w-full overflow-hidden rounded-lg" :style="{ minHeight: `${height}px` }" />

    <div class="space-y-2">
      <div class="flex items-center gap-3">
        <span class="w-12 shrink-0 text-right font-mono text-xs tabular-nums text-muted">{{ formatOffset(current) }}</span>
        <div class="relative min-w-0 flex-1">
          <USlider :model-value="current" :min="0" :max="Math.max(total, 1)" :step="100" size="sm" :aria-label="t('replay.player.seek')" @update:model-value="onSlide" />
          <!-- отметки ошибок над шкалой: щелчок — к ошибке -->
          <button
            v-for="(m, i) in marks ?? []"
            :key="i"
            type="button"
            class="absolute -top-2.5 size-2.5 -translate-x-1/2 rounded-full bg-error ring-2 ring-default"
            :style="{ left: markLeft(m.offset) }"
            :title="m.title"
            :aria-label="m.title"
            @click="seek(m.offset)"
          />
        </div>
        <span class="w-12 shrink-0 font-mono text-xs tabular-nums text-muted">{{ formatOffset(total) }}</span>
      </div>

      <div class="flex flex-wrap items-center justify-center gap-x-4 gap-y-2">
        <UButton size="sm" color="neutral" variant="ghost" icon="i-lucide-rotate-ccw" :aria-label="t('replay.player.back5')" @click="seek(current - 5000)" />
        <UButton size="lg" color="primary" :icon="playing ? 'i-lucide-pause' : 'i-lucide-play'" :aria-label="playing ? t('replay.player.pause') : t('replay.player.play')" @click="toggle" />
        <UButton size="sm" color="neutral" variant="ghost" icon="i-lucide-rotate-cw" :aria-label="t('replay.player.forward5')" @click="seek(current + 5000)" />
        <div class="flex items-center gap-1">
          <UButton
            v-for="s in SPEEDS"
            :key="s"
            size="xs"
            :color="speed === s ? 'primary' : 'neutral'"
            :variant="speed === s ? 'soft' : 'ghost'"
            :label="`${s}×`"
            @click="setSpeed(s)"
          />
        </div>
        <USwitch :model-value="skipInactive" size="sm" :label="t('replay.player.skipInactive')" @update:model-value="setSkip" />
      </div>
      <p class="text-center text-xs text-dimmed">{{ t('replay.player.keys') }}</p>
    </div>
  </div>
</template>

<style scoped>
/* встроенная панель rrweb-player скрыта (методы проигрывателя работают через неё), высота — без неё */
.replay-player :deep(.rr-controller) {
  display: none;
}
.replay-player :deep(.rr-player) {
  box-shadow: none;
  height: auto !important;
  background: var(--ui-bg-elevated);
}
</style>
