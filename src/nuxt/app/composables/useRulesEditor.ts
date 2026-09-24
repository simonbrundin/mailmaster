export function useRulesEditor() {
  const config = useAppConfig()

  const showEditor = ref(false)
  const content = ref('')
  const loading = ref(false)
  const saving = ref(false)
  const error = ref<string | null>(null)
  const success = ref<string | null>(null)

  async function open() {
    showEditor.value = true
    error.value = null
    success.value = null
    loading.value = true

    try {
      const res = await $fetch<{ content: string }>(`${config.apiBase}/rules`)
      content.value = res.content
    } catch (e: any) {
      error.value = e.data?.error || e.message || 'Failed to load rules'
    } finally {
      loading.value = false
    }
  }

  function close() {
    showEditor.value = false
    error.value = null
    success.value = null
  }

  async function save() {
    saving.value = true
    error.value = null
    success.value = null

    try {
      const response = await $fetch<{ success: boolean; message: string; rulesCount?: number }>(
        `${config.apiBase}/rules`,
        {
          method: 'PUT',
          body: { content: content.value }
        }
      )
      console.log('Rules saved response:', JSON.stringify(response, null, 2))
      success.value = response.message || `Regler sparade! (${response.rulesCount || '?'} regler)`
    } catch (e: any) {
      console.error('Rules save error:', e)
      error.value = e.data?.error || e.message || 'Failed to save rules'
    } finally {
      saving.value = false
    }
  }

  function handleTab(e: KeyboardEvent) {
    const textarea = e.target as HTMLTextAreaElement
    const start = textarea.selectionStart
    const end = textarea.selectionEnd
    const value = textarea.value

    textarea.value = value.substring(0, start) + '  ' + value.substring(end)
    textarea.selectionStart = textarea.selectionEnd = start + 2
    content.value = textarea.value
  }

  return {
    showEditor,
    content,
    loading,
    saving,
    error,
    success,
    open,
    close,
    save,
    handleTab
  }
}
