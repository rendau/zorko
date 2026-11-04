<script setup lang="ts">
const props = defineProps<{ level: string }>()
const { t, te } = useI18n()
const label = computed(() => (te(`level.${props.level}`) ? t(`level.${props.level}`) : props.level))
</script>

<template>
  <UBadge :color="levelColor(level)" variant="subtle" size="sm">{{ label }}</UBadge>
</template>
