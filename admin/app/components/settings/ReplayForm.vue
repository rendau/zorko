<script setup lang="ts">
import type { ProjectMain, ReplayMode } from '~/types/project'

// Запись сессий (ADR-013): режим приёма, доля сессий для примера подключения, предел в час.
// Записывать или нет, решает SDK в браузере; zorko может только не принять запись и остановить
// её (заголовок для SDK) — поэтому режим и предел действуют сразу, а новая доля — с релизом сайта.
const props = defineProps<{ project: ProjectMain, isAdmin: boolean }>()
const emit = defineEmits<{ saved: [] }>()
const { t } = useI18n()
const { loading, run } = useApiAction()

const mode = ref<ReplayMode>('off')
const ratePercent = ref(10)
const hourLimit = ref(1000)

function reset() {
  mode.value = props.project.replay_mode || 'off'
  ratePercent.value = Math.round((props.project.replay_session_rate ?? 0.1) * 1000) / 10
  hourLimit.value = props.project.replay_hour_limit || 1000
}
watch(() => props.project, reset, { immediate: true })

const modeItems = computed(() => (['off', 'error', 'session'] as ReplayMode[]).map(m => ({
  label: t(`settings.replay.mode.${m}`),
  description: t(`settings.replay.modeHint.${m}`),
  value: m,
})))

async function save() {
  const ok = await run(async () => {
    await projectApi.update(props.project.id, {
      replay_mode: mode.value,
      replay_session_rate: Math.min(Math.max(Number(ratePercent.value) || 0, 0), 100) / 100,
      replay_hour_limit: Number(hourLimit.value) || 1000,
    })
    return true
  }, { success: t('common.saved') })
  if (ok)
    emit('saved')
}

const available = computed(() => props.project.replay_available !== false)
const rateChanged = computed(() => Math.abs((Number(ratePercent.value) || 0) / 100 - (props.project.replay_session_rate ?? 0.1)) > 0.0001)
</script>

<template>
  <div class="space-y-4">
    <UAlert v-if="!available" color="warning" variant="subtle" icon="i-lucide-hard-drive" :description="t('settings.replay.unavailable')" />
    <form class="space-y-4" @submit.prevent="save">
      <UFormField :label="t('settings.replay.modeLabel')">
        <URadioGroup v-model="mode" :items="modeItems" :disabled="!isAdmin || !available" />
      </UFormField>
      <div class="flex flex-wrap gap-6">
        <UFormField v-if="mode === 'session'" :label="t('settings.replay.rate')" :description="t('settings.replay.rateHint')">
          <UInput v-model.number="ratePercent" type="number" min="0" max="100" step="0.1" class="w-32" :disabled="!isAdmin">
            <template #trailing>
              <span class="text-xs text-muted">%</span>
            </template>
          </UInput>
        </UFormField>
        <UFormField v-if="mode !== 'off'" :label="t('settings.replay.hourLimit')" :description="t('settings.replay.hourLimitHint')">
          <UInput v-model.number="hourLimit" type="number" min="1" max="1000000" class="w-32" :disabled="!isAdmin" />
        </UFormField>
      </div>
      <UAlert v-if="mode === 'session' && rateChanged && isAdmin" color="info" variant="subtle" icon="i-lucide-rocket" :description="t('settings.replay.rateRelease')" />
      <p v-if="mode !== 'off'" class="text-xs text-muted">{{ t('settings.replay.privacy') }}</p>
      <p v-if="mode !== 'off'" class="text-xs text-muted">{{ t('settings.replay.csp') }}</p>
      <UButton v-if="isAdmin && available" type="submit" :label="t('common.save')" :loading="loading" />
    </form>
  </div>
</template>
