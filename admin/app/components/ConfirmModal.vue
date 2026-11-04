<script setup lang="ts">
// Подтверждение разрушительного действия.
defineProps<{
  title: string
  description?: string
  confirmLabel?: string
  loading?: boolean
}>()

const open = defineModel<boolean>('open', { required: true })
const emit = defineEmits<{ confirm: [] }>()
const { t } = useI18n()
</script>

<template>
  <UModal v-model:open="open" :title="title" :description="description">
    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton color="neutral" variant="ghost" :label="t('common.cancel')" @click="open = false" />
        <UButton color="error" :label="confirmLabel ?? t('common.delete')" :loading="loading" @click="emit('confirm')" />
      </div>
    </template>
  </UModal>
</template>
