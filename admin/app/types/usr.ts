export interface UsrMain {
  id: string
  created_at: string
  modified_at: string
  active: boolean
  is_admin: boolean
  name: string
  username: string
  // проекты, которые видит участник; пусто — все. Админу не ограничивает.
  project_ids?: string[]
}

export interface UsrCreate {
  active?: boolean
  is_admin?: boolean
  name?: string
  username?: string
  password?: string
  project_ids?: string[]
}

// UsrUpdate — список проектов в обёртке: не передан — не меняется
export interface UsrUpdate extends Omit<UsrCreate, 'project_ids'> {
  project_ids?: { values: string[] }
}

export interface TokenPair {
  jwt: string
  refresh_token: string
}
