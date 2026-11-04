import type { PaginationInfo } from '~/types/common'
import type { EventMain, EventSummary } from '~/types/event'
import type { IssueGetRep, IssueListItem, IssueSort, IssueStatPoint, Period, TagDist } from '~/types/issue'

export interface IssueScope {
  orgId: string
  projectIds: string[]
  environment: string
  period: Period
}

function scopeQuery(s: IssueScope) {
  return { org_id: s.orgId, project_ids: s.projectIds, environment: s.environment || undefined, period: s.period }
}

export const issueApi = {
  list: (scope: IssueScope, q: string, sort: IssueSort, page: number, pageSize: number, signal?: AbortSignal) =>
    apiFetch<{ pagination_info: PaginationInfo, results: IssueListItem[], truncated: boolean }>('/issue', {
      query: { ...scopeQuery(scope), q: q || undefined, sort, list_params: { page, page_size: pageSize } },
      signal,
    }),
  get: (id: string, period: Period, environment: string) =>
    apiFetch<IssueGetRep>(`/issue/${id}`, { query: { period, environment: environment || undefined } }),
  stat: (id: string, period: Period, interval: 'hour' | 'day', environment: string) =>
    apiFetch<{ points: IssueStatPoint[] }>(`/issue/${id}/stat`, { query: { period, interval, environment: environment || undefined } }),
  tags: (id: string, period: Period, environment: string) =>
    apiFetch<{ tags: TagDist[] }>(`/issue/${id}/tag`, { query: { period, environment: environment || undefined } }),
}

export const eventApi = {
  list: (issueId: string, q: string, page: number, pageSize: number) =>
    apiFetch<{ pagination_info: PaginationInfo, results: EventSummary[] }>('/event', {
      query: { issue_id: issueId, q: q || undefined, list_params: { page, page_size: pageSize } },
    }),
  // eventId — id, 'latest' или 'oldest'
  get: (issueId: string, eventId: string) => apiFetch<EventMain>(`/event/${eventId}`, { query: { issue_id: issueId } }),
}

export const tagApi = {
  keys: (scope: IssueScope) => apiFetch<{ keys: string[] }>('/tag/key', { query: scopeQuery(scope) }),
  values: (scope: IssueScope, key: string, prefix: string) =>
    apiFetch<{ values: string[] }>('/tag/value', { query: { ...scopeQuery(scope), key, prefix } }),
}
