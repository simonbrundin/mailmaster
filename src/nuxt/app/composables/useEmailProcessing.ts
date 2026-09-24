export interface Email {
  uid: number
  subject: string
  from: string
  date: string
  has_attachment: boolean
  folder: string
}

export interface ProcessResult {
  uid: number
  subject: string
  action: string
  confidence: string
  motivation: string
  success: boolean
  error?: string
}

const FOLDERS = ['INBOX.Simon', 'INBOX', 'INBOX.Sent', 'INBOX.Drafts', 'INBOX.Trash'] as const
type Folder = typeof FOLDERS[number]

export function useEmailProcessing() {
  const config = useAppConfig()

  const emails = ref<Email[]>([])
  const loading = ref(false)
  const processing = ref(false)
  const error = ref<string | null>(null)
  const processResults = ref<ProcessResult[]>([])
  const showResults = ref(false)
  const folder = ref<Folder>('INBOX.Simon')

  async function fetchEmails() {
    loading.value = true
    error.value = null
    showResults.value = false

    try {
      const res = await $fetch<{ emails: Email[] }>(
        `${config.apiBase}/accounts/${config.account}/folders/${folder.value}/emails`,
        { query: { limit: 50 } }
      )
      emails.value = res.emails
    } catch (e: any) {
      error.value = e.message || 'Failed to fetch emails'
    } finally {
      loading.value = false
    }
  }

  async function processEmails() {
    processing.value = true
    error.value = null
    processResults.value = []
    showResults.value = false

    try {
      const res = await $fetch<{ results: ProcessResult[] }>(
        `${config.apiBase}/accounts/${config.account}/process`,
        {
          method: 'POST',
          body: { folder: folder.value, limit: 50 }
        }
      )
      processResults.value = res.results
      showResults.value = true
    } catch (e: any) {
      error.value = e.message || 'Failed to process emails'
    } finally {
      processing.value = false
    }
  }

  function closeResults() {
    showResults.value = false
  }

  function formatDate(dateStr: string): string {
    const date = new Date(dateStr)
    return date.toLocaleString('sv-SE', {
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit'
    })
  }

  return {
    emails,
    loading,
    processing,
    error,
    processResults,
    showResults,
    folder,
    folders: FOLDERS,
    fetchEmails,
    processEmails,
    closeResults,
    formatDate
  }
}
