<script setup lang="ts">
// Пустое состояние списка/секции: иконка + текст + опциональное действие
// (слот). Не «дыра», а объяснение, что тут будет и как это получить.
defineProps<{
  icon?: string
  title: string
  description?: string
}>()
</script>

<template>
  <div class="flex flex-col items-center gap-2 py-10 text-center">
    <UIcon v-if="icon" :name="icon" class="size-8 text-dimmed" />
    <p class="text-sm text-muted">{{ title }}</p>
    <p v-if="description" class="max-w-md text-xs text-dimmed">{{ description }}</p>
    <slot />
  </div>
</template>
