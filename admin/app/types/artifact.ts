// Карты исходников: архивы (из CI или с сайта), их файлы, история скачивания с сайта, токены CI

export interface ArtifactBundleMain {
  id: string
  created_at: string
  source: 'upload' | 'site'
  state: 'assembling' | 'ok' | 'error'
  detail: string
  release: string
  dist: string
  file_count: number
  size: string
  last_used_at?: string | null
}

export interface ArtifactBundleFile {
  url: string
  type: 'minified_source' | 'source_map' | 'source'
  debug_id: string
  sourcemap: string
  size: string
}

export interface SiteFetchMain {
  url: string
  state: 'ok' | 'error'
  detail: string
  fetched_at: string
}

export interface OrgTokenMain {
  id: string
  created_at: string
  org_id: string
  created_by_name: string
  name: string
  hint: string
  last_used_at?: string | null
}
