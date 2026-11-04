// Проверка локалей i18n/locales/*.json (pnpm check:i18n): у всех языков
// один и тот же набор ключей, значения — непустые строки, форм
// множественного числа («a | b | c») не больше, чем категорий у языка
// (Intl.PluralRules: ru — 4 с дробными, en — 2, kk — 2). Эталон — ru.json.
//
// И каждое сообщение собирается тем же компилятором, что при сборке
// (generateJSON из @intlify/bundle-utils — его зовёт unplugin-vue-i18n).
// Одно несобираемое сообщение роняет всю локаль: страница молча показывается
// по-русски и выглядит непереведённой, а не сломанной. Ломают сборку:
// похожее на HTML-тег латиницей (<token>, <br>, «a < b > c»; <токен>
// кириллицей — нет), @ вне ссылки на ключ (admin@example.com, «a @ b»),
// пустая форма множественного числа («a || b», «a |»), непарная { или } и
// пустая {}. Литерал — {'@'}, {'{'}, {'}'}, {'|'}; угловые скобки —
// в шаблон, не в сообщение.
import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const dir = join(dirname(fileURLToPath(import.meta.url)), '..', 'i18n', 'locales')
const LOCALES = { ru: 'ru-RU', en: 'en-GB' }

function flatten(obj, prefix = '', out = new Map()) {
  for (const [k, v] of Object.entries(obj)) {
    const key = prefix ? `${prefix}.${k}` : k
    if (v && typeof v === 'object')
      flatten(v, key, out)
    else
      out.set(key, v)
  }
  return out
}

// Необязательные группы ключей: есть только у языков, где форма отличается
// (calendar.monthInDate — месяц внутри даты, «15 ноября»; в en и kk
// совпадает с заголовочной формой, и группы там нет). Если группа у языка
// есть — то целиком.
const OPTIONAL_GROUPS = ['calendar.monthInDate.']
const optionalGroup = key => OPTIONAL_GROUPS.find(prefix => key.startsWith(prefix))

const messages = Object.fromEntries(Object.keys(LOCALES).map(code => [code, flatten(JSON.parse(readFileSync(join(dir, `${code}.json`), 'utf8')))]))
const errors = []
const reference = messages.ru

for (const [code, tag] of Object.entries(LOCALES)) {
  const own = messages[code]
  const categories = new Intl.PluralRules(tag).resolvedOptions().pluralCategories.length
  for (const key of reference.keys()) {
    if (!own.has(key) && !optionalGroup(key))
      errors.push(`${code}: нет ключа ${key}`)
  }
  for (const prefix of OPTIONAL_GROUPS) {
    const refKeys = [...reference.keys()].filter(k => k.startsWith(prefix))
    const ownKeys = [...own.keys()].filter(k => k.startsWith(prefix))
    if (ownKeys.length && ownKeys.length !== refKeys.length)
      errors.push(`${code}: группа ${prefix}* неполная — ${ownKeys.length} из ${refKeys.length}`)
  }
  for (const [key, value] of own) {
    if (!reference.has(key))
      errors.push(`${code}: лишний ключ ${key} (нет в ru.json)`)
    if (typeof value !== 'string' || value.trim() === '') {
      errors.push(`${code}: ${key} — пустое значение`)
      continue
    }
    const forms = value.split('|').length
    if (forms > categories)
      errors.push(`${code}: ${key} — форм ${forms}, у языка категорий ${categories}`)
  }
}

// компилятор — той версии, что у сборки: по цепочке зависимостей модуля
// i18n, а не отдельной зависимостью, которая могла бы разойтись с ним
async function loadGenerateJSON() {
  let req = createRequire(import.meta.url)
  let path = ''
  for (const name of ['@nuxtjs/i18n', '@intlify/unplugin-vue-i18n', '@intlify/bundle-utils']) {
    path = req.resolve(name)
    req = createRequire(path)
  }
  return (await import(path)).generateJSON
}

// подсказка по тексту ошибки компилятора — что исправить в сообщении
function compileHint(reason) {
  if (reason.startsWith('Detected HTML'))
    return 'похоже на HTML-тег — угловые скобки вынести в шаблон'
  if (/linked/i.test(reason))
    return '@ читается как ссылка на ключ — литерал {\'@\'}'
  if (/plural/i.test(reason))
    return 'пустая форма множественного числа — лишний |'
  if (/brace|placeholder/i.test(reason))
    return 'непарная или пустая фигурная скобка — литерал {\'{\'} / {\'}\'}'
  return ''
}

const generateJSON = await loadGenerateJSON()
for (const code of Object.keys(LOCALES)) {
  const file = join(dir, `${code}.json`)
  // одна ошибка на ключ: на один лишний @ компилятор отвечает тремя
  const seen = new Set()
  generateJSON(readFileSync(file, 'utf8'), {
    type: 'plain',
    filename: file,
    env: 'production',
    // как у сборки: strictMessage по умолчанию — HTML в сообщении запрещён
    onError: (reason, extra) => {
      if (seen.has(extra?.path))
        return
      seen.add(extra?.path)
      const hint = compileHint(reason)
      errors.push(`${code}: ${extra?.path || '?'} — не собирается: ${reason}${hint ? ` (${hint})` : ''}\n    строка: ${JSON.stringify(extra?.source ?? '')}`)
    },
  })
}

if (errors.length) {
  console.error(errors.join('\n'))
  process.exit(1)
}
console.log(`locales ok: ${reference.size} ключей × ${Object.keys(LOCALES).length} языка`)
