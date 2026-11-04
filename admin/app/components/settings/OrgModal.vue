<script setup lang="ts">
import type { OrgMain } from '~/types/org'

// Создать или переименовать организацию
const props = defineProps<{ org: OrgMain | null }>()
const open = defineModel<boolean>('open', { required: true })
const emit = defineEmits<{ saved: [id: string] }>()
const { t } = useI18n()
const { loading, run } = useApiAction()

const slug = ref('')
const name = ref('')
watch(open, (v) => {
  if (v) {
    slug.value = props.org?.slug ?? ''
    name.value = props.org?.name ?? ''
  }
})

async function save() {
  const body = { slug: slug.value.trim(), name: name.value.trim() }
  const id = await run(async () => (props.org ? (await orgApi.update(props.org.id, body), props.org.id) : (await orgApi.create(body)).id), { success: t('common.saved') })
  if (id) {
    open.value = false
    emit('saved', id)
  }
}
</script>

<template>
  <UModal v-model:open="open" :title="org ? t('settings.org.edit') : t('settings.org.create')">
    <template #body>
      <form id="org-form" class="space-y-4" @submit.prevent="save">
        <UFormField :label="t('settings.field.name')">
          <UInput v-model="name" class="w-full" autofocus />
        </UFormField>
        <UFormField :label="t('settings.field.slug')" :hint="t('settings.slugHint')">
          <UInput v-model="slug" class="w-full font-mono" />
        </UFormField>
      </form>
    </template>
    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton color="neutral" variant="ghost" :label="t('common.cancel')" @click="open = false" />
        <UButton type="submit" form="org-form" :label="t('common.save')" :loading="loading" :disabled="!slug" />
      </div>
    </template>
  </UModal>
</template>
