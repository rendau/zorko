import { apiErrorMessage } from '~/api/client'

// Обёртка над вызовом API для действий (создать, назначить, удалить, ...):
// loading-стейт, тост ошибки, опциональный success-тост.
export function useApiAction() {
  const toast = useToast()
  const { t } = useI18n()
  const loading = ref(false)

  async function run<T>(
    request: () => Promise<T>,
    options: { success?: string, silent?: boolean } = {},
  ): Promise<T | undefined> {
    loading.value = true
    try {
      const result = await request()
      if (options.success)
        toast.add({ title: options.success, color: 'success' })
      return result
    }
    catch (error) {
      if (!options.silent) {
        toast.add({
          title: t('common.error'),
          description: apiErrorMessage(error),
          color: 'error',
        })
      }
      return undefined
    }
    finally {
      loading.value = false
    }
  }

  return { loading, run }
}
