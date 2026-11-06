<script setup lang="ts">
import type { RrwebEvent } from '~/utils/replay'
import Player from '@sentry-internal/rrweb-player'
import '@sentry-internal/rrweb-player/dist/style.css'

// Проигрыватель записи (ADR-015): rrweb-player из той же ветки rrweb, что пишет SDK Sentry.
// Страница проигрывается в рамке без скриптов (sandbox без allow-scripts — так создаёт rrweb).
// Ширина — по контейнеру, высота — по пропорциям окна посетителя.
const props = defineProps<{ events: RrwebEvent[], startAt?: number }>()
const emit = defineEmits<{ time: [offsetMs: number] }>()

// то, чем пользуемся у компонента Svelte (Svelte 5, совместимый класс)
interface PlayerApi {
  $set: (props: Record<string, unknown>) => void
  $destroy: () => void
  addEventListener: (name: string, cb: (e: { payload: unknown }) => void) => void
  goto: (timeOffset: number, play?: boolean) => void
  pause: () => void
  triggerResize: () => void
}

const host = ref<HTMLElement | null>(null)
const width = useElementWidth(host)
const vp = computed(() => viewport(props.events))
// панель управления проигрывателя — около 80 px под записью
const CONTROLLER = 80
const height = computed(() => Math.min(Math.round(width.value * vp.value.height / vp.value.width), Math.round(window.innerHeight * 0.7)))

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
  player.addEventListener('ui-update-current-time', (e) => {
    if (typeof e.payload === 'number')
      emit('time', e.payload)
  })
  // без перехода (и на 0 тоже) проигрыватель не отрисовывает первый кадр до «играть»
  player.goto(Math.max(props.startAt ?? 0, 0), false)
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

defineExpose({
  // goto — перейти к моменту записи (мс от начала) и играть дальше
  goto: (offsetMs: number) => player?.goto(Math.max(offsetMs, 0), true),
  pause: () => player?.pause(),
})
</script>

<template>
  <div ref="host" class="replay-player w-full" :style="{ minHeight: `${height + CONTROLLER}px` }" />
</template>

<style scoped>
/* rrweb-player рисует свою тень и скругления — в карточке они лишние */
.replay-player :deep(.rr-player) {
  box-shadow: none;
  border-radius: 0.5rem;
  overflow: hidden;
  background: var(--ui-bg-elevated);
}
.replay-player :deep(.rr-controller) {
  background: var(--ui-bg);
}
/* время на шкале по умолчанию тёмное — на тёмной теме не видно */
.replay-player :deep(.rr-timeline__time) {
  color: var(--ui-text-muted);
}
</style>
