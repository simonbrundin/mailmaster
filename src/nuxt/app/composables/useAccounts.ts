import { pickAccount, pickFolder } from '~/utils/accounts'

export interface MailAccount {
  name: string
  username: string
  server: string
  folder: string
  auth_method: 'password' | 'oauth2'
  logged_in: boolean
  login_needed: boolean
  message?: string
}

export interface LoginChallenge {
  user_code: string
  verification_uri: string
  message?: string
  expires_in: number
  interval: number
}

export type LoginState = 'idle' | 'waiting' | 'complete' | 'expired' | 'error'

const ACCOUNT_KEY = 'mailmaster.account'

export function useAccounts() {
  const config = useAppConfig()

  const accounts = useState<MailAccount[]>('mailmaster.accounts', () => [])
  const currentAccount = useState<string>('mailmaster.currentAccount', () => '')
  const folders = useState<string[]>('mailmaster.folders', () => [])
  const folder = useState<string>('mailmaster.folder', () => '')
  const loadingAccounts = useState('mailmaster.loadingAccounts', () => false)
  const accountError = useState<string | null>('mailmaster.accountError', () => null)

  const loginState = useState<LoginState>('mailmaster.loginState', () => 'idle')
  const loginChallenge = useState<LoginChallenge | null>('mailmaster.loginChallenge', () => null)
  const loginMessage = useState<string | null>('mailmaster.loginMessage', () => null)

  let pollTimer: ReturnType<typeof setTimeout> | null = null

  const current = computed(() => accounts.value.find(a => a.name === currentAccount.value) ?? null)
  const needsLogin = computed(() => current.value?.login_needed === true)

  async function loadAccounts() {
    loadingAccounts.value = true
    accountError.value = null
    try {
      const res = await $fetch<{ accounts: MailAccount[] }>(`${config.apiBase}/accounts`)
      accounts.value = res.accounts ?? []

      const preferred = pickAccount(accounts.value, localStorage.getItem(ACCOUNT_KEY))
      if (preferred !== currentAccount.value) {
        currentAccount.value = preferred
        localStorage.setItem(ACCOUNT_KEY, preferred)
        await loadFolders()
      }
    } catch (e: any) {
      accountError.value = e.message || 'Failed to load accounts'
    } finally {
      loadingAccounts.value = false
    }
  }

  async function selectAccount(name: string) {
    if (currentAccount.value === name) return
    currentAccount.value = name
    localStorage.setItem(ACCOUNT_KEY, name)
    resetLogin()
    folder.value = ''
    await loadFolders()
  }

  async function loadFolders() {
    if (!currentAccount.value) {
      folders.value = []
      return
    }
    try {
      const res = await $fetch<{ folders: string[] }>(
        `${config.apiBase}/accounts/${encodeURIComponent(currentAccount.value)}/folders`
      )
      folders.value = res.folders ?? []
      accountError.value = null
      folder.value = pickFolder(folders.value, folder.value || defaultFolder())
    } catch (e: any) {
      // A failed folder listing usually means the account is not logged in.
      // The login button already shows that, so no error is shown here.
      folders.value = []
      await refreshAccountState()
    }
  }

  // defaultFolder returns the folder the account is configured to use, when
  // that folder exists on the server.
  function defaultFolder(): string {
    const account = current.value
    if (!account) return ''
    return folders.value.includes(account.folder) ? account.folder : ''
  }

  // selectFolder picks a folder explicitly, e.g. from the folder dropdown.
  function selectFolder(name: string) {
    folder.value = name
  }

  async function refreshAccountState() {
    if (!currentAccount.value) return
    try {
      const res = await $fetch<{ accounts: MailAccount[] }>(`${config.apiBase}/accounts`)
      accounts.value = res.accounts ?? []
    } catch {
      // Keep the previous state when the refresh fails.
    }
  }

  function resetLogin() {
    loginState.value = 'idle'
    loginChallenge.value = null
    loginMessage.value = null
    stopPolling()
  }

  function stopPolling() {
    if (pollTimer) {
      clearTimeout(pollTimer)
      pollTimer = null
    }
  }

  async function startLogin() {
    if (!currentAccount.value) return
    loginState.value = 'waiting'
    loginMessage.value = null
    try {
      const challenge = await $fetch<LoginChallenge>(
        `${config.apiBase}/accounts/${encodeURIComponent(currentAccount.value)}/auth/login`,
        { method: 'POST' }
      )
      loginChallenge.value = challenge
      loginMessage.value = challenge.message ?? null
      schedulePoll(0)
    } catch (e: any) {
      loginState.value = 'error'
      loginMessage.value = e.message || 'Failed to start login'
    }
  }

  function schedulePoll(delayMs: number) {
    stopPolling()
    pollTimer = setTimeout(poll, delayMs)
  }

  async function poll() {
    if (!currentAccount.value) return
    try {
      const res = await $fetch<{ status: string; logged_in: boolean; message?: string }>(
        `${config.apiBase}/accounts/${encodeURIComponent(currentAccount.value)}/auth/status`
      )

      if (res.status === 'complete') {
        loginState.value = 'complete'
        loginMessage.value = null
        await refreshAccountState()
        await loadFolders()
        return
      }
      if (res.status === 'expired') {
        loginState.value = 'expired'
        loginMessage.value = res.message ?? 'Koden gick ut.'
        return
      }
      if (res.status === 'error') {
        loginState.value = 'error'
        loginMessage.value = res.message ?? 'Inloggningen misslyckades.'
        return
      }

      // waiting or idle - keep polling
      schedulePoll(3000)
    } catch (e: any) {
      loginMessage.value = e.message || 'Kunde inte kontrollera inloggningen'
      schedulePoll(5000)
    }
  }

  return {
    accounts,
    currentAccount,
    current,
    folders,
    loadingAccounts,
    accountError,
    loginState,
    loginChallenge,
    loginMessage,
    needsLogin,
    loadAccounts,
    selectAccount,
    loadFolders,
    selectFolder,
    defaultFolder,
    folder,
    startLogin,
    resetLogin,
  }
}
