// Переключатель языка: языки — только SWITCHER_LOCALES (utils/locale.ts),
// названия — самоназвания из nuxt.config.ts, на любом языке интерфейса
// одни. Общий для меню пользователя (layouts/default.vue) и страниц входа
// (layouts/auth.vue).
export function useLocaleSwitcher() {
  const { locale, locales, setLocale } = useI18n()

  const items = computed(() => locales.value
    .filter(l => isSwitcherLocale(l.code))
    .map(l => ({ code: l.code, label: l.name ?? l.code })))

  return {
    items,
    current: locale,
    select: (code: string) => {
      if (isSwitcherLocale(code))
        void setLocale(code)
    },
  }
}
