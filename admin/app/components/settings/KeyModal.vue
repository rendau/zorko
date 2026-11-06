<script setup lang="ts">
import type { ProjectKeyMain } from '~/types/project'

// Создать или изменить ключ проекта: метка, вкл/выкл, предел в секунду, разрешённые сайты
const props = defineProps<{ projectKey: ProjectKeyMain | null, projectId: string }>()
const open = defineModel<boolean>('open', { required: true })
const emit = defineEmits<{ saved: [] }>()
const { t } = useI18n()
const { loading, run } = useApiAction()

const label = ref('')
const active = ref(true)
const rateLimit = ref(0)
const origins = ref('')
watch(open, (v) => {
  if (v) {
    label.value = props.projectKey?.label ?? ''
    active.value = props.projectKey?.active ?? true
    rateLimit.value = props.projectKey?.rate_limit_per_sec ?? 0
    origins.value = (props.projectKey?.allowed_origin ?? []).join('\n')
  }
})

const originList = computed(() => origins.value.split(/[\s,]+/).map(s => s.trim()).filter(Boolean))
const mobileAllowed = computed(() => originList.value.some(o => o.toLowerCase() === MOBILE_ORIGIN || o === '*'))

// Capacitor/Cordova шлют события с хоста localhost — дописываем его в непустой список
// (пустой список и так пускает всех, кнопка там не показывается)
function addMobile() {
  origins.value = [...originList.value, MOBILE_ORIGIN].join('\n')
}

async function save() {
  const body = {
    label: label.value.trim(),
    active: active.value,
    rate_limit_per_sec: Number(rateLimit.value) || 0,
    allowed_origin: { values: originList.value },
  }
  const ok = await run(async () => {
    if (props.projectKey)
      await projectKeyApi.update(props.projectKey.id, body)
    else
      await projectKeyApi.create({ ...body, project_id: props.projectId })
    return true
  }, { success: t('common.saved') })
  if (ok) {
    open.value = false
    emit('saved')
  }
}
</script>

<template>
  <UModal v-model:open="open" :title="projectKey ? t('settings.key.edit') : t('settings.key.create')">
    <template #body>
      <form id="key-form" class="space-y-4" @submit.prevent="save">
        <UFormField :label="t('settings.key.label')">
          <UInput v-model="label" class="w-full" autofocus />
        </UFormField>
        <UFormField :label="t('settings.key.active')">
          <USwitch v-model="active" />
        </UFormField>
        <UFormField :label="t('settings.key.rateLimit')" :hint="t('settings.key.rateLimitHint')">
          <UInput v-model.number="rateLimit" type="number" min="0" class="w-40" />
        </UFormField>
        <UFormField :label="t('settings.key.origins')" :hint="t('settings.key.originsHint')">
          <UTextarea v-model="origins" :rows="3" class="w-full font-mono" />
          <div class="mt-2 space-y-2">
            <p class="text-xs text-muted">{{ t('settings.key.mobileHint') }}</p>
            <UButton
              v-if="originList.length && !mobileAllowed"
              size="xs"
              color="neutral"
              variant="outline"
              icon="i-lucide-smartphone"
              :label="t('settings.key.addMobile')"
              @click="addMobile"
            />
            <p v-else-if="originList.length" class="flex items-center gap-1 text-xs text-success">
              <UIcon name="i-lucide-check" class="size-3.5" />
              {{ t('settings.key.mobileAdded') }}
            </p>
          </div>
        </UFormField>
      </form>
    </template>
    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton color="neutral" variant="ghost" :label="t('common.cancel')" @click="open = false" />
        <UButton type="submit" form="key-form" :label="t('common.save')" :loading="loading" />
      </div>
    </template>
  </UModal>
</template>
