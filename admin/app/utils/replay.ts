// Записи сессий (ADR-015): распаковка сегментов и разбор служебных событий SDK для вкладок проигрывателя.
// Сегмент — JSON-массив событий rrweb, сжатый zlib; SDK Sentry кладёт в него и свои события
// (type 5): breadcrumb — клики, переходы, консоль; performanceSpan — запросы к серверу.
// Заголовки запроса и ответа SDK кладёт в performanceSpan только для адресов из
// networkDetailAllowUrls (настройка replayIntegration на сайте), для остальных — один размер тела.

export interface RrwebEvent {
  type: number
  timestamp: number
  data?: unknown
}

// RRWEB_CUSTOM — событие SDK поверх записи; RRWEB_META — размер окна; RRWEB_FULL_SNAPSHOT — снимок страницы
export const RRWEB_FULL_SNAPSHOT = 2
export const RRWEB_META = 4
export const RRWEB_CUSTOM = 5

type Json = Record<string, unknown>

function isObject(v: unknown): v is Json {
  return typeof v === 'object' && v !== null && !Array.isArray(v)
}

function str(v: unknown): string {
  return typeof v === 'string' ? v : typeof v === 'number' ? String(v) : ''
}

// decodeSegment — тело сегмента (zlib или голый JSON) → события rrweb
export async function decodeSegment(buf: ArrayBuffer): Promise<RrwebEvent[]> {
  const bytes = new Uint8Array(buf)
  let text: string
  if (bytes[0] === 0x78) {
    const stream = new Blob([bytes]).stream().pipeThrough(new DecompressionStream('deflate'))
    text = await new Response(stream).text()
  }
  else {
    text = new TextDecoder().decode(bytes)
  }
  const parsed: unknown = JSON.parse(text)
  if (!Array.isArray(parsed))
    return []
  return parsed
    .filter((e): e is RrwebEvent => isObject(e) && typeof e.type === 'number' && typeof e.timestamp === 'number')
    .map(normalizeTimestamp)
}

// SDK Sentry пишет часть своих событий (запросы, замеры памяти, фокус) со временем в секундах,
// остальные — в миллисекундах; без приведения они встают в начало записи на десятилетия раньше
function normalizeTimestamp(e: RrwebEvent): RrwebEvent {
  return e.timestamp < 1e11 ? { ...e, timestamp: Math.round(e.timestamp * 1000) } : e
}

export type ReplayItemKind = 'console' | 'network' | 'click' | 'navigation' | 'input' | 'other'

// ReplayNetworkPart — половина запроса (отправленное или ответ) так, как её прислал сайт.
// Тела SDK тоже умеет слать (networkCaptureBodies) — их не показываем
export interface ReplayNetworkPart {
  headers: [string, string][] // имена в нижнем регистре (так пишет SDK), по алфавиту
  size: number // размер тела, байт; 0 — тела нет или размер неизвестен
}

export interface ReplayItem {
  kind: ReplayItemKind
  at: number // время, мс Unix
  title: string
  detail: string
  level: string // для консоли: log / warn / error
  status: number // для запросов: код ответа, 0 — нет
  method: string
  durationMs: number
  request: ReplayNetworkPart | null // для запросов; null — сайт не прислал ничего
  response: ReplayNetworkPart | null
}

function seconds(v: unknown): number {
  return typeof v === 'number' ? Math.round(v * 1000) : 0
}

function breadcrumbItem(at: number, p: Json): ReplayItem | null {
  const category = str(p.category)
  const data = isObject(p.data) ? p.data : {}
  const base = { at: seconds(p.timestamp) || at, detail: '', level: str(p.level) || 'info', status: 0, method: '', durationMs: 0, request: null, response: null }
  switch (category) {
    case 'console': {
      const args = Array.isArray(data.arguments) ? data.arguments.map(a => (typeof a === 'string' ? a : JSON.stringify(a))).join(' ') : ''
      return { ...base, kind: 'console', title: str(p.message) || args, detail: args && args !== str(p.message) ? args : '' }
    }
    case 'navigation':
      return { ...base, kind: 'navigation', title: str(data.to) || str(p.message), detail: str(data.from) }
    case 'ui.click':
    case 'ui.tap':
    case 'ui.multiClick':
    case 'ui.slowClickDetected': {
      const node = isObject(data.node) ? data.node : {}
      return { ...base, kind: 'click', title: str(p.message), detail: str(node.textContent) }
    }
    case 'ui.input':
      return { ...base, kind: 'input', title: str(p.message), detail: '' }
    default:
      return null
  }
}

function networkPart(v: unknown): ReplayNetworkPart | null {
  if (!isObject(v))
    return null
  const headers = isObject(v.headers)
    ? Object.entries(v.headers)
        .map(([k, value]): [string, string] => [k, str(value)])
        .filter(([, value]) => value !== '')
        .sort(([a], [b]) => a.localeCompare(b))
    : []
  return { headers, size: Number(v.size) || 0 }
}

function spanItem(at: number, p: Json): ReplayItem | null {
  const op = str(p.op)
  if (op !== 'resource.fetch' && op !== 'resource.xhr')
    return null
  const data = isObject(p.data) ? p.data : {}
  const start = seconds(p.startTimestamp) || at
  return {
    kind: 'network',
    at: start,
    title: str(p.description),
    detail: '',
    level: 'info',
    status: Number(data.statusCode) || 0,
    method: str(data.method) || 'GET',
    durationMs: Math.max(seconds(p.endTimestamp) - start, 0),
    request: networkPart(data.request),
    response: networkPart(data.response),
  }
}

// replayItems — консоль, запросы, клики и переходы из событий SDK, по времени
export function replayItems(events: RrwebEvent[]): ReplayItem[] {
  const result: ReplayItem[] = []
  for (const e of events) {
    if (e.type !== RRWEB_CUSTOM || !isObject(e.data) || !isObject(e.data.payload))
      continue
    const item = e.data.tag === 'breadcrumb'
      ? breadcrumbItem(e.timestamp, e.data.payload)
      : e.data.tag === 'performanceSpan' ? spanItem(e.timestamp, e.data.payload) : null
    if (item)
      result.push(item)
  }
  return result.sort((a, b) => a.at - b.at)
}

// playable — события для проигрывателя: с первого Meta (размер окна, за ним снимок страницы).
// SDK кладёт перед ним свои события (загрузка страницы, скрипты) — с ними проигрыватель стартует
// раньше снимка и показывает пустую рамку, пока не нажмут «играть»
export function playable(events: RrwebEvent[]): RrwebEvent[] {
  const i = events.findIndex(e => e.type === RRWEB_META)
  return i > 0 ? events.slice(i) : events
}

// viewport — размер окна посетителя из первого события Meta (для пропорций проигрывателя)
export function viewport(events: RrwebEvent[]): { width: number, height: number } {
  const meta = events.find(e => e.type === RRWEB_META)
  const d = meta && isObject(meta.data) ? meta.data : {}
  return { width: Number(d.width) || 1280, height: Number(d.height) || 720 }
}

// formatOffset — 75 400 мс → «1:15»
export function formatOffset(ms: number): string {
  const total = Math.max(Math.floor(ms / 1000), 0)
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = String(total % 60).padStart(2, '0')
  return h ? `${h}:${String(m).padStart(2, '0')}:${s}` : `${m}:${s}`
}

// mapLimit — fn по элементам не больше limit одновременно; порядок результатов — как у items
export async function mapLimit<T, R>(items: T[], limit: number, fn: (item: T, i: number) => Promise<R>): Promise<R[]> {
  const result: R[] = Array.from({ length: items.length })
  let next = 0
  async function worker() {
    while (next < items.length) {
      const i = next++
      result[i] = await fn(items[i]!, i)
    }
  }
  await Promise.all(Array.from({ length: Math.min(limit, items.length) }, worker))
  return result
}
