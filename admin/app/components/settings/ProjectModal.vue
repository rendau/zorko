<script setup lang="ts">
import type { ProjectMain } from '~/types/project'

// Создать или изменить проект текущей организации
const props = defineProps<{ project: ProjectMain | null, orgId: string }>()
const open = defineModel<boolean>('open', { required: true })
const emit = defineEmits<{ saved: [id: string] }>()
const { t } = useI18n()
const { loading, run } = useApiAction()

const slug = ref('')
const name = ref('')
const platform = ref('vue')
watch(open, (v) => {
  if (v) {
    slug.value = props.project?.slug ?? ''
    name.value = props.project?.name ?? ''
    platform.value = props.project?.platform || 'vue'
  }
})

const platformItems = PLATFORMS.map(p => ({ label: p, value: p }))

async function save() {
  const body = { slug: slug.value.trim(), name: name.value.trim(), platform: platform.value }
  const id = await run(async () => {
    if (props.project) {
      await projectApi.update(props.project.id, body)
      return props.project.id
    }
    return (await projectApi.create({ ...body, org_id: props.orgId })).id
  }, { success: t('common.saved') })
  if (id) {
    open.value = false
    emit('saved', id)
  }
}
</script>

<template>
  <UModal v-model:open="open" :title="project ? t('settings.project.edit') : t('settings.project.create')">
    <template #body>
      <form id="project-form" class="space-y-4" @submit.prevent="save">
        <UFormField :label="t('settings.field.name')">
          <UInput v-model="name" class="w-full" autofocus />
        </UFormField>
        <UFormField :label="t('settings.field.slug')" :hint="t('settings.slugHint')">
          <UInput v-model="slug" class="w-full font-mono" />
        </UFormField>
        <UFormField :label="t('settings.field.platform')">
          <USelect v-model="platform" :items="platformItems" class="w-full" />
        </UFormField>
      </form>
    </template>
    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton color="neutral" variant="ghost" :label="t('common.cancel')" @click="open = false" />
        <UButton type="submit" form="project-form" :label="t('common.save')" :loading="loading" :disabled="!slug" />
      </div>
    </template>
  </UModal>
</template>
