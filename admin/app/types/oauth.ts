// Вход MCP-клиентов по OAuth (ADR-020): страница согласия и подключённые приложения

// OAuthAuthorizeReq — параметры адреса страницы согласия как есть
export interface OAuthAuthorizeReq {
  response_type: string
  client_id: string
  redirect_uri: string
  state: string
  code_challenge: string
  code_challenge_method: string
  scope: string
  resource: string
}

export interface OAuthAuthorizeInfo {
  client_name: string
  redirect_uri: string
  redirect_host: string
}

export interface OAuthGrant {
  id: string
  client_name: string
  redirect_host: string
  created_at: string
  connected_at?: string
  // нет — приложение ещё не обращалось
  last_used_at?: string
}
