<script setup lang="ts">
import { en, ru } from '@nuxt/ui/locale'

// Локаль компонентов Nuxt UI и lang документа — за языком интерфейса
const { locale } = useI18n()
const uiLocale = computed(() => (locale.value === 'en' ? en : ru))

useHead({ htmlAttrs: { lang: locale } })
</script>

<template>
  <UApp :locale="uiLocale" :toaster="{ position: 'top-right' }">
    <NuxtLayout>
      <NuxtPage />
    </NuxtLayout>
  </UApp>
</template>
