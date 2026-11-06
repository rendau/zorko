import type { PaginationInfo } from '~/types/common'
import type { ProjectEdit, ProjectKeyEdit, ProjectKeyMain, ProjectMain } from '~/types/project'

export const projectApi = {
  list: (orgId: string) => apiFetch<{ pagination_info: PaginationInfo, results: ProjectMain[] }>('/project', { query: { org_id: orgId, list_params: { page: 0, page_size: 100 } } }),
  // listAll — проекты всех организаций (выбор проектов человека; только админу)
  listAll: () => apiFetch<{ pagination_info: PaginationInfo, results: ProjectMain[] }>('/project', { query: { list_params: { page: 0, page_size: 100 } } }),
  get: (id: string) => apiFetch<ProjectMain>(`/project/${id}`),
  create: (body: ProjectEdit) => apiFetch<{ id: string }>('/project', { method: 'POST', body }),
  update: (id: string, body: ProjectEdit) => apiFetch(`/project/${id}`, { method: 'PUT', body }),
  remove: (id: string) => apiFetch(`/project/${id}`, { method: 'DELETE' }),
}

export const projectKeyApi = {
  list: (projectId: string) => apiFetch<{ pagination_info: PaginationInfo, results: ProjectKeyMain[] }>('/project_key', { query: { project_id: projectId, list_params: { page: 0, page_size: 100 } } }),
  create: (body: ProjectKeyEdit) => apiFetch<{ id: string }>('/project_key', { method: 'POST', body }),
  update: (id: string, body: ProjectKeyEdit) => apiFetch(`/project_key/${id}`, { method: 'PUT', body }),
  remove: (id: string) => apiFetch(`/project_key/${id}`, { method: 'DELETE' }),
}
