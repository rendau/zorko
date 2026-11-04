// Множественное число по правилам языка (Intl.PluralRules, CLDR): форм
// столько, сколько категорий у языка — в русском one/few/many (+ other для
// дробных), в английском one/other, в казахском одна. Формы в сообщении
// перечисляются в каноническом порядке категорий; недостающие хвостовые
// формы заменяет последняя данная — так в казахском хватает одной формы.

const CATEGORY_ORDER: Intl.LDMLPluralRule[] = ['zero', 'one', 'two', 'few', 'many', 'other']

const rulesCache = new Map<string, { rules: Intl.PluralRules, categories: Intl.LDMLPluralRule[] }>()

function pluralRules(locale: string) {
  let entry = rulesCache.get(locale)
  if (!entry) {
    const rules = new Intl.PluralRules(locale)
    const present = rules.resolvedOptions().pluralCategories
    entry = { rules, categories: CATEGORY_ORDER.filter(c => present.includes(c)) }
    rulesCache.set(locale, entry)
  }
  return entry
}

// pluralIndex — номер формы для n среди choicesLength перечисленных
export function pluralIndex(locale: string, n: number, choicesLength: number): number {
  const { rules, categories } = pluralRules(locale)
  const index = categories.indexOf(rules.select(n))
  return Math.max(0, Math.min(index, choicesLength - 1))
}

// intlPluralRule — правило для vue-i18n (pluralRules в i18n.config.ts)
export function intlPluralRule(locale: string) {
  return (choice: number, choicesLength: number) => pluralIndex(locale, Math.abs(choice), choicesLength)
}
