import type { OrgMain } from '~/types/org'
import type { ProjectMain } from '~/types/project'

const ORG_KEY = 'zorko:org'

// Текущая организация (выбор в меню) и её проекты; выбор помнит этот браузер
export function useOrg() {
  const orgs = useState<OrgMain[]>('org:list', () => [])
  const projects = useState<ProjectMain[]>('org:projects', () => [])
  const currentId = useState<string>('org:current', () => '')
  const loaded = useState<boolean>('org:loaded', () => false)

  const current = computed(() => orgs.value.find(o => o.id === currentId.value) ?? null)

  async function loadProjects() {
    projects.value = currentId.value ? (await projectApi.list(currentId.value)).results : []
  }

  async function load(force = false) {
    if (loaded.value && !force)
      return
    orgs.value = (await orgApi.list()).results
    let saved = ''
    try {
      saved = localStorage.getItem(ORG_KEY) ?? ''
    }
    catch {
      // хранилище недоступно — первая организация
    }
    if (!orgs.value.some(o => o.id === currentId.value))
      currentId.value = orgs.value.find(o => o.id === saved)?.id ?? orgs.value[0]?.id ?? ''
    await loadProjects()
    loaded.value = true
  }

  async function select(id: string) {
    currentId.value = id
    try {
      localStorage.setItem(ORG_KEY, id)
    }
    catch {
      // хранилище недоступно — выбор до перезагрузки
    }
    await loadProjects()
  }

  function projectName(id: string): string {
    const p = projects.value.find(x => x.id === id)
    return p ? (p.name || p.slug) : ''
  }

  return { orgs, projects, currentId, current, load, loadProjects, select, projectName }
}
