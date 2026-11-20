/**
 * Synkar Kameo-uppgifterna från 1Password till en SOPS-krypterad fil.
 *
 *   cd src/nuxt && bun run kameo:sync
 *
 * Resultatet skrivs till environments/dev/kameo.env.enc (krypterat med age)
 * och kan dekrypteras med:
 *
 *   cd src/nuxt && bun run kameo:decrypt
 */
import { execFileSync } from 'node:child_process'
import { chmodSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { hasSopsAgeKey, sopsEncrypt } from '../server/utils/sops'

const scriptDir = dirname(fileURLToPath(import.meta.url))
const nuxtRoot = resolve(scriptDir, '..')
const repoRoot = resolve(nuxtRoot, '../..')

const outputFile = process.env.KAMEO_ENCRYPTED_FILE
  ? resolve(process.env.KAMEO_ENCRYPTED_FILE)
  : resolve(repoRoot, 'environments/dev/kameo.env.enc')

const item = process.env.KAMEO_OP_ITEM || 'Kameo - Michael Brundin AB'
const vault = process.env.KAMEO_OP_VAULT || 'Private'

function readItem(): { username: string; password: string; otpSecret: string | null } {
  let raw: string

  try {
    raw = execFileSync('op', ['item', 'get', item, '--vault', vault, '--format=json', '--reveal'], {
      encoding: 'utf8',
      stdio: ['inherit', 'pipe', 'inherit'],
    })
  } catch {
    console.error(
      `\nKunde inte läsa "${item}" från 1Password.\n` +
        'Kontrollera att 1Password-appen är igång, att CLI-integreringen är på\n' +
        '(1Password > Settings > Developer > Integrate with 1Password CLI) och att\n' +
        `vaulten "${vault}" innehåller objektet.\n`,
    )
    process.exit(1)
  }

  const parsed = JSON.parse(raw)
  const fields: any[] = parsed.fields || []
  const username = fields.find((field) => field.purpose === 'USERNAME')?.value
  const password = fields.find((field) => field.purpose === 'PASSWORD')?.value
  const otpSecret = fields.find((field) => field.type === 'OTP')?.value ?? null

  if (!username || !password) {
    console.error(`Objektet "${item}" saknar e-postadress eller lösenord.`)
    process.exit(1)
  }

  return { username, password, otpSecret }
}

const { username, password, otpSecret } = readItem()

const plain = [
  `KAMEO_EMAIL=${username}`,
  `KAMEO_PASSWORD=${password}`,
  `KAMEO_TOTP_SECRET=${otpSecret ?? ''}`,
  '',
].join('\n')

if (!hasSopsAgeKey()) {
  console.error(
    '\nIngen age-nyckel hittades (~/.config/sops-age/keys.txt).\n' +
      'Sätt SOPS_AGE_KEY_FILE eller generera en nyckel med "age-keygen".\n',
  )
  process.exit(1)
}

const encrypted = await sopsEncrypt(plain)

mkdirSync(dirname(outputFile), { recursive: true })
writeFileSync(outputFile, encrypted, { mode: 0o600 })
chmodSync(outputFile, 0o600)

console.log(`✓ Synkade och krypterade Kameo-uppgifterna till ${outputFile}`)
console.log(`  Användare: ${username}`)
console.log(`  Kryptering: SOPS + age`)
console.log('\nNästa steg:')
console.log('  • Docker:  cd src/nuxt && bun run kameo:decrypt && docker compose up -d --force-recreate nuxt')
console.log('  • Värd:    cd src/nuxt && bun run dev   (läser .enc-filen direkt)')
