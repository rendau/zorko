import type { ListParams, PaginationInfo } from '~/types/common'
import type { TokenPair, UsrEdit, UsrMain } from '~/types/usr'

export const usrApi = {
  bootstrapStatus: () => apiFetch<{ bootstrap_required: boolean }>('/usr/bootstrap/status'),
  login: (username: string, password: string) => apiFetch<TokenPair>('/usr/login', { method: 'POST', body: { username, password } }),
  profile: () => apiFetch<UsrMain>('/usr/profile'),
  updateProfile: (body: { name?: string, username?: string, password?: string, current_password?: string }) =>
    apiFetch('/usr/profile', { method: 'PUT', body }),
  list: (list_params: ListParams) => apiFetch<{ pagination_info: PaginationInfo, results: UsrMain[] }>('/usr', { query: { list_params: { ...list_params, sort: undefined } } }),
  create: (body: UsrEdit) => apiFetch<{ id: string }>('/usr', { method: 'POST', body }),
  update: (id: string, body: UsrEdit) => apiFetch(`/usr/${id}`, { method: 'PUT', body }),
  remove: (id: string) => apiFetch(`/usr/${id}`, { method: 'DELETE' }),
}
