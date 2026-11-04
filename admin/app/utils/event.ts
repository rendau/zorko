// Разбор исходного JSON события Sentry (поле data из /event/{id}) в вид для карточки.
// Чистые функции без Nuxt: проверяются node --test (tests/event.test.mjs).
// Формат: develop.sentry.dev/sdk/data-model/event-payloads

export interface ContextLine {
  line: number
  text: string
  current: boolean
}

export interface FrameView {
  filename: string
  module: string
  function: string
  lineno: number
  colno: number
  inApp: boolean
  context: ContextLine[]
  // карты исходников: кадр переведён; иначе причина (код) из CI и с сайта
  symbolicated: boolean
  error: string
  siteError: string
}

export interface ExceptionView {
  type: string
  value: string
  mechanism: string
  handled: boolean | null
  synthetic: boolean
  // кадры в порядке показа: место падения первым
  frames: FrameView[]
  // исходный сжатый стек, если кадры переведены по картам
  rawFrames: FrameView[] | null
}

// SourcemapProblem — кадры, которые карты не перевели: сколько и главная причина
export interface SourcemapProblem {
  count: number
  error: string
  siteError: string
}

export interface BreadcrumbView {
  timestamp: string // ISO; пусто — SDK не прислал
  category: string
  type: string
  level: string
  message: string
  data: [string, string][]
}

export interface KeyValueGroup {
  key: string // browser, os, device, runtime, app, user, request, trace, sdk, …
  items: [string, string][]
}

export interface EventView {
  exceptions: ExceptionView[]
  breadcrumbs: BreadcrumbView[]
  tags: [string, string][]
  contexts: KeyValueGroup[]
  extra: [string, string][]
  message: string
}

type Json = Record<string, unknown>

const isObject = (v: unknown): v is Json => typeof v === 'object' && v !== null && !Array.isArray(v)

export function stringify(v: unknown): string {
  if (v === null || v === undefined)
    return ''
  if (typeof v === 'string')
    return v
  if (typeof v === 'number' || typeof v === 'boolean')
    return String(v)
  return JSON.stringify(v)
}

// values — {values: [...]} или сразу [...]
function values(v: unknown): unknown[] {
  if (Array.isArray(v))
    return v
  if (isObject(v) && Array.isArray(v.values))
    return v.values
  return []
}

function entries(v: unknown): [string, string][] {
  if (Array.isArray(v))
    return v.filter(p => Array.isArray(p) && p.length === 2).map(p => [stringify(p[0]), stringify(p[1])])
  if (isObject(v))
    return Object.entries(v).map(([k, x]) => [k, stringify(x)])
  return []
}

// isInApp — in_app от SDK, иначе по пути, как на сервере (service/grouping)
export function isInApp(f: Json): boolean {
  if (typeof f.in_app === 'boolean')
    return f.in_app
  const path = stringify(f.abs_path) || stringify(f.filename)
  if (!path || path === '<anonymous>' || path === '[native code]')
    return false
  return !(path.includes('node_modules')
    || path.startsWith('node:')
    || /^(chrome|moz|safari|safari-web)-extension:\/\//.test(path))
}

function frameView(f: Json): FrameView {
  const data = isObject(f.data) ? f.data : {}
  const lineno = Number(f.lineno) || 0
  const pre = Array.isArray(f.pre_context) ? f.pre_context.map(stringify) : []
  const post = Array.isArray(f.post_context) ? f.post_context.map(stringify) : []
  const context: ContextLine[] = []
  if (typeof f.context_line === 'string') {
    pre.forEach((text, i) => context.push({ line: lineno - pre.length + i, text, current: false }))
    context.push({ line: lineno, text: f.context_line, current: true })
    post.forEach((text, i) => context.push({ line: lineno + 1 + i, text, current: false }))
  }
  return {
    filename: stringify(f.filename) || stringify(f.abs_path),
    module: stringify(f.module),
    function: stringify(f.function),
    lineno,
    colno: Number(f.colno) || 0,
    inApp: isInApp(f),
    context,
    symbolicated: data.symbolicated === true,
    error: stringify(data.zorko_error),
    siteError: stringify(data.zorko_site_error),
  }
}

function exceptionView(e: unknown): ExceptionView | null {
  if (!isObject(e))
    return null
  const mechanism = isObject(e.mechanism) ? e.mechanism : {}
  const stack = isObject(e.stacktrace) && Array.isArray(e.stacktrace.frames) ? e.stacktrace.frames : []
  const raw = isObject(e.raw_stacktrace) && Array.isArray(e.raw_stacktrace.frames) ? e.raw_stacktrace.frames : null
  return {
    type: stringify(e.type),
    value: stringify(e.value),
    mechanism: stringify(mechanism.type),
    handled: typeof mechanism.handled === 'boolean' ? mechanism.handled : null,
    synthetic: mechanism.synthetic === true,
    frames: stack.filter(isObject).map(frameView).reverse(),
    rawFrames: raw ? raw.filter(isObject).map(frameView).reverse() : null,
  }
}

// sourcemapProblem — непереведённые кадры исключения (без своих кадров — все); нет — null
export function sourcemapProblem(e: ExceptionView): SourcemapProblem | null {
  const failed = e.frames.filter(f => f.error !== '')
  if (!failed.length)
    return null
  const counts = new Map<string, number>()
  for (const f of failed) {
    const key = `${f.error}|${f.siteError}`
    counts.set(key, (counts.get(key) ?? 0) + 1)
  }
  const [top] = [...counts.entries()].sort((a, b) => b[1] - a[1])
  const [error, siteError] = top![0].split('|')
  return { count: failed.length, error: error!, siteError: siteError! }
}

// timestamp — секунды (число) или строка → ISO
function isoTime(v: unknown): string {
  if (typeof v === 'number')
    return new Date(v * 1000).toISOString()
  if (typeof v === 'string') {
    const d = new Date(v)
    return Number.isNaN(d.getTime()) ? '' : d.toISOString()
  }
  return ''
}

function breadcrumbView(b: unknown): BreadcrumbView | null {
  if (!isObject(b))
    return null
  return {
    timestamp: isoTime(b.timestamp),
    category: stringify(b.category),
    type: stringify(b.type) || 'default',
    level: stringify(b.level) || 'info',
    message: stringify(b.message),
    data: entries(b.data),
  }
}

// порядок групп контекста в карточке
const CONTEXT_ORDER = ['user', 'request', 'browser', 'os', 'device', 'runtime', 'app', 'culture', 'trace', 'sdk']

export function parseEvent(raw: string): EventView {
  let ev: Json = {}
  try {
    const parsed: unknown = JSON.parse(raw)
    if (isObject(parsed))
      ev = parsed
  }
  catch {
    // битый JSON — пустая карточка, сырой текст покажет вкладка JSON
  }

  const contexts: KeyValueGroup[] = []
  if (isObject(ev.user))
    contexts.push({ key: 'user', items: entries(ev.user) })
  if (isObject(ev.request)) {
    const r = ev.request
    const items: [string, string][] = [['url', stringify(r.url)], ['method', stringify(r.method)]]
    if (r.query_string)
      items.push(['query', stringify(r.query_string)])
    for (const [k, v] of entries(r.headers))
      items.push([`header: ${k}`, v])
    contexts.push({ key: 'request', items: items.filter(([, v]) => v !== '') })
  }
  if (isObject(ev.contexts)) {
    for (const [key, value] of Object.entries(ev.contexts)) {
      if (isObject(value))
        contexts.push({ key, items: entries(value).filter(([k]) => k !== 'type') })
    }
  }
  if (isObject(ev.sdk))
    contexts.push({ key: 'sdk', items: entries({ name: ev.sdk.name, version: ev.sdk.version }) })
  contexts.sort((a, b) => rank(a.key) - rank(b.key))

  const logentry = isObject(ev.logentry) ? ev.logentry : {}
  const message = isObject(ev.message) ? stringify(ev.message.formatted || ev.message.message) : stringify(ev.message)

  return {
    // в карточке — снаружи внутрь: последнее исключение цепочки (пойманное) первым
    exceptions: values(ev.exception).map(exceptionView).filter((e): e is ExceptionView => e !== null).reverse(),
    breadcrumbs: values(ev.breadcrumbs).map(breadcrumbView).filter((b): b is BreadcrumbView => b !== null),
    tags: entries(ev.tags).sort(([a], [b]) => a.localeCompare(b)),
    contexts: contexts.filter(c => c.items.length > 0),
    extra: entries(ev.extra),
    message: stringify(logentry.formatted || logentry.message) || message,
  }
}

function rank(key: string): number {
  const i = CONTEXT_ORDER.indexOf(key)
  return i === -1 ? CONTEXT_ORDER.length : i
}

// relativeToError — «−3,2 с» от момента ошибки; пусто — времени нет
export function secondsBefore(crumb: string, error: string): number | null {
  if (!crumb || !error)
    return null
  return (new Date(crumb).getTime() - new Date(error).getTime()) / 1000
}
