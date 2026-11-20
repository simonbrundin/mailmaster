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

/**
 * pickAccount returns the account that should be selected.
 *
 * The stored account wins when it still exists, otherwise the first
 * configured account is used.
 */
export function pickAccount(accounts: MailAccount[], stored: string | null): string {
  if (stored && accounts.some(a => a.name === stored)) {
    return stored
  }
  return accounts[0]?.name ?? ''
}

/**
 * needsLogin reports whether the login button should be shown for an account.
 * An account that has never logged in, or that has lost its token, needs a
 * new login.
 */
export function needsLogin(account: MailAccount | null | undefined): boolean {
  if (!account) return false
  return account.login_needed === true || account.logged_in === false
}

/**
 * pickFolder returns the folder to show for an account: the current one when
 * it exists in the list, otherwise the first one.
 */
export function pickFolder(folders: string[], current: string): string {
  if (folders.length === 0) return current
  if (folders.includes(current)) return current
  return folders[0]
}
