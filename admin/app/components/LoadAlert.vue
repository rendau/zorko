<script setup lang="ts">
// Ошибка загрузки страницы с кнопкой повтора.
defineProps<{
  title: string
  error: string
}>()

const emit = defineEmits<{ retry: [] }>()
const { t } = useI18n()
</script>

<template>
  <UAlert
    v-if="error"
    color="error"
    variant="subtle"
    :title="title"
    :description="error"
    :actions="[{ label: t('common.retry'), color: 'error', variant: 'soft', onClick: () => emit('retry') }]"
  />
</template>
