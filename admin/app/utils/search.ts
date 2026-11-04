// Строка поиска (язык — internal/service/searchquery): слово под курсором для подсказок

export interface SearchToken {
  start: number
  end: number
  negate: boolean
  key: string // до двоеточия; пусто — набирается ключ
  value: string // после двоеточия
  hasColon: boolean
}

// tokenAt — слово, в котором стоит курсор (для подсказок хватает разбиения по пробелам)
export function tokenAt(q: string, cursor: number): SearchToken {
  let start = cursor
  while (start > 0 && !/\s/.test(q[start - 1] ?? ''))
    start--
  let end = cursor
  while (end < q.length && !/\s/.test(q[end] ?? ''))
    end++

  let text = q.slice(start, end)
  const negate = text.startsWith('!')
  if (negate)
    text = text.slice(1)

  const colon = text.indexOf(':')
  return colon === -1
    ? { start, end, negate, key: text, value: '', hasColon: false }
    : { start, end, negate, key: text.slice(0, colon), value: text.slice(colon + 1).replace(/^"|"$/g, ''), hasColon: true }
}

function quote(value: string): string {
  return /[\s"]/.test(value) ? `"${value.replace(/"/g, '\\"')}"` : value
}

// replaceToken — подставить на место слова ключ (и значение); значение с пробелом — в кавычках
export function replaceToken(q: string, token: SearchToken, key: string, value?: string): { q: string, cursor: number } {
  const text = `${token.negate ? '!' : ''}${key}:${value === undefined ? '' : quote(value)}`
  const suffix = value === undefined ? '' : ' '
  const rest = value === undefined ? q.slice(token.end) : q.slice(token.end).replace(/^\s+/, '')
  return { q: q.slice(0, token.start) + text + suffix + rest, cursor: token.start + text.length + suffix.length }
}

// addCondition — добавить key:value (клик по значению тега), заменив прежнее условие на этот ключ
export function addCondition(q: string, key: string, value: string): string {
  const parts = q.split(/\s+/).filter(p => p && !p.startsWith(`${key}:`))
  parts.push(`${key}:${quote(value)}`)
  return parts.join(' ')
}
