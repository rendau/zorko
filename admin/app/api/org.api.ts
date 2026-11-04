import type { PaginationInfo } from '~/types/common'
import type { OrgEdit, OrgMain } from '~/types/org'

export const orgApi = {
  list: () => apiFetch<{ pagination_info: PaginationInfo, results: OrgMain[] }>('/org', { query: { list_params: { page: 0, page_size: 100 } } }),
  create: (body: OrgEdit) => apiFetch<{ id: string }>('/org', { method: 'POST', body }),
  update: (id: string, body: OrgEdit) => apiFetch(`/org/${id}`, { method: 'PUT', body }),
  remove: (id: string) => apiFetch(`/org/${id}`, { method: 'DELETE' }),
}
