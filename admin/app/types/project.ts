export interface ProjectMain {
  id: string
  created_at: string
  modified_at: string
  org_id: string
  slug: string
  name: string
  platform: string
}

export interface ProjectEdit {
  org_id?: string
  slug?: string
  name?: string
  platform?: string
}

export interface ProjectKeyMain {
  id: string
  created_at: string
  modified_at: string
  project_id: string
  public_key: string
  label: string
  active: boolean
  rate_limit_per_sec: number
  allowed_origin: string[]
  dsn: string
}

export interface ProjectKeyEdit {
  project_id?: string
  label?: string
  active?: boolean
  rate_limit_per_sec?: number
  allowed_origin?: { values: string[] }
}
