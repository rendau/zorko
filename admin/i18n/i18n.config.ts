import en from './locales/en.json'
import ru from './locales/ru.json'
import { intlPluralRule } from '../app/utils/plural'

// Локали подключены статически — они внутри единственного бандла (ADR-007).
// Множественное число — правилами Intl.PluralRules: «{n} событие | {n} события | {n} событий», t('key', n).
export default defineI18nConfig(() => ({
  fallbackLocale: 'ru',
  messages: { ru, en },
  pluralRules: {
    ru: intlPluralRule('ru-RU'),
    en: intlPluralRule('en-GB'),
  },
}))
