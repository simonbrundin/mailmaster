export interface Email {
  uid: number
  subject: string
  from: string
  date: string
  has_attachment: boolean
  folder: string
}

export interface SuggestedAction {
  type: string
  description: string
}

export interface Analysis {
  uid: number
  subject: string
  from: string
  folder: string
  matched: boolean
  rule: string
  condition: string
  description: string
  motivation: string
  confidence: string
  actions: SuggestedAction[]
  error?: string
}

export type DecisionState = 'pending' | 'approved' | 'denied' | 'error'

export interface Decision {
  state: DecisionState
  message?: string
}

export function useEmailProcessing() {
  const config = useAppConfig()
  const { currentAccount, folder } = useAccounts()

  const emails = ref<Email[]>([])
  const loading = ref(false)
  const analyzing = ref(false)
  const error = ref<string | null>(null)
  const analyses = ref<Record<number, Analysis>>({})
  const decisions = ref<Record<number, Decision>>({})
  const decidingUid = ref<number | null>(null)

  // accountPath is the account name used in the API calls. It follows the
  // account selected in the UI.
  function accountPath(): string {
    return currentAccount.value || config.account
  }

  async function fetchEmails() {
    loading.value = true
    error.value = null

    try {
      const res = await $fetch<{ emails: Email[] }>(
        `${config.apiBase}/accounts/${encodeURIComponent(accountPath())}/folders/${encodeURIComponent(folder.value)}/emails`,
        { query: { limit: 50 } }
      )
      emails.value = res.emails
    } catch (e: any) {
      error.value = e.message || 'Failed to fetch emails'
    } finally {
      loading.value = false
    }
  }

  // analyzeEmails asks the AI to analyse the emails without executing any
  // action. The result is shown next to each email so the user can approve
  // or deny the proposed actions.
  async function analyzeEmails() {
    analyzing.value = true
    error.value = null
    analyses.value = {}
    decisions.value = {}

    try {
      const res = await $fetch<{ results?: Analysis[] }>(
        `${config.apiBase}/accounts/${encodeURIComponent(accountPath())}/analyze`,
        {
          method: 'POST',
          body: { folder: folder.value, limit: 10 }
        }
      )
      const map: Record<number, Analysis> = {}
      for (const analysis of res.results ?? []) {
        map[analysis.uid] = analysis
      }
      analyses.value = map
    } catch (e: any) {
      error.value = e.message || 'Failed to analyze emails'
    } finally {
      analyzing.value = false
    }
  }

  // decideEmail approves or denies the proposed actions for one email.
  async function decideEmail(uid: number, decision: 'approve' | 'deny') {
    decidingUid.value = uid
    error.value = null

    try {
      const res = await $fetch<{ success: boolean; message?: string }>(
        `${config.apiBase}/accounts/${encodeURIComponent(accountPath())}/decision`,
        {
          method: 'POST',
          body: { folder: folder.value, uid, decision }
        }
      )
      decisions.value = {
        ...decisions.value,
        [uid]: {
          state: res.success ? (decision === 'approve' ? 'approved' : 'denied') : 'error',
          message: res.message
        }
      }

      // The mail may have been archived or moved - refresh the list.
      if (res.success && decision === 'approve') {
        await fetchEmails()
      }
    } catch (e: any) {
      error.value = e.message || 'Failed to save the decision'
      decisions.value = {
        ...decisions.value,
        [uid]: { state: 'error', message: e.message || 'Failed to save the decision' }
      }
    } finally {
      decidingUid.value = null
    }
  }

  function analysisFor(uid: number): Analysis | undefined {
    return analyses.value[uid]
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
    analyzing,
    error,
    analyses,
    decisions,
    decidingUid,
    folder,
    fetchEmails,
    analyzeEmails,
    decideEmail,
    analysisFor,
    formatDate
  }
}
