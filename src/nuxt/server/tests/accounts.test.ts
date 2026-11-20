import { describe, expect, test } from 'bun:test'
import { needsLogin, pickAccount, pickFolder } from '../../app/utils/accounts'

const passwordAccount = {
  name: 'Michael Brundins',
  username: 'michael@brundins.se',
  server: 'imap.one.com',
  folder: 'INBOX.Simon',
  auth_method: 'password' as const,
  logged_in: true,
  login_needed: false,
}

const oauthAccount = {
  name: 'Soulmate',
  username: 'michael.brundin@soulmate.se',
  server: 'outlook.office365.com',
  folder: 'INBOX',
  auth_method: 'oauth2' as const,
  logged_in: false,
  login_needed: true,
}

describe('pickAccount', () => {
  test('väljer det sparade kontot när det finns kvar', () => {
    const accounts = [passwordAccount, oauthAccount]
    expect(pickAccount(accounts, 'Soulmate')).toBe('Soulmate')
  })

  test('faller tillbaka på första kontot när det sparade inte finns', () => {
    const accounts = [passwordAccount, oauthAccount]
    expect(pickAccount(accounts, 'Borta')).toBe('Michael Brundins')
  })

  test('väljer första kontot när inget är sparat', () => {
    expect(pickAccount([passwordAccount, oauthAccount], null)).toBe('Michael Brundins')
  })

  test('ger tom sträng när det inte finns några konton', () => {
    expect(pickAccount([], null)).toBe('')
  })
})

describe('needsLogin', () => {
  test('sant för ett konto som inte är inloggat', () => {
    expect(needsLogin(oauthAccount)).toBe(true)
  })

  test('falskt för ett inloggat konto', () => {
    expect(needsLogin(passwordAccount)).toBe(false)
  })

  test('falskt när det inte finns något konto', () => {
    expect(needsLogin(null)).toBe(false)
  })
})

describe('pickFolder', () => {
  test('behåller den valda mappen när den finns', () => {
    expect(pickFolder(['INBOX', 'INBOX.Simon'], 'INBOX.Simon')).toBe('INBOX.Simon')
  })

  test('tar första mappen när den valda inte finns', () => {
    expect(pickFolder(['INBOX', 'INBOX.Simon'], 'Archive')).toBe('INBOX')
  })

  test('behåller den valda mappen när listan är tom', () => {
    expect(pickFolder([], 'INBOX')).toBe('INBOX')
  })
})
