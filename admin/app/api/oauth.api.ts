import type { OAuthAuthorizeInfo, OAuthAuthorizeReq, OAuthGrant } from '~/types/oauth'

// Страница согласия для MCP-клиентов и подключённые приложения вошедшего человека (ADR-020)
export const oauthApi = {
  // запрос уходит на сервер как есть: клиента и адрес возврата проверяет он, до показа и до перехода
  describe: (body: OAuthAuthorizeReq) => apiFetch<OAuthAuthorizeInfo>('/oauth/authorize/describe', { method: 'POST', body }),
  authorize: (body: OAuthAuthorizeReq, approve: boolean) =>
    apiFetch<{ redirect_url: string }>('/oauth/authorize', { method: 'POST', body: { ...body, approve } }),
  grants: () => apiFetch<{ results?: OAuthGrant[] }>('/oauth/grant').then(rep => rep.results ?? []),
  removeGrant: (id: string) => apiFetch(`/oauth/grant/${encodeURIComponent(id)}`, { method: 'DELETE' }),
}
