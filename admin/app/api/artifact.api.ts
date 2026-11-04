import type { PaginationInfo } from '~/types/common'
import type { ArtifactBundleFile, ArtifactBundleMain, OrgTokenMain, SiteFetchMain } from '~/types/artifact'

export const artifactApi = {
  list: (projectId: string, q: string, page: number, pageSize: number) =>
    apiFetch<{ pagination_info: PaginationInfo, results: ArtifactBundleMain[] }>('/artifact_bundle', {
      query: { project_id: projectId, q: q || undefined, list_params: { page, page_size: pageSize } },
    }),
  files: (id: string) => apiFetch<{ results: ArtifactBundleFile[] }>(`/artifact_bundle/${id}/file`),
  remove: (id: string) => apiFetch(`/artifact_bundle/${id}`, { method: 'DELETE' }),
  siteFetch: (projectId: string, state?: 'ok' | 'error') =>
    apiFetch<{ results: SiteFetchMain[] }>('/site_fetch', { query: { project_id: projectId, state } }),
}

export const orgTokenApi = {
  list: (orgId: string) => apiFetch<{ results: OrgTokenMain[] }>('/org_token', { query: { org_id: orgId } }),
  create: (orgId: string, name: string) => apiFetch<{ item: OrgTokenMain, token: string }>('/org_token', { method: 'POST', body: { org_id: orgId, name } }),
  remove: (id: string) => apiFetch(`/org_token/${id}`, { method: 'DELETE' }),
}
