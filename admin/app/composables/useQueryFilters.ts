import type { LocationQuery, LocationQueryValue } from 'vue-router'

// Фильтры списка живут в query (?status=todo&roles=a&roles=b): переход в
// карточку и обратно (или ссылка коллеге) воспроизводит тот же срез.
// Значения, равные дефолту, из URL убираются; запись — одним replace, без
// записей в истории. Строки, массивы строк и булевы — по типу дефолта.

export type QueryFilterValue = string | boolean | string[]

// булев дефолт выводится литералом (false), а хранить нужно boolean
type Widen<V> = V extends boolean ? boolean : V
type QueryFilters<T> = { [K in keyof T]: Widen<T[K]> }

function readOne(v: LocationQueryValue | LocationQueryValue[] | undefined): string | undefined {
  if (Array.isArray(v))
    return readOne(v[0])
  return v == null ? undefined : String(v)
}

function readMany(v: LocationQueryValue | LocationQueryValue[] | undefined): string[] {
  if (v == null)
    return []
  return (Array.isArray(v) ? v : [v]).filter((x): x is string => typeof x === 'string' && x !== '')
}

function fromQuery<T extends Record<string, QueryFilterValue>>(defaults: T, query: LocationQuery): QueryFilters<T> {
  const out = { ...defaults } as Record<string, QueryFilterValue>
  for (const [key, def] of Object.entries(defaults)) {
    const raw = query[key]
    if (raw === undefined)
      continue
    if (Array.isArray(def))
      out[key] = readMany(raw)
    else if (typeof def === 'boolean')
      out[key] = readOne(raw) === '1'
    else
      out[key] = readOne(raw) ?? def
  }
  return out as QueryFilters<T>
}

function toQuery<T extends Record<string, QueryFilterValue>>(defaults: T, state: QueryFilters<T>): Record<string, string | string[] | undefined> {
  const out: Record<string, string | string[] | undefined> = {}
  for (const [key, def] of Object.entries(defaults)) {
    const value = state[key as keyof T] as QueryFilterValue
    if (Array.isArray(def))
      out[key] = (value as string[]).length ? (value as string[]) : undefined
    else if (typeof def === 'boolean')
      out[key] = value === def ? undefined : (value ? '1' : '0')
    else
      out[key] = value === def ? undefined : String(value)
  }
  return out
}

export function useQueryFilters<T extends Record<string, QueryFilterValue>>(defaults: T) {
  const route = useRoute()
  const router = useRouter()

  const state = reactive(fromQuery(defaults, route.query)) as QueryFilters<T>

  watch(state, (value) => {
    const merged = { ...route.query, ...toQuery(defaults, value as QueryFilters<T>) }
    const next = Object.fromEntries(Object.entries(merged).filter(([, v]) => v !== undefined))
    router.replace({ query: next })
  }, { deep: true })

  function reset() {
    Object.assign(state, defaults)
  }

  // хоть один фильтр отличается от дефолта — показать кнопку сброса
  const dirty = computed(() => Object.entries(defaults).some(([key, def]) => {
    const value = state[key as keyof T] as QueryFilterValue
    return Array.isArray(def) ? (value as string[]).length > 0 : value !== def
  }))

  return { state, reset, dirty }
}
