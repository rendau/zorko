export interface EventSummary {
  event_id: string
  timestamp: string
  level: string
  release: string
  environment: string
  browser: string
  os: string
  device: string
  user_id: string
  user_email: string
  user_ip: string
  url: string
  title: string
  message: string
}

export interface EventMain {
  summary: EventSummary
  data: string
  prev_event_id: string
  next_event_id: string
}
