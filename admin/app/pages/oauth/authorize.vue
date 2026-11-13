<script setup lang="ts">
import type { OAuthAuthorizeInfo, OAuthAuthorizeReq } from '~/types/oauth'

definePageMeta({ layout: 'auth' })

// Страница согласия (ADR-020): MCP-клиент (приложение Claude, Claude Code) открывает её в браузере,
// человек входит в zorko как обычно и разрешает или запрещает подключение. Без входа сюда не
// попасть — middleware уводит на /login и возвращает обратно с теми же параметрами.
const { t } = useI18n()
const route = useRoute()
const auth = useAuth()

function param(name: string): string {
  const value = route.query[name]
  return typeof value === 'string' ? value : ''
}

const request = computed<OAuthAuthorizeReq>(() => ({
  response_type: param('response_type'),
  client_id: param('client_id'),
  redirect_uri: param('redirect_uri'),
  state: param('state'),
  code_challenge: param('code_challenge'),
  code_challenge_method: param('code_challenge_method'),
  scope: param('scope'),
  resource: param('resource'),
}))

const info = ref<OAuthAuthorizeInfo | null>(null)
const loading = ref(true)
const error = ref('')
const deciding = ref<'' | 'allow' | 'deny'>('')
const leaving = ref(false)

const username = computed(() => auth.user.value?.username || auth.user.value?.name || '')

async function decide(approve: boolean) {
  deciding.value = approve ? 'allow' : 'deny'
  error.value = ''
  try {
    const rep = await oauthApi.authorize(request.value, approve)
    leaving.value = true
    window.location.assign(rep.redirect_url)
  }
  catch (e) {
    error.value = apiErrorMessage(e)
    deciding.value = ''
  }
}

onMounted(async () => {
  try {
    info.value = await oauthApi.describe(request.value)
  }
  catch (e) {
    error.value = apiErrorMessage(e)
  }
  finally {
    loading.value = false
  }
})
</script>

<template>
  <div class="space-y-4">
    <h1 class="text-base font-semibold text-highlighted">{{ t('oauth.title') }}</h1>

    <p v-if="loading" class="text-sm text-muted">{{ t('oauth.checking') }}</p>
    <p v-else-if="leaving" class="text-sm text-muted">{{ t('oauth.returning', { host: info?.redirect_host ?? '' }) }}</p>

    <template v-else-if="info">
      <i18n-t keypath="oauth.lead" tag="p" class="text-sm break-words">
        <template #client>
          <strong class="text-highlighted">{{ info.client_name }}</strong>
        </template>
        <template #user>
          <strong class="text-highlighted">{{ username }}</strong>
        </template>
      </i18n-t>

      <ul class="space-y-2 rounded-md border border-default p-3 text-sm">
        <li class="flex gap-2">
          <UIcon name="i-lucide-eye" class="mt-0.5 size-4 shrink-0 text-muted" />
          <span>{{ t('oauth.scopeRead') }}</span>
        </li>
        <li class="flex gap-2">
          <UIcon name="i-lucide-lock" class="mt-0.5 size-4 shrink-0 text-muted" />
          <span>{{ t('oauth.scopeNoWrite') }}</span>
        </li>
      </ul>

      <i18n-t keypath="oauth.note" tag="p" class="text-xs text-muted break-words">
        <template #host>
          <code class="font-mono">{{ info.redirect_host }}</code>
        </template>
      </i18n-t>

      <UAlert v-if="error" color="error" variant="subtle" :description="error" />

      <div class="flex justify-end gap-2">
        <UButton color="neutral" variant="outline" :label="t('oauth.deny')" :loading="deciding === 'deny'" :disabled="!!deciding" @click="decide(false)" />
        <UButton icon="i-lucide-check" :label="t('oauth.allow')" :loading="deciding === 'allow'" :disabled="!!deciding" @click="decide(true)" />
      </div>
    </template>

    <template v-else>
      <UAlert color="error" variant="subtle" :title="t('oauth.invalid')" :description="error" />
      <UButton block color="neutral" variant="outline" :label="t('oauth.toApp')" to="/issues" />
    </template>
  </div>
</template>
