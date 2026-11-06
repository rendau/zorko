export type ReplayType = 'buffer' | 'session'

export interface ReplaySummary {
  id: string
  project_id: string
  replay_type: ReplayType
  started_at: string
  finished_at: string
  duration_ms: string
  segments: number
  max_segment: number
  url: string
  error_count: number
  environment: string
  release: string
  user_id: string
  user_email: string
  user_ip: string
  browser: string
  os: string
  device: string
  sdk_name: string
  sdk_version: string
  // только в записях проблемы: первая ошибка проблемы в записи
  error_at?: string | null
}

export interface ReplayErrorMark {
  issue_id: string
  title: string
  event_id: string
  timestamp: string
}

export interface ReplayGetRep {
  summary: ReplaySummary
  errors: ReplayErrorMark[]
}

export interface ReplayFilter {
  orgId: string
  projectIds: string[]
  environment: string
  q: string
  // '' — любые; 'yes' / 'no'
  errors: '' | 'yes' | 'no'
  type: '' | ReplayType
}
