<script setup lang="ts">
// Копировать текст в буфер обмена
const props = defineProps<{ text: string }>()
const { t } = useI18n()
const toast = useToast()

async function copy() {
  try {
    await navigator.clipboard.writeText(props.text)
    toast.add({ title: t('common.copied'), color: 'success' })
  }
  catch {
    toast.add({ title: t('common.copyFailed'), color: 'error' })
  }
}
</script>

<template>
  <UButton icon="i-lucide-copy" size="xs" color="neutral" variant="ghost" :aria-label="t('common.copy')" @click="copy" />
</template>
