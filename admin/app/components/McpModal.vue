<script setup lang="ts">
import type { OAuthGrant } from '~/types/oauth'

// AI-агенты (ADR-020): адрес MCP-сервера, который вписывают в Claude, и приложения, которым
// человек разрешил вход под собой. Отключение действует сразу.
const open = defineModel<boolean>('open', { required: true })
const { t } = useI18n()
const { loading, run } = useApiAction()

const mcpUrl = useMcpUrl()
const grants = ref<OAuthGrant[]>([])
const loaded = ref(false)
const removing = ref<OAuthGrant | null>(null)
const confirmOpen = ref(false)

async function load() {
  const items = await run(() => oauthApi.grants())
  if (items) {
    grants.value = items
    loaded.value = true
  }
}

watch(open, (v) => {
  if (v) {
    loaded.value = false
    void load()
  }
})

function askRemove(grant: OAuthGrant) {
  removing.value = grant
  confirmOpen.value = true
}

async function remove() {
  const grant = removing.value
  if (!grant)
    return
  const ok = await run(() => oauthApi.removeGrant(grant.id).then(() => true), { success: t('mcp.disconnected', { name: grant.client_name }) })
  if (ok) {
    grants.value = grants.value.filter(g => g.id !== grant.id)
    confirmOpen.value = false
  }
}
</script>

<template>
  <UModal v-model:open="open" :title="t('mcp.title')" :description="t('mcp.description')">
    <template #body>
      <div class="space-y-5 text-sm">
        <div class="space-y-2">
          <div class="font-medium text-highlighted">{{ t('mcp.address') }}</div>
          <div class="flex items-center gap-1 rounded-md border border-default px-3 py-1.5">
            <code class="min-w-0 flex-1 font-mono break-all">{{ mcpUrl }}</code>
            <CopyButton :text="mcpUrl" />
          </div>
          <p class="text-muted">{{ t('mcp.howTo') }}</p>
        </div>

        <div class="space-y-2">
          <div class="font-medium text-highlighted">{{ t('mcp.connected') }}</div>
          <p v-if="!loaded" class="text-muted">{{ t('mcp.loading') }}</p>
          <p v-else-if="!grants.length" class="text-muted">{{ t('mcp.empty') }}</p>
          <ul v-else class="divide-y divide-default rounded-md border border-default">
            <li v-for="grant in grants" :key="grant.id" class="flex items-center gap-3 px-3 py-2">
              <div class="min-w-0 flex-1">
                <div class="font-medium break-words text-highlighted">{{ grant.client_name }}</div>
                <div class="text-xs text-muted break-words">
                  {{ grant.redirect_host }}
                  <template v-if="grant.connected_at">
                    · {{ t('mcp.connectedAt') }} <RelativeTime :value="grant.connected_at" />
                  </template>
                  ·
                  <template v-if="grant.last_used_at">
                    {{ t('mcp.lastUsed') }} <RelativeTime :value="grant.last_used_at" />
                  </template>
                  <template v-else>{{ t('mcp.neverUsed') }}</template>
                </div>
              </div>
              <UButton size="xs" color="error" variant="subtle" :label="t('mcp.disconnect')" @click="askRemove(grant)" />
            </li>
          </ul>
        </div>
      </div>
    </template>
    <template #footer>
      <div class="flex w-full justify-end">
        <UButton color="neutral" variant="ghost" :label="t('common.close')" @click="open = false" />
      </div>
    </template>
  </UModal>

  <ConfirmModal
    v-model:open="confirmOpen"
    :title="t('mcp.disconnectTitle', { name: removing?.client_name ?? '' })"
    :description="t('mcp.disconnectHint')"
    :confirm-label="t('mcp.disconnect')"
    :loading="loading"
    @confirm="remove"
  />
</template>
