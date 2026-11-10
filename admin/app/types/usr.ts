export interface UsrMain {
  id: string
  created_at: string
  modified_at: string
  active: boolean
  is_admin: boolean
  name: string
  username: string
  // проекты, которые видит участник (если не all_projects); пусто — ни одного. Админу не ограничивает.
  project_ids?: string[]
  all_projects?: boolean
  // привязан к внешнему провайдеру входа: логин, имя и роль админа приходят от него
  external?: boolean
}

export interface UsrCreate {
  active?: boolean
  is_admin?: boolean
  name?: string
  username?: string
  password?: string
  project_ids?: string[]
  all_projects?: boolean
}

// UsrUpdate — список проектов в обёртке: не передан — не меняется
export interface UsrUpdate extends Omit<UsrCreate, 'project_ids'> {
  project_ids?: { values: string[] }
}

export interface TokenPair {
  jwt: string
  refresh_token: string
}
