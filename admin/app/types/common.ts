// Общие типы API (grpc-gateway, protojson): int64 — строками, время — RFC 3339

export interface ListParams {
  page: number
  page_size: number
  with_total_count?: boolean
  sort?: string[]
}

export interface PaginationInfo {
  page: string
  page_size: string
  total_count: string
}

export interface ErrorRep {
  code: string
  message: string
  fields?: Record<string, string>
}
