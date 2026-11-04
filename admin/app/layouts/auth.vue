<script setup lang="ts">
// Вход и первый запуск: меню нет, язык выбирается здесь же
const localeSwitcher = useLocaleSwitcher()
const { t } = useI18n()
</script>

<template>
  <div class="flex min-h-screen items-center justify-center bg-default p-4">
    <div class="w-full max-w-sm">
      <div class="mb-6 flex items-center justify-center gap-2 text-lg font-bold text-highlighted">
        <AppLogo class="size-7" />
        zorko
      </div>
      <UCard>
        <slot />
      </UCard>
      <div class="mt-4 flex justify-center">
        <USelect
          :model-value="localeSwitcher.current.value"
          :items="localeSwitcher.items.value"
          value-key="code"
          icon="i-lucide-languages"
          size="sm"
          variant="ghost"
          :aria-label="t('layout.language')"
          @update:model-value="v => localeSwitcher.select(String(v))"
        />
      </div>
    </div>
  </div>
</template>
