import { FetchError } from 'ofetch'
import type { ErrorRep } from '~/types/common'
import type { TokenPair } from '~/types/usr'

// HTTP-клиент API zorko (grpc-gateway):
// - query: вложенные объекты — через точку (list_params.page_size=50), массивы — повтором ключа,
//   undefined не отправляется; page — с 0;
// - ошибки — телом ErrorRep { code, message, fields }; 401 — нужен вход, 403 — нет прав;
// - access-токен живёт 15 минут: при 401 один раз меняем refresh на новую пару и повторяем запрос.

export type QueryValue = string | number | boolean | undefined | null
export type QueryParams = Record<string, QueryValue | QueryValue[] | Record<string, QueryValue | QueryValue[]>>

export interface ApiRequestOptions {
  method?: 'GET' | 'POST' | 'PUT' | 'DELETE'
  query?: QueryParams
  body?: unknown
  signal?: AbortSignal
}

const AUTH_PATHS = ['/usr/login', '/usr/token/refresh']

export function flattenQuery(params?: QueryParams): Record<string, string | string[]> {
  const out: Record<string, string | string[]> = {}
  if (!params)
    return out

  const assign = (key: string, value: QueryValue | QueryValue[]) => {
    if (value === undefined || value === null)
      return
    if (Array.isArray(value)) {
      const items = value.filter(v => v !== undefined && v !== null && v !== '').map(String)
      if (items.length)
        out[key] = items
    }
    else {
      out[key] = String(value)
    }
  }

  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === null)
      continue
    if (typeof value === 'object' && !Array.isArray(value)) {
      for (const [subKey, subValue] of Object.entries(value))
        assign(`${key}.${subKey}`, subValue)
    }
    else {
      assign(key, value)
    }
  }
  return out
}

// apiErrorMessage — сообщение из ErrorRep: код переводится словарём apiError.<код>, незнакомый — как есть
export function apiErrorMessage(error: unknown): string {
  if (error instanceof FetchError) {
    const rep = error.data as ErrorRep | undefined
    if (rep?.code) {
      const key = `apiError.${rep.code}`
      if (tfExists(key))
        return tf(key)
      return rep.message && rep.message !== rep.code ? rep.message : rep.code
    }
    return error.message
  }
  return String(error)
}

export function apiErrorCode(error: unknown): string {
  return error instanceof FetchError ? ((error.data as ErrorRep | undefined)?.code ?? '') : ''
}

export function apiErrorFields(error: unknown): Record<string, string> {
  return error instanceof FetchError ? ((error.data as ErrorRep | undefined)?.fields ?? {}) : {}
}

function isUnauthorized(error: unknown): boolean {
  return error instanceof FetchError && error.statusCode === 401
}

// один обмен refresh на все запросы, упавшие с 401 одновременно
let refreshing: Promise<boolean> | null = null

async function refreshTokens(): Promise<boolean> {
  const refresh = getStoredRefresh()
  if (!refresh)
    return false
  refreshing ??= $fetch<TokenPair>('/usr/token/refresh', {
    baseURL: useApiBaseUrl(),
    method: 'POST',
    body: { refresh_token: refresh },
  }).then((pair) => {
    storeTokens(pair.jwt, pair.refresh_token)
    return true
  }).catch(() => false).finally(() => {
    refreshing = null
  })
  return refreshing
}

function request<T>(path: string, options: ApiRequestOptions): Promise<T> {
  const jwt = getStoredJwt()
  return $fetch<T>(path, {
    baseURL: useApiBaseUrl(),
    method: options.method ?? 'GET',
    query: flattenQuery(options.query),
    body: options.body as Record<string, unknown> | undefined,
    signal: options.signal,
    headers: { Accept: 'application/json', ...(jwt ? { Authorization: `Bearer ${jwt}` } : {}) },
  })
}

export async function apiFetch<T>(path: string, options: ApiRequestOptions = {}): Promise<T> {
  try {
    return await request<T>(path, options)
  }
  catch (error) {
    if (!isUnauthorized(error) || AUTH_PATHS.includes(path))
      throw error

    if (await refreshTokens())
      return await request<T>(path, options)

    clearTokens()
    await navigateTo({ path: '/login', query: { redirect: useRoute().fullPath } })
    throw error
  }
}
