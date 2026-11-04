<script setup lang="ts">
import type { OrgTokenMain } from '~/types/artifact'

// Токены CI организации (только админ): заливка карт и релизов из сборки. Значение показывается один раз.
const props = defineProps<{ orgId: string }>()
const { t } = useI18n()
const { loading, run } = useApiAction()

const items = ref<OrgTokenMain[]>([])
const error = ref('')
async function load() {
  error.value = ''
  try {
    items.value = (await orgTokenApi.list(props.orgId)).results
  }
  catch (e) {
    error.value = apiErrorMessage(e)
  }
}
watch(() => props.orgId, load, { immediate: true })

const modal = ref(false)
const name = ref('')
const created = ref('')
function openCreate() {
  name.value = ''
  created.value = ''
  modal.value = true
}
async function create() {
  const rep = await run(() => orgTokenApi.create(props.orgId, name.value.trim()))
  if (rep) {
    created.value = rep.token
    await load()
  }
}

const confirm = ref<OrgTokenMain | null>(null)
async function remove() {
  const tok = confirm.value
  if (tok && await run(() => orgTokenApi.remove(tok.id), { success: t('common.deleted') }) !== undefined) {
    confirm.value = null
    await load()
  }
}
</script>

<template>
  <div class="space-y-3">
    <div class="flex items-center justify-between gap-2">
      <div>
        <h3 class="text-sm font-semibold text-highlighted">{{ t('settings.tokens.title') }}</h3>
        <p class="text-xs text-muted">{{ t('settings.tokens.description') }}</p>
      </div>
      <UButton icon="i-lucide-plus" size="sm" :label="t('settings.tokens.create')" @click="openCreate" />
    </div>
    <LoadAlert :title="t('settings.tokens.loadError')" :error="error" @retry="load" />
    <EmptyState v-if="!error && !items.length" icon="i-lucide-key-square" :title="t('settings.tokens.empty')" />
    <div v-else-if="items.length" class="overflow-x-auto rounded-lg border border-default">
      <table class="w-full min-w-[560px] text-sm">
        <thead class="bg-elevated/50 text-left text-xs text-muted">
          <tr>
            <th class="px-3 py-2 font-medium">{{ t('settings.tokens.name') }}</th>
            <th class="px-3 py-2 font-medium">{{ t('settings.tokens.token') }}</th>
            <th class="px-3 py-2 font-medium">{{ t('settings.tokens.createdBy') }}</th>
            <th class="px-3 py-2 font-medium">{{ t('settings.tokens.used') }}</th>
            <th class="px-3 py-2" />
          </tr>
        </thead>
        <tbody class="divide-y divide-default">
          <tr v-for="tok in items" :key="tok.id">
            <td class="px-3 py-2 text-highlighted">{{ tok.name }}</td>
            <td class="px-3 py-2 font-mono text-xs">sntrys_…{{ tok.hint }}</td>
            <td class="px-3 py-2 text-xs">
              {{ tok.created_by_name || '—' }} · <RelativeTime :value="tok.created_at" />
            </td>
            <td class="px-3 py-2 text-xs">
              <RelativeTime v-if="tok.last_used_at" :value="tok.last_used_at" />
              <span v-else class="text-dimmed">{{ t('settings.tokens.neverUsed') }}</span>
            </td>
            <td class="px-3 py-2 text-right">
              <UButton size="xs" color="error" variant="ghost" icon="i-lucide-trash-2" :aria-label="t('common.delete')" @click="confirm = tok" />
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <UModal v-model:open="modal" :title="t('settings.tokens.create')" :dismissible="!created">
      <template #body>
        <form v-if="!created" id="token-form" class="space-y-4" @submit.prevent="create">
          <UFormField :label="t('settings.tokens.name')" :hint="t('settings.tokens.nameHint')">
            <UInput v-model="name" class="w-full" autofocus maxlength="100" />
          </UFormField>
        </form>
        <div v-else class="space-y-3">
          <UAlert color="warning" variant="subtle" icon="i-lucide-triangle-alert" :title="t('settings.tokens.onceTitle')" :description="t('settings.tokens.onceDescription')" />
          <div class="flex items-center gap-1 rounded-md border border-default bg-elevated/40 px-2 py-1">
            <code class="min-w-0 flex-1 break-all font-mono text-xs">{{ created }}</code>
            <CopyButton :text="created" />
          </div>
          <p class="text-xs text-muted">{{ t('settings.tokens.usage') }}</p>
        </div>
      </template>
      <template #footer>
        <div class="flex w-full justify-end gap-2">
          <template v-if="!created">
            <UButton color="neutral" variant="ghost" :label="t('common.cancel')" @click="modal = false" />
            <UButton type="submit" form="token-form" :label="t('settings.tokens.createSubmit')" :loading="loading" :disabled="!name.trim()" />
          </template>
          <UButton v-else :label="t('settings.tokens.done')" @click="modal = false" />
        </div>
      </template>
    </UModal>

    <ConfirmModal
      :open="!!confirm"
      :title="t('settings.tokens.deleteTitle')"
      :description="t('settings.tokens.deleteDescription')"
      :loading="loading"
      @update:open="v => { if (!v) confirm = null }"
      @confirm="remove"
    />
  </div>
</template>
