<script setup lang="ts">
import type { SiteFetchMain } from '~/types/artifact'
import type { ProjectMain } from '~/types/project'

// Карты с сайта: запасной путь, если из CI карты нет. Выключено по умолчанию; правит админ,
// участник видит настройки без правки. Ниже — последние неудачные адреса с причиной.
const props = defineProps<{ project: ProjectMain, isAdmin: boolean }>()
const emit = defineEmits<{ saved: [] }>()
const { t, te } = useI18n()
const { loading, run } = useApiAction()

const enabled = ref(false)
const hosts = ref('')
const headerName = ref('')
const headerValue = ref('')
const headerClear = ref(false)
const interval = ref(60)

function reset() {
  enabled.value = props.project.site_fetch_enabled ?? false
  hosts.value = (props.project.site_fetch_host ?? []).join('\n')
  headerName.value = props.project.site_fetch_header_name ?? ''
  headerValue.value = ''
  headerClear.value = false
  interval.value = props.project.site_fetch_interval_min || 60
}
watch(() => props.project, reset, { immediate: true })

async function save() {
  const ok = await run(async () => {
    await projectApi.update(props.project.id, {
      site_fetch_enabled: enabled.value,
      site_fetch_host: { values: hosts.value.split(/[\s,]+/).map(s => s.trim()).filter(Boolean) },
      site_fetch_header_name: headerName.value.trim(),
      ...(headerValue.value ? { site_fetch_header_value: headerValue.value } : {}),
      site_fetch_header_clear: headerClear.value,
      site_fetch_interval_min: Number(interval.value) || 60,
    })
    return true
  }, { success: t('common.saved') })
  if (ok)
    emit('saved')
}

const failures = ref<SiteFetchMain[]>([])
async function loadFailures() {
  try {
    failures.value = (await artifactApi.siteFetch(props.project.id, 'error')).results
  }
  catch {
    failures.value = []
  }
}
watch(() => props.project.id, loadFailures, { immediate: true })

function codeText(code: string): string {
  const http = /^site_http_(\d+)$/.exec(code)
  if (http)
    return t('sourcemap.code.site_http', { status: http[1] })
  return te(`sourcemap.code.${code}`) ? t(`sourcemap.code.${code}`) : code
}
</script>

<template>
  <div class="space-y-4">
    <form id="site-fetch-form" class="space-y-4" @submit.prevent="save">
      <UFormField :label="t('settings.siteFetch.enabled')" :description="t('settings.siteFetch.enabledHint')">
        <USwitch v-model="enabled" :disabled="!isAdmin" />
      </UFormField>
      <UFormField :label="t('settings.siteFetch.hosts')" :hint="t('settings.siteFetch.hostsHint')">
        <UTextarea v-model="hosts" :rows="2" class="w-full max-w-md font-mono" :disabled="!isAdmin" placeholder="admin.mdev.kz" />
      </UFormField>
      <div class="grid max-w-2xl gap-4 sm:grid-cols-2">
        <UFormField :label="t('settings.siteFetch.headerName')" :hint="t('settings.siteFetch.optional')">
          <UInput v-model="headerName" class="w-full font-mono" :disabled="!isAdmin" placeholder="X-Zorko-Token" />
        </UFormField>
        <UFormField :label="t('settings.siteFetch.headerValue')" :hint="project.site_fetch_header_value_set ? t('settings.siteFetch.headerSet') : t('settings.siteFetch.optional')">
          <UInput v-model="headerValue" type="password" autocomplete="new-password" class="w-full" :disabled="!isAdmin || headerClear" />
        </UFormField>
      </div>
      <UCheckbox v-if="project.site_fetch_header_value_set && isAdmin" v-model="headerClear" :label="t('settings.siteFetch.headerClear')" />
      <UFormField :label="t('settings.siteFetch.interval')" :hint="t('settings.siteFetch.intervalHint')">
        <UInput v-model.number="interval" type="number" min="1" max="10080" class="w-32" :disabled="!isAdmin" />
      </UFormField>
      <UButton v-if="isAdmin" type="submit" :label="t('common.save')" :loading="loading" />
    </form>

    <div v-if="failures.length" class="space-y-2">
      <h4 class="text-xs font-semibold text-muted">{{ t('settings.siteFetch.failures') }}</h4>
      <ul class="divide-y divide-default rounded-lg border border-default text-xs">
        <li v-for="f in failures" :key="f.url" class="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2">
          <span class="min-w-0 flex-1 break-all font-mono">{{ f.url }}</span>
          <UBadge color="warning" variant="subtle" size="sm">{{ codeText(f.detail) }}</UBadge>
          <RelativeTime :value="f.fetched_at" class="text-dimmed" />
        </li>
      </ul>
    </div>
  </div>
</template>
