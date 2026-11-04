import { ref } from 'vue'

// Язык вне компонентов: форматирование чисел и дат (Intl) и тексты утилит.
// Состояние держит plugins/locale.client.ts — он связывает его с vue-i18n.

export type LocaleCode = 'ru' | 'en'

// языки переключателя — единственное место, где язык включают и выключают
export const SWITCHER_LOCALES: readonly LocaleCode[] = ['ru', 'en']

export function isSwitcherLocale(code: unknown): code is LocaleCode {
  return typeof code === 'string' && (SWITCHER_LOCALES as readonly string[]).includes(code)
}

// тег Intl текущего языка; ref — чтобы шаблоны с форматтерами перерисовывались при смене языка
export const intlTag = ref('ru-RU')

export function setIntlLocale(tag: string) {
  intlTag.value = tag
}

type Translate = (key: string, named?: Record<string, unknown> | number) => string
let translate: Translate = key => key
let exists: (key: string) => boolean = () => false

export function bindTranslate(t: Translate, te: (key: string) => boolean) {
  translate = t
  exists = te
}

// tf — перевод для утилит; число вторым аргументом — множественная форма
export function tf(key: string, named?: Record<string, unknown> | number): string {
  return translate(key, named)
}

export function tfExists(key: string): boolean {
  return exists(key)
}
