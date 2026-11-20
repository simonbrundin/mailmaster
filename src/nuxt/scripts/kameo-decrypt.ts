/**
 * Dekrypterar environments/dev/kameo.env.enc till environments/dev/kameo.env
 * så att docker compose kan läsa den via env_file.
 *
 *   cd src/nuxt && bun run kameo:decrypt
 */
import { chmodSync, existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { sopsDecrypt } from '../server/utils/sops'

const scriptDir = dirname(fileURLToPath(import.meta.url))
const nuxtRoot = resolve(scriptDir, '..')
const repoRoot = resolve(nuxtRoot, '../..')

const encryptedFile = process.env.KAMEO_ENCRYPTED_FILE
  ? resolve(process.env.KAMEO_ENCRYPTED_FILE)
  : resolve(repoRoot, 'environments/dev/kameo.env.enc')

const plainFile = process.env.KAMEO_CREDENTIALS_FILE
  ? resolve(process.env.KAMEO_CREDENTIALS_FILE)
  : resolve(repoRoot, 'environments/dev/kameo.env')

if (!existsSync(encryptedFile)) {
  console.error(
    `\nHittade inte ${encryptedFile}.\n` +
      'Kör "bun run kameo:sync" först för att hämta uppgifterna från 1Password.\n',
  )
  process.exit(1)
}

const plain = await sopsDecrypt(readFileSync(encryptedFile, 'utf8'))

mkdirSync(dirname(plainFile), { recursive: true })
writeFileSync(plainFile, plain, { mode: 0o600 })
chmodSync(plainFile, 0o600)

console.log(`✓ Dekrypterade till ${plainFile} (läges 600)`)
console.log('  Starta om containern: docker compose up -d --force-recreate nuxt')
