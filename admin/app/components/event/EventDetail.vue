<script setup lang="ts">
import type { EventMain } from '~/types/event'

// Событие целиком: исключения со стеком, что было перед ошибкой, теги, контекст, extra, сырой JSON
const props = defineProps<{ event: EventMain }>()
const emit = defineEmits<{ tag: [key: string, value: string] }>()
const { t, te } = useI18n()
const toast = useToast()

const view = computed(() => parseEvent(props.event.data))
const showJson = ref(false)
const prettyJson = computed(() => {
  try {
    return JSON.stringify(JSON.parse(props.event.data), null, 2)
  }
  catch {
    return props.event.data
  }
})

function contextTitle(key: string): string {
  return te(`event.context.${key}`) ? t(`event.context.${key}`) : key
}

async function copyJson() {
  try {
    await navigator.clipboard.writeText(prettyJson.value)
    toast.add({ title: t('common.copied'), color: 'success' })
  }
  catch {
    // буфер обмена недоступен (не https) — JSON виден на экране
  }
}
</script>

<template>
  <div class="space-y-6">
    <section v-if="view.exceptions.length" class="space-y-4">
      <h3 class="text-sm font-semibold text-highlighted">{{ t('event.section.exception') }}</h3>
      <EventExceptionBlock v-for="(e, i) in view.exceptions" :key="i" :exception="e" />
    </section>

    <section v-if="view.message && !view.exceptions.some(e => !e.synthetic)">
      <h3 class="mb-2 text-sm font-semibold text-highlighted">{{ t('event.section.message') }}</h3>
      <p class="whitespace-pre-wrap break-words rounded-md border border-default bg-elevated/40 px-3 py-2 font-mono text-xs">{{ view.message }}</p>
    </section>

    <section v-if="view.breadcrumbs.length">
      <h3 class="mb-2 text-sm font-semibold text-highlighted">{{ t('event.section.breadcrumbs') }}</h3>
      <EventBreadcrumbList :breadcrumbs="view.breadcrumbs" :error-at="event.summary.timestamp" />
    </section>

    <section v-if="view.tags.length">
      <h3 class="mb-2 text-sm font-semibold text-highlighted">{{ t('event.section.tags') }}</h3>
      <div class="flex flex-wrap gap-1.5">
        <button
          v-for="[k, v] in view.tags"
          :key="k"
          type="button"
          class="inline-flex max-w-full items-center overflow-hidden rounded-md border border-default text-xs hover:border-primary"
          :title="t('event.tagFilter')"
          @click="emit('tag', k, v)"
        >
          <span class="bg-elevated px-1.5 py-0.5 text-muted">{{ k }}</span>
          <span class="truncate px-1.5 py-0.5 font-mono">{{ v }}</span>
        </button>
      </div>
    </section>

    <section v-if="view.contexts.length">
      <h3 class="mb-2 text-sm font-semibold text-highlighted">{{ t('event.section.context') }}</h3>
      <div class="grid gap-4 md:grid-cols-2">
        <div v-for="c in view.contexts" :key="c.key" class="rounded-md border border-default px-3 py-2">
          <div class="mb-1 text-xs font-semibold text-highlighted">{{ contextTitle(c.key) }}</div>
          <EventKeyValueTable :items="c.items" />
        </div>
      </div>
    </section>

    <section v-if="view.extra.length">
      <h3 class="mb-2 text-sm font-semibold text-highlighted">{{ t('event.section.extra') }}</h3>
      <div class="rounded-md border border-default px-3 py-2">
        <EventKeyValueTable :items="view.extra" />
      </div>
    </section>

    <section>
      <div class="mb-2 flex items-center gap-2">
        <h3 class="text-sm font-semibold text-highlighted">{{ t('event.section.json') }}</h3>
        <UButton size="xs" variant="link" :label="showJson ? t('common.hide') : t('common.show')" @click="showJson = !showJson" />
        <UButton v-if="showJson" size="xs" variant="ghost" color="neutral" icon="i-lucide-copy" :label="t('common.copy')" @click="copyJson" />
      </div>
      <pre v-if="showJson" class="max-h-[32rem] overflow-auto rounded-md border border-default bg-elevated/40 p-3 font-mono text-xs">{{ prettyJson }}</pre>
    </section>
  </div>
</template>
