import type { PaginationInfo } from '~/types/common'
import type { ReplayFilter, ReplayGetRep, ReplaySummary } from '~/types/replay'

export const replayApi = {
  list: (f: ReplayFilter, page: number, pageSize: number, signal?: AbortSignal) =>
    apiFetch<{ pagination_info: PaginationInfo, results: ReplaySummary[] }>('/replay', {
      query: {
        org_id: f.orgId,
        project_ids: f.projectIds,
        environment: f.environment || undefined,
        q: f.q || undefined,
        has_errors: f.errors === '' ? undefined : f.errors === 'yes',
        replay_type: f.type || undefined,
        list_params: { page, page_size: pageSize },
      },
      signal,
    }),
  get: (projectId: string, id: string) => apiFetch<ReplayGetRep>(`/replay/${id}`, { query: { project_id: projectId } }),
  issueList: (issueId: string, page: number, pageSize: number) =>
    apiFetch<{ pagination_info: PaginationInfo, results: ReplaySummary[] }>(`/issue/${issueId}/replay`, {
      query: { list_params: { page, page_size: pageSize } },
    }),
  // segment — тело сегмента как есть (zlib): распаковывает браузер (ADR-015)
  segment: (projectId: string, id: string, n: number, signal?: AbortSignal) =>
    apiFetch<ArrayBuffer>(`/replay/${id}/segment/${n}`, { query: { project_id: projectId }, responseType: 'arrayBuffer', signal }),
}
