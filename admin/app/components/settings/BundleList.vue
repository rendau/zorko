<script setup lang="ts">
import type { ArtifactBundleFile, ArtifactBundleMain } from '~/types/artifact'

// Залитые карты проекта: архивы из CI и скачанное с сайта; поиск по релизу или метке, файлы архива,
// удаление (админ)
const props = defineProps<{ projectId: string, isAdmin: boolean }>()
const { t } = useI18n()
const { loading: removing, run } = useApiAction()

const PAGE_SIZE = 20
const q = ref('')
const page = ref(1)
const items = ref<ArtifactBundleMain[]>([])
const total = ref(0)
const loading = ref(false)
const error = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    const rep = await artifactApi.list(props.projectId, q.value.trim(), page.value - 1, PAGE_SIZE)
    items.value = rep.results
    total.value = Number(rep.pagination_info.total_count)
  }
  catch (e) {
    error.value = apiErrorMessage(e)
  }
  finally {
    loading.value = false
  }
}
watch(() => props.projectId, load, { immediate: true })
watch(page, load)

let searchTimer: ReturnType<typeof setTimeout> | undefined
watch(q, () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    if (page.value !== 1)
      page.value = 1
    else
      void load()
  }, 300)
})

// файлы архива — по раскрытию
const openId = ref('')
const files = ref<ArtifactBundleFile[]>([])
const filesLoading = ref(false)
async function toggleFiles(b: ArtifactBundleMain) {
  if (openId.value === b.id) {
    openId.value = ''
    return
  }
  openId.value = b.id
  filesLoading.value = true
  files.value = []
  try {
    files.value = (await artifactApi.files(b.id)).results
  }
  catch (e) {
    error.value = apiErrorMessage(e)
  }
  finally {
    filesLoading.value = false
  }
}

const confirm = ref<ArtifactBundleMain | null>(null)
async function remove() {
  const b = confirm.value
  if (b && await run(() => artifactApi.remove(b.id), { success: t('common.deleted') }) !== undefined) {
    confirm.value = null
    await load()
  }
}

const stateColor = (s: string) => s === 'ok' ? 'success' : s === 'error' ? 'error' : 'warning'
</script>

<template>
  <div class="space-y-3">
    <UInput v-model="q" icon="i-lucide-search" :placeholder="t('settings.maps.search')" class="w-full max-w-md" />
    <LoadAlert :title="t('settings.maps.loadError')" :error="error" @retry="load" />

    <EmptyState v-if="!loading && !error && !items.length" icon="i-lucide-map" :title="q ? t('settings.maps.notFound') : t('settings.maps.empty')" :description="q ? undefined : t('settings.maps.emptyHint')" />

    <div v-else-if="items.length" class="overflow-x-auto rounded-lg border border-default">
      <table class="w-full min-w-[720px] text-sm">
        <thead class="bg-elevated/50 text-left text-xs text-muted">
          <tr>
            <th class="px-3 py-2 font-medium">{{ t('settings.maps.col.uploaded') }}</th>
            <th class="px-3 py-2 font-medium">{{ t('settings.maps.col.release') }}</th>
            <th class="px-3 py-2 font-medium">{{ t('settings.maps.col.source') }}</th>
            <th class="px-3 py-2 text-right font-medium">{{ t('settings.maps.col.files') }}</th>
            <th class="px-3 py-2 text-right font-medium">{{ t('settings.maps.col.size') }}</th>
            <th class="px-3 py-2 font-medium">{{ t('settings.maps.col.used') }}</th>
            <th class="px-3 py-2" />
          </tr>
        </thead>
        <tbody class="divide-y divide-default">
          <template v-for="b in items" :key="b.id">
            <tr>
              <td class="px-3 py-2"><RelativeTime :value="b.created_at" /></td>
              <td class="px-3 py-2">
                <span v-if="b.release" class="font-mono text-xs">{{ b.release }}<template v-if="b.dist"> · {{ b.dist }}</template></span>
                <span v-else class="text-dimmed">—</span>
                <div v-if="b.state !== 'ok'" class="mt-0.5 flex items-center gap-1">
                  <UBadge :color="stateColor(b.state)" variant="subtle" size="sm">{{ t(`settings.maps.state.${b.state}`) }}</UBadge>
                  <span v-if="b.detail" class="text-xs text-muted">{{ b.detail }}</span>
                </div>
              </td>
              <td class="px-3 py-2">
                <UBadge :color="b.source === 'site' ? 'info' : 'neutral'" variant="subtle" size="sm">{{ t(`settings.maps.sourceKind.${b.source}`) }}</UBadge>
              </td>
              <td class="px-3 py-2 text-right tabular-nums">{{ formatNumber(b.file_count) }}</td>
              <td class="px-3 py-2 text-right tabular-nums whitespace-nowrap">{{ formatBytes(b.size) }}</td>
              <td class="px-3 py-2 text-xs">
                <RelativeTime v-if="b.last_used_at" :value="b.last_used_at" />
                <span v-else class="text-dimmed">{{ t('settings.maps.neverUsed') }}</span>
              </td>
              <td class="px-3 py-2 text-right whitespace-nowrap">
                <UButton size="xs" color="neutral" variant="ghost" :icon="openId === b.id ? 'i-lucide-chevron-up' : 'i-lucide-list'" :aria-label="t('settings.maps.showFiles')" :aria-expanded="openId === b.id" @click="toggleFiles(b)" />
                <UButton v-if="isAdmin" size="xs" color="error" variant="ghost" icon="i-lucide-trash-2" :aria-label="t('common.delete')" @click="confirm = b" />
              </td>
            </tr>
            <tr v-if="openId === b.id">
              <td colspan="7" class="bg-elevated/30 px-3 py-2">
                <USkeleton v-if="filesLoading" class="h-6 w-full" />
                <ul v-else class="space-y-1">
                  <li v-for="f in files" :key="f.url" class="flex flex-wrap items-center gap-x-3 gap-y-0.5 font-mono text-xs">
                    <span class="break-all text-highlighted">{{ f.url }}</span>
                    <UBadge color="neutral" variant="outline" size="sm">{{ t(`settings.maps.fileType.${f.type}`) }}</UBadge>
                    <span v-if="f.debug_id" class="text-dimmed" :title="t('settings.maps.debugId')">{{ f.debug_id }}</span>
                    <span class="text-dimmed">{{ formatBytes(f.size) }}</span>
                  </li>
                </ul>
              </td>
            </tr>
          </template>
        </tbody>
      </table>
    </div>

    <UPagination v-if="total > PAGE_SIZE" v-model:page="page" :total="total" :items-per-page="PAGE_SIZE" size="sm" />

    <ConfirmModal
      :open="!!confirm"
      :title="t('settings.maps.deleteTitle')"
      :description="t('settings.maps.deleteDescription')"
      :loading="removing"
      @update:open="v => { if (!v) confirm = null }"
      @confirm="remove"
    />
  </div>
</template>
