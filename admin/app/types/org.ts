export interface OrgMain {
  id: string
  created_at: string
  modified_at: string
  slug: string
  name: string
}

export interface OrgEdit {
  slug?: string
  name?: string
}
