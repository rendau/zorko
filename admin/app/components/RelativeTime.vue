<script setup lang="ts">
// «5 мин назад», точное время — в подсказке
defineProps<{ value: string }>()
</script>

<template>
  <time :datetime="value" :title="formatDateTime(value)" class="whitespace-nowrap">{{ relativeTime(value) }}</time>
</template>
