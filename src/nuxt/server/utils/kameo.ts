import { execFile } from 'node:child_process'
import { existsSync, readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { promisify } from 'node:util'
import { sopsDecrypt } from './sops'
import { generateTotp } from './totp'

const execFileAsync = promisify(execFile)

const KAMEO_API = 'https://api.kameo.se/v1'
const OP_ITEM = process.env.KAMEO_OP_ITEM || 'Kameo - Michael Brundin AB'
const OP_VAULT = process.env.KAMEO_OP_VAULT || 'Private'
const LOGIN_TIMEOUT_MS = 60_000

export type LoanClassification = 'kortsiktig' | 'langsiktig' | 'okand'

export interface KameoLoan {
  loanNumber: number
  loanName: string
  status: string
  statusLabel: string
  termMonths: number | null
  interestRate: string | null
  investedAmount: number
  remainingAmount: number
  maturityDate: string | null
  nextInterestPaymentDate: string | null
  nextInterestPaymentAmount: number | null
  monthsRemaining: number | null
  classification: LoanClassification
  classificationBasis: string
}

export type CredentialSource = '1password' | 'env' | 'fil' | 'krypterad'

export interface KameoLookupResult {
  query: string
  account: {
    name: string | null
    email: string | null
    accountNo: string | null
  }
  source: CredentialSource
  results: KameoLoan[]
}

/**
 * Antal hela månader mellan två datum. Om slutförsdatumet infaller före
 * "dagen i månaden" räknas den sista månaden som inte avslutad.
 */
export function monthsUntil(from: Date, to: Date): number {
  let months = (to.getUTCFullYear() - from.getUTCFullYear()) * 12
  months += to.getUTCMonth() - from.getUTCMonth()

  if (to.getUTCDate() < from.getUTCDate()) {
    months -= 1
  }

  return months
}

export interface ClassifiableLoan {
  status: string
  termMonths: number | null
  maturityDate: string | null
}

/**
 * Klassificerar en tillgång som kortsiktig eller långsiktig.
 *
 * Grundregel: en tillgång är kortsiktig om den har mindre än 12 månader
 * kvar att löpa, annars långsiktig. För redan återbetald lån finns ingen
 * återstående löptid, och då används lånets ursprungliga löptid.
 */
export function classifyLoan(
  loan: ClassifiableLoan,
  now: Date = new Date(),
): { classification: LoanClassification; monthsRemaining: number | null; basis: string } {
  if (loan.maturityDate) {
    const maturity = new Date(loan.maturityDate)
    const months = monthsUntil(now, maturity)
    return {
      classification: months < 12 ? 'kortsiktig' : 'langsiktig',
      monthsRemaining: months,
      basis: 'återstående löptid',
    }
  }

  if (loan.termMonths !== null) {
    return {
      classification: loan.termMonths < 12 ? 'kortsiktig' : 'langsiktig',
      monthsRemaining: null,
      basis: 'lånets löptid',
    }
  }

  return { classification: 'okand', monthsRemaining: null, basis: 'okänd' }
}

/**
 * Söker fram lån utifrån ett lånnummer. Exakta träffar listas först,
 * därefter delträffar.
 */
export function filterLoans(loans: KameoLoan[], query: string): KameoLoan[] {
  const trimmed = query.trim()
  if (!trimmed) {
    return loans
  }

  const exact = loans.filter((loan) => String(loan.loanNumber) === trimmed)
  const partial = loans.filter(
    (loan) => String(loan.loanNumber) !== trimmed && String(loan.loanNumber).includes(trimmed),
  )

  return [...exact, ...partial]
}

interface OpItem {
  username: string
  password: string
  otpSecret: string | null
}

async function runOp(args: string[]): Promise<string> {
  try {
    const { stdout } = await execFileAsync('op', args, { timeout: 30_000 })
    return stdout.trim()
  } catch (error: any) {
    throw new Error(
      `Kunde inte läsa uppgifterna från 1Password (${error?.message ?? 'okänt fel'}). ` +
        'Se till att 1Password är upplåst (t.ex. "op signin") och att "op" finns i PATH.',
    )
  }
}

async function readOpItem(): Promise<OpItem> {
  const raw = await runOp(['item', 'get', OP_ITEM, '--vault', OP_VAULT, '--format=json', '--reveal'])
  const item = JSON.parse(raw)
  const fields: any[] = item.fields || []

  const username = fields.find((field) => field.purpose === 'USERNAME')?.value
  const password = fields.find((field) => field.purpose === 'PASSWORD')?.value
  const otpSecret = fields.find((field) => field.type === 'OTP')?.value ?? null

  if (!username || !password) {
    throw new Error(`1Password-objektet "${OP_ITEM}" saknar e-postadress eller lösenord.`)
  }

  return { username, password, otpSecret }
}

async function readOpOtp(): Promise<string> {
  return runOp(['item', 'get', OP_ITEM, '--vault', OP_VAULT, '--otp'])
}

interface Credentials {
  email: string
  password: string
  otp: string
  source: CredentialSource
  secret: string | null
}

const CREDENTIALS_HELP =
  'Kör "bun run kameo:sync" på värden för att hämta uppgifterna från 1Password, ' +
  'eller sätt KAMEO_EMAIL, KAMEO_PASSWORD och KAMEO_TOTP_SECRET.'

/**
 * Platsen där uppgifterna från 1Password sparas av `bun run kameo:sync`.
 * Sökordningen gör att både värd- och container-körning hittar filen.
 */
export function credentialFileCandidates(): string[] {
  const candidates: string[] = []

  if (process.env.KAMEO_CREDENTIALS_FILE) {
    candidates.push(process.env.KAMEO_CREDENTIALS_FILE)
  }

  const cwd = process.cwd()
  candidates.push(
    resolve(cwd, '../../environments/dev/kameo.env'),
    resolve(cwd, '../environments/dev/kameo.env'),
    resolve(cwd, 'kameo.env'),
    resolve(cwd, '../kameo.env'),
  )

  return candidates
}

/**
 * Samma sökvägar, men för den SOPS-krypterade filen.
 */
export function encryptedFileCandidates(): string[] {
  const candidates: string[] = []

  if (process.env.KAMEO_ENCRYPTED_FILE) {
    candidates.push(process.env.KAMEO_ENCRYPTED_FILE)
  }

  const cwd = process.cwd()
  candidates.push(
    resolve(cwd, '../../environments/dev/kameo.env.enc'),
    resolve(cwd, '../environments/dev/kameo.env.enc'),
    resolve(cwd, 'kameo.env.enc'),
    resolve(cwd, '../kameo.env.enc'),
  )

  return candidates
}

export function parseEnvFile(content: string): Record<string, string> {
  const values: Record<string, string> = {}

  for (const line of content.split('\n')) {
    const trimmed = line.trim()
    if (!trimmed || trimmed.startsWith('#')) {
      continue
    }

    const separator = trimmed.indexOf('=')
    if (separator === -1) {
      continue
    }

    const key = trimmed.slice(0, separator).trim()
    let value = trimmed.slice(separator + 1).trim()
    if (
      (value.startsWith('"') && value.endsWith('"')) ||
      (value.startsWith("'") && value.endsWith("'"))
    ) {
      value = value.slice(1, -1)
    }

    values[key] = value
  }

  return values
}

function readFileCredentials(now: number): Credentials | null {
  const file = credentialFileCandidates().find((candidate) => existsSync(candidate))
  if (!file) {
    return null
  }

  return credentialsFromValues(parseEnvFile(readFileSync(file, 'utf8')), now, 'fil')
}

async function readEncryptedFileCredentials(now: number): Promise<Credentials | null> {
  const file = encryptedFileCandidates().find((candidate) => existsSync(candidate))
  if (!file) {
    return null
  }

  try {
    const plain = await sopsDecrypt(readFileSync(file, 'utf8'))
    return credentialsFromValues(parseEnvFile(plain), now, 'krypterad')
  } catch (error: any) {
    throw new Error(
      `Kunde inte dekryptera ${file} (${error?.message ?? 'okänt fel'}). ` +
        'Kontrollera att age-nyckeln finns i ~/.config/sops-age/keys.txt.',
    )
  }
}

function credentialsFromValues(
  values: Record<string, string>,
  now: number,
  source: CredentialSource,
): Credentials | null {
  const email = values.KAMEO_EMAIL
  const password = values.KAMEO_PASSWORD
  const secret = values.KAMEO_TOTP_SECRET

  if (!email || !password || !secret) {
    return null
  }

  return { email, password, otp: generateTotp(secret, now), source, secret }
}

function readEnvCredentials(now: number): Credentials | null {
  const email = process.env.KAMEO_EMAIL
  const password = process.env.KAMEO_PASSWORD
  const secret = process.env.KAMEO_TOTP_SECRET

  if (!email || !password || !secret) {
    return null
  }

  return { email, password, otp: generateTotp(secret, now), source: 'env', secret }
}

async function readOpCredentials(now: number): Promise<Credentials> {
  const item = await readOpItem()
  const otp = item.otpSecret
    ? generateTotp(item.otpSecret, now)
    : await readOpOtp()

  return {
    email: item.username,
    password: item.password,
    otp,
    source: '1password',
    secret: item.otpSecret,
  }
}

/**
 * Slår upp inloggningsuppgifterna i prioriterad ordning:
 * miljövariabler -> synkad fil -> 1Password via op-cli.
 */
async function resolveCredentials(now: number): Promise<Credentials> {
  const fromEnv = readEnvCredentials(now)
  if (fromEnv) {
    return fromEnv
  }

  const fromFile = readFileCredentials(now)
  if (fromFile) {
    return fromFile
  }

  const fromEncryptedFile = await readEncryptedFileCredentials(now)
  if (fromEncryptedFile) {
    return fromEncryptedFile
  }

  try {
    return await readOpCredentials(now)
  } catch (error: any) {
    throw new Error(`${error?.message || 'Kunde inte hämta uppgifterna från 1Password.'} ${CREDENTIALS_HELP}`)
  }
}

let cachedToken: { token: string; expiresAt: number } | null = null
let lastUsedOtp: { code: string; period: number } | null = null
let lastSource: CredentialSource = '1password'

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

function decodeTokenExpiry(token: string): number {
  try {
    const payload = token.split('.')[1]
    const json = JSON.parse(Buffer.from(payload, 'base64url').toString('utf8'))
    return typeof json.exp === 'number' ? json.exp * 1000 : Date.now() + 10 * 60_000
  } catch {
    return Date.now() + 10 * 60_000
  }
}

/**
 * En ny TOTP-kod för nästa 30-sekundersfönster.
 */
async function readFreshOtp(credentials: Credentials): Promise<string> {
  if (credentials.source === '1password') {
    const fresh = await readOpCredentials(Date.now())
    return fresh.otp
  }

  const secret = credentials.secret
  return secret ? generateTotp(secret, Date.now()) : credentials.otp
}

/**
 * Loggar in på Kameo med uppgifterna från 1Password (alternativt miljövariabler
 * eller den synkade filen) och returnerar en access token. Token cachas tills
 * den går ut.
 */
export async function login(now: number = Date.now()): Promise<string> {
  if (cachedToken && cachedToken.expiresAt > now + 60_000) {
    return cachedToken.token
  }

  const credentials = await resolveCredentials(now)
  lastSource = credentials.source

  // Samma TOTP-kod får bara användas en gång per tidsfönster.
  const period = Math.floor(now / 30_000)
  if (lastUsedOtp && lastUsedOtp.code === credentials.otp && lastUsedOtp.period === period) {
    const waitMs = (period + 1) * 30_000 - now + 1_000
    await sleep(Math.min(Math.max(waitMs, 0), 35_000))
    credentials.otp = await readFreshOtp(credentials)
  }

  const response = await fetch(`${KAMEO_API}/authenticate/2fa`, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({
      email: credentials.email,
      password: credentials.password,
      otp: credentials.otp,
    }),
    signal: AbortSignal.timeout(LOGIN_TIMEOUT_MS),
  })

  if (!response.ok) {
    const body = await response.text()
    throw new Error(`Kameo-inloggningen misslyckades (HTTP ${response.status}): ${body}`)
  }

  const payload = await response.json()
  const token: string = payload?.data?.access_token

  if (!token) {
    throw new Error('Kameo-inloggningen misslyckades: inget access token returnerades.')
  }

  cachedToken = { token, expiresAt: decodeTokenExpiry(token) }
  lastUsedOtp = { code: credentials.otp, period }

  return token
}

async function apiGet<T>(token: string, path: string): Promise<T> {
  const response = await fetch(`${KAMEO_API}${path}`, {
    headers: { authorization: `Bearer ${token}` },
    signal: AbortSignal.timeout(LOGIN_TIMEOUT_MS),
  })

  if (!response.ok) {
    const body = await response.text()
    throw new Error(`Kameo-anropet ${path} misslyckades (HTTP ${response.status}): ${body}`)
  }

  return (await response.json()) as T
}

function toNumber(value: unknown): number {
  const parsed = Number.parseFloat(String(value ?? '').replace(',', '.'))
  return Number.isFinite(parsed) ? parsed : 0
}

const STATUS_LABELS: Record<string, string> = {
  ongoing: 'Pågående',
  repaid: 'Återbetalt',
}

/**
 * Loggar in på Kameo och hämtar lånen. Om ett lånnummer anges returneras bara
 * lån som matchar numret (exakta träffar först).
 */
export async function lookupKameoLoans(query: string): Promise<KameoLookupResult> {
  const now = new Date()
  const token = await login(now.getTime())

  const me = await apiGet<any>(token, '/me')
  const accounts = await apiGet<any[]>(token, '/me/customer-accounts')
  const primaryAccount = accounts.find((account) => account.is_primary) || accounts[0]
  const accountNo: string | null = primaryAccount?.account_no
    ? String(primaryAccount.account_no)
    : null

  const loans: any[] = []
  const perPage = 50

  for (let page = 1; page <= 10; page++) {
    const batch = await apiGet<{ data: any[] }>(
      token,
      `/account/transactions-by-loan?home_currency=SEK&per_page=${perPage}&page=${page}`,
    )
    const rows = batch?.data || []
    loans.push(...rows)
    if (rows.length < perPage) {
      break
    }
  }

  const positions = new Map<number, any>()
  const upcoming = new Map<number, any>()

  if (accountNo) {
    const risk = await apiGet<any>(
      token,
      `/account/risk-distribution?accounts=${encodeURIComponent(accountNo)}&currency=SEK`,
    )
    for (const position of risk?.data?.positions || []) {
      const match = /LoanApplication\.(\d+)/.exec(position.reference || '')
      if (match) {
        positions.set(Number(match[1]), position)
      }
    }

    const repayments = await apiGet<any>(
      token,
      `/account/upcoming-repayments?accounts=${encodeURIComponent(accountNo)}`,
    )
    for (const repayment of repayments?.data?.upcoming_repayments || []) {
      const match = /(\d+)\s*$/.exec(repayment.name || '')
      if (match) {
        upcoming.set(Number(match[1]), repayment)
      }
    }
  }

  const mapped: KameoLoan[] = loans.map((loan) => {
    const loanNumber = Number(loan.loan_number)
    const position = positions.get(loanNumber)
    const payment = upcoming.get(loanNumber)
    const maturityDate: string | null = position?.next_amortization_payment?.due_date ?? null
    const termMonths: number | null =
      typeof loan.loan_duration_months === 'number' ? loan.loan_duration_months : null

    const { classification, monthsRemaining, basis } = classifyLoan(
      { status: loan.status, termMonths, maturityDate },
      now,
    )

    return {
      loanNumber,
      loanName: loan.loan_name,
      status: loan.status,
      statusLabel: STATUS_LABELS[loan.status] || loan.status,
      termMonths,
      interestRate: loan.interest_rate_percentage ?? null,
      investedAmount: toNumber(loan.invested_bought),
      remainingAmount: toNumber(loan.remaining_amount),
      maturityDate,
      nextInterestPaymentDate: position?.next_interest_payment?.due_date ?? null,
      nextInterestPaymentAmount: position?.next_interest_payment?.amount ?? null,
      monthsRemaining,
      classification,
      classificationBasis: basis,
    }
  })

  return {
    query: query.trim(),
    account: {
      name: me?.data?.organization_name
        ? me.data.organization_name
        : [me?.data?.first_name, me?.data?.last_name].filter(Boolean).join(' ') || null,
      email: me?.data?.email ?? null,
      accountNo,
    },
    source: lastSource,
    results: filterLoans(mapped, query),
  }
}
