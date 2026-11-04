<script setup lang="ts">
// Маленький график событий за период в строке списка
const props = defineProps<{ values: (string | number)[] }>()

const bars = computed(() => {
  const nums = props.values.map(Number)
  const max = Math.max(1, ...nums)
  const w = 100 / Math.max(nums.length, 1)
  return nums.map((n, i) => ({ x: i * w, w: w * 0.7, h: n === 0 ? 0 : Math.max(2, (n / max) * 26) }))
})
</script>

<template>
  <svg viewBox="0 0 100 28" preserveAspectRatio="none" class="h-7 w-full" aria-hidden="true">
    <rect v-for="(b, i) in bars" :key="i" :x="b.x" :y="28 - b.h" :width="b.w" :height="b.h" rx="0.5" class="fill-primary/70" />
    <line x1="0" y1="27.5" x2="100" y2="27.5" class="stroke-(--ui-border)" stroke-width="0.5" vector-effect="non-scaling-stroke" />
  </svg>
</template>
