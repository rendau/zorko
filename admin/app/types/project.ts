export interface ProjectMain {
  id: string
  created_at: string
  modified_at: string
  org_id: string
  slug: string
  name: string
  platform: string
  site_fetch_enabled?: boolean
  site_fetch_host?: string[]
  site_fetch_header_name?: string
  site_fetch_header_value_set?: boolean
  site_fetch_interval_min?: number
}

export interface ProjectEdit {
  org_id?: string
  slug?: string
  name?: string
  platform?: string
  site_fetch_enabled?: boolean
  site_fetch_host?: { values: string[] }
  site_fetch_header_name?: string
  // пусто — не менять; стереть — site_fetch_header_clear
  site_fetch_header_value?: string
  site_fetch_header_clear?: boolean
  site_fetch_interval_min?: number
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
