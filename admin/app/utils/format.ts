// Числа, даты и «5 мин назад» — по текущему языку (intlTag)

export function formatNumber(n: number | string): string {
  return new Intl.NumberFormat(intlTag.value).format(Number(n))
}

// compactNumber — 1234 → «1,2 тыс.» / «1.2K»
export function compactNumber(n: number | string): string {
  return new Intl.NumberFormat(intlTag.value, { notation: 'compact', maximumFractionDigits: 1 }).format(Number(n))
}

export function formatDateTime(iso: string): string {
  if (!iso)
    return ''
  return new Intl.DateTimeFormat(intlTag.value, { dateStyle: 'medium', timeStyle: 'medium' }).format(new Date(iso))
}

export function formatTime(iso: string): string {
  return new Intl.DateTimeFormat(intlTag.value, { hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date(iso))
}

export function formatShortDate(d: Date, withTime: boolean): string {
  return new Intl.DateTimeFormat(intlTag.value, withTime
    ? { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' }
    : { day: 'numeric', month: 'short' }).format(d)
}

const UNITS: [Intl.RelativeTimeFormatUnit, number][] = [
  ['year', 365 * 24 * 3600],
  ['month', 30 * 24 * 3600],
  ['week', 7 * 24 * 3600],
  ['day', 24 * 3600],
  ['hour', 3600],
  ['minute', 60],
  ['second', 1],
]

// relativeTime — «5 минут назад» относительно now
export function relativeTime(iso: string, now = Date.now()): string {
  if (!iso)
    return ''
  const diff = (new Date(iso).getTime() - now) / 1000
  const fmt = new Intl.RelativeTimeFormat(intlTag.value, { numeric: 'auto', style: 'short' })
  for (const [unit, seconds] of UNITS) {
    if (Math.abs(diff) >= seconds || unit === 'second')
      return fmt.format(Math.round(diff / seconds), unit)
  }
  return ''
}

// formatBytes — 1913000 → «1,8 МБ» / «1.8 MB»
export function formatBytes(n: number | string): string {
  let v = Number(n) || 0
  const units = tf('common.byteUnits').split(',')
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${new Intl.NumberFormat(intlTag.value, { maximumFractionDigits: i ? 1 : 0 }).format(v)} ${units[i]}`
}
