import { createHmac } from 'node:crypto'

const BASE32_ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'

/**
 * Dekodar en base32-sträng (t.ex. 1Passwords TOTP-hemlighet) till bytes.
 */
export function base32Decode(secret: string): Buffer {
  const normalized = secret.toUpperCase().replace(/=+$/, '').replace(/\s+/g, '')
  let bits = ''

  for (const char of normalized) {
    const index = BASE32_ALPHABET.indexOf(char)
    if (index === -1) {
      continue
    }
    bits += index.toString(2).padStart(5, '0')
  }

  const bytes: number[] = []
  for (let i = 0; i + 8 <= bits.length; i += 8) {
    bytes.push(parseInt(bits.slice(i, i + 8), 2))
  }

  return Buffer.from(bytes)
}

/**
 * Genererar en TOTP-kod (RFC 6238, SHA-1, 30 sekunder, 6 siffror).
 * Används som reserv när 1Password inte är tillgängligt i processen.
 */
export function generateTotp(
  secret: string,
  timestamp: number = Date.now(),
  period: number = 30,
  digits: number = 6,
): string {
  const counter = Math.floor(timestamp / 1000 / period)
  const buffer = Buffer.alloc(8)
  buffer.writeBigUInt64BE(BigInt(counter))

  const digest = createHmac('sha1', base32Decode(secret)).update(buffer).digest()
  const offset = digest[digest.length - 1] & 0x0f
  const binary =
    ((digest[offset] & 0x7f) << 24) |
    (digest[offset + 1] << 16) |
    (digest[offset + 2] << 8) |
    digest[offset + 3]

  return String(binary % 10 ** digits).padStart(digits, '0')
}
