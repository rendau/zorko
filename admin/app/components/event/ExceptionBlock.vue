<script setup lang="ts">
import type { ExceptionView, FrameView } from '~/utils/event'

// Исключение со стеком: свой код раскрыт, кадры библиотек свёрнуты подряд идущими группами.
// Переведённый по картам стек — с переключателем на исходный сжатый; у непереведённых кадров — причина.
const props = defineProps<{ exception: ExceptionView, projectId?: string }>()
const { t, te } = useI18n()

interface Group {
  inApp: boolean
  frames: FrameView[]
}

const showRaw = ref(false)
const frames = computed(() => (showRaw.value && props.exception.rawFrames) ? props.exception.rawFrames : props.exception.frames)
const problem = computed(() => showRaw.value ? null : sourcemapProblem(props.exception))

const groups = computed<Group[]>(() => {
  const out: Group[] = []
  for (const f of frames.value) {
    const last = out[out.length - 1]
    if (last && last.inApp === f.inApp)
      last.frames.push(f)
    else
      out.push({ inApp: f.inApp, frames: [f] })
  }
  return out
})

// если своих кадров нет совсем — показываем всё раскрытым
const anyInApp = computed(() => frames.value.some(f => f.inApp))
const expanded = ref(new Set<number>())
const openFrames = ref(new Set<string>())

function toggleGroup(i: number) {
  const next = new Set(expanded.value)
  if (next.has(i))
    next.delete(i)
  else
    next.add(i)
  expanded.value = next
}

function frameKey(gi: number, fi: number) {
  return `${gi}:${fi}`
}

function toggleFrame(key: string) {
  const next = new Set(openFrames.value)
  if (next.has(key))
    next.delete(key)
  else
    next.add(key)
  openFrames.value = next
}

// контекст первого своего кадра (место падения) раскрыт сразу
function openFirst() {
  expanded.value = new Set()
  const gi = groups.value.findIndex(g => g.inApp)
  openFrames.value = gi >= 0 && groups.value[gi]!.frames[0]?.context.length ? new Set([frameKey(gi, 0)]) : new Set()
}
onMounted(openFirst)
watch(showRaw, openFirst)

// codeText — пояснение по коду причины; site_http_404 → «сайт ответил 404»
function codeText(code: string): string {
  const http = /^site_http_(\d+)$/.exec(code)
  if (http)
    return t('sourcemap.code.site_http', { status: http[1] })
  return te(`sourcemap.code.${code}`) ? t(`sourcemap.code.${code}`) : code
}

function reason(error: string, siteError: string): string {
  const ci = codeText(error)
  return siteError ? t('sourcemap.reason', { ci, site: codeText(siteError) }) : ci
}

const viewItems = computed(() => [
  { label: t('sourcemap.view.original'), value: 'original' },
  { label: t('sourcemap.view.minified'), value: 'minified' },
])
const viewMode = computed({
  get: () => showRaw.value ? 'minified' : 'original',
  set: (v: string) => { showRaw.value = v === 'minified' },
})
</script>

<template>
  <div class="space-y-2">
    <div>
      <div class="flex flex-wrap items-center gap-2">
        <span class="font-mono text-sm font-semibold text-highlighted">{{ exception.synthetic ? t('event.message') : (exception.type || t('event.noType')) }}</span>
        <UBadge v-if="exception.handled === false" color="error" variant="subtle" size="sm">{{ t('event.unhandled') }}</UBadge>
        <UBadge v-else-if="exception.handled" color="neutral" variant="subtle" size="sm">{{ t('event.handled') }}</UBadge>
        <span v-if="exception.mechanism" class="text-xs text-dimmed">{{ t('event.mechanism') }}: {{ exception.mechanism }}</span>
        <UTabs
          v-if="exception.rawFrames"
          v-model="viewMode"
          :items="viewItems"
          :content="false"
          size="xs"
          class="ms-auto w-auto"
          :aria-label="t('sourcemap.view.aria')"
        />
      </div>
      <p v-if="exception.value && !exception.synthetic" class="mt-1 break-words text-sm">{{ exception.value }}</p>
    </div>

    <UAlert
      v-if="problem"
      color="warning"
      variant="subtle"
      icon="i-lucide-map"
      :title="t('sourcemap.problem.title', problem.count)"
      :description="reason(problem.error, problem.siteError)"
      :actions="projectId ? [{ label: t('sourcemap.problem.toMaps'), to: `/settings/projects/${projectId}#maps`, color: 'warning', variant: 'outline', size: 'xs' }] : []"
      :ui="{ title: 'text-sm', description: 'text-xs' }"
    />

    <div v-if="frames.length" class="overflow-hidden rounded-md border border-default text-xs">
      <template v-for="(g, gi) in groups" :key="`${showRaw}-${gi}`">
        <button
          v-if="!g.inApp && anyInApp && !expanded.has(gi)"
          type="button"
          class="flex w-full items-center gap-2 border-b border-default bg-elevated/40 px-3 py-1.5 text-left text-muted last:border-b-0 hover:bg-elevated"
          @click="toggleGroup(gi)"
        >
          <UIcon name="i-lucide-chevrons-up-down" class="size-3.5" />
          {{ t('event.libraryFrames', g.frames.length) }}
        </button>
        <template v-else>
          <div v-for="(f, fi) in g.frames" :key="fi" class="border-b border-default last:border-b-0">
            <button
              type="button"
              class="flex w-full flex-wrap items-baseline gap-x-2 px-3 py-1.5 text-left font-mono hover:bg-elevated/50"
              :class="f.inApp ? 'text-highlighted' : 'text-muted'"
              :disabled="!f.context.length"
              @click="toggleFrame(frameKey(gi, fi))"
            >
              <span class="break-all">{{ f.module || f.filename }}</span>
              <span v-if="f.function" class="text-dimmed">{{ t('event.in') }}</span>
              <span v-if="f.function" class="font-semibold">{{ f.function }}</span>
              <span v-if="f.lineno" class="text-dimmed">{{ t('event.at', { line: f.lineno, col: f.colno }) }}</span>
              <span class="ms-auto flex items-center gap-1">
                <UTooltip v-if="f.error && !showRaw" :text="reason(f.error, f.siteError)">
                  <UBadge color="neutral" variant="outline" size="sm" icon="i-lucide-map-pin-off" tabindex="0">{{ t('sourcemap.notResolved') }}</UBadge>
                </UTooltip>
                <UBadge v-if="f.inApp" color="primary" variant="soft" size="sm">{{ t('event.inApp') }}</UBadge>
              </span>
            </button>
            <pre
              v-if="openFrames.has(frameKey(gi, fi)) && f.context.length"
              class="overflow-x-auto bg-elevated/40 py-1 font-mono text-xs leading-5"
            ><code><div
              v-for="l in f.context"
              :key="l.line"
              class="flex"
              :class="l.current ? 'bg-error/15 text-highlighted' : 'text-muted'"
            ><span class="inline-block w-12 shrink-0 select-none pe-3 text-right text-dimmed">{{ l.line }}</span><span class="whitespace-pre">{{ l.text }}</span></div></code></pre>
          </div>
        </template>
      </template>
    </div>
  </div>
</template>
