export interface IssueMain {
  id: string
  created_at: string
  project_id: string
  title: string
  culprit: string
  type: string
  value: string
  level: string
  platform: string
  status: string
  first_seen: string
  last_seen: string
  times_seen: string
  first_release: string
  last_release: string
}

export interface IssueListItem {
  issue: IssueMain
  events: string
  users: string
  sparkline: string[]
}

export interface IssueGetRep {
  issue: IssueMain
  events: string
  users: string
}

export interface IssueStatPoint {
  ts: string
  count: string
}

export interface TagValueCount {
  value: string
  count: string
}

export interface TagDist {
  key: string
  total: string
  values: TagValueCount[]
}

export type IssueSort = 'last_seen' | 'first_seen' | 'events' | 'users'
export type Period = '1h' | '24h' | '7d' | '14d' | '30d'
