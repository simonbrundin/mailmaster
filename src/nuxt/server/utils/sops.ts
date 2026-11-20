import { execFile } from 'node:child_process'
import { existsSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { homedir, tmpdir } from 'node:os'
import { resolve } from 'node:path'
import { promisify } from 'node:util'

const execFileAsync = promisify(execFile)

/**
 * Publika age-mottagarnyckeln - samma nyckel som används i infra-repoet,
 * så en nyckel räcker för båda.
 */
export const SOPS_AGE_RECIPIENT =
  process.env.SOPS_AGE_RECIPIENT || 'age1mv3yrgdtqddlr5v9tg67nydvztm3sm6r3uu3j9zhdp736p0wt5gqwyj9lt'

/**
 * Letar upp den privata age-nyckeln. SOPS söker default i
 * ~/.config/sops/age/keys.txt, den här miljön har den i ~/.config/sops-age.
 */
export function sopsEnv(): NodeJS.ProcessEnv {
  const env: NodeJS.ProcessEnv = { ...process.env }

  if (!env.SOPS_AGE_KEY_FILE) {
    const candidates = [
      resolve(homedir(), '.config/sops-age/keys.txt'),
      resolve(homedir(), '.config/sops/age/keys.txt'),
    ]
    const found = candidates.find((candidate) => existsSync(candidate))
    if (found) {
      env.SOPS_AGE_KEY_FILE = found
    }
  }

  return env
}

export function hasSopsAgeKey(): boolean {
  return Boolean(sopsEnv().SOPS_AGE_KEY_FILE)
}

/**
 * SOPS 3.13 läser alltid från en fil, så vi skriver temporärt och städar bort.
 */
async function runSops(args: string[], content: string): Promise<string> {
  const dir = mkdtempSync(resolve(tmpdir(), 'sops-'))
  const file = resolve(dir, 'payload.env')

  try {
    writeFileSync(file, content, { mode: 0o600 })
    const { stdout } = await execFileAsync('sops', [...args, file], {
      env: sopsEnv(),
      maxBuffer: 10 * 1024 * 1024,
    })
    return stdout
  } finally {
    rmSync(dir, { recursive: true, force: true })
  }
}

export async function sopsEncrypt(content: string): Promise<string> {
  return runSops(
    ['--encrypt', '--age', SOPS_AGE_RECIPIENT, '--input-type', 'dotenv', '--output-type', 'dotenv'],
    content,
  )
}

export async function sopsDecrypt(content: string): Promise<string> {
  return runSops(['--decrypt', '--input-type', 'dotenv', '--output-type', 'dotenv'], content)
}
