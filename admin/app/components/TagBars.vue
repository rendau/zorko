<script setup lang="ts">
import type { TagDist } from '~/types/issue'

// Распределение значений тегов: топ-5 с процентами; клик — отобрать события с этим значением
defineProps<{ tags: TagDist[] }>()
const emit = defineEmits<{ select: [key: string, value: string] }>()

function pct(count: string, total: string): number {
  return Number(total) ? Math.round((Number(count) / Number(total)) * 100) : 0
}
</script>

<template>
  <div class="space-y-4">
    <div v-for="tag in tags" :key="tag.key">
      <div class="mb-1 flex items-baseline justify-between gap-2">
        <span class="truncate text-xs font-medium text-highlighted">{{ tag.key }}</span>
        <span class="text-xs text-dimmed">{{ compactNumber(tag.total) }}</span>
      </div>
      <button
        v-for="v in tag.values"
        :key="v.value"
        type="button"
        class="relative mb-0.5 flex w-full items-center justify-between gap-2 overflow-hidden rounded px-1.5 py-0.5 text-left text-xs hover:bg-elevated"
        :title="v.value"
        @click="emit('select', tag.key, v.value)"
      >
        <span class="absolute inset-y-0 left-0 bg-primary/15" :style="{ width: `${pct(v.count, tag.total)}%` }" />
        <span class="relative truncate">{{ v.value }}</span>
        <span class="relative shrink-0 text-muted">{{ pct(v.count, tag.total) }}%</span>
      </button>
    </div>
  </div>
</template>
