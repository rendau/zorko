// Язык интерфейса: восстановить выбор и держать в одном состоянии vue-i18n и Intl утилит
const STORAGE_KEY = 'zorko:locale'

export default defineNuxtPlugin({
  name: 'zorko:locale',
  dependsOn: ['i18n:plugin'],
  async setup(nuxtApp) {
    const i18n = nuxtApp.$i18n

    const intlTagOf = (code: string) => i18n.locales.value.find(l => l.code === code)?.language ?? code
    const sync = (code: string) => setIntlLocale(intlTagOf(code))

    bindTranslate(
      (key, named) => (typeof named === 'number' ? i18n.t(key, named) : named ? i18n.t(key, named) : i18n.t(key)),
      key => i18n.te(key) || i18n.te(key, 'ru'),
    )

    let stored: string | null = null
    try {
      stored = localStorage.getItem(STORAGE_KEY)
    }
    catch {
      // хранилище недоступно — язык по умолчанию
    }
    if (isSwitcherLocale(stored) && stored !== i18n.locale.value)
      await i18n.setLocale(stored)

    nuxtApp.hook('i18n:localeSwitched', ({ newLocale }) => {
      sync(newLocale)
      try {
        localStorage.setItem(STORAGE_KEY, newLocale)
      }
      catch {
        // хранилище недоступно — выбор до перезагрузки
      }
    })
    sync(i18n.locale.value)
  },
})
