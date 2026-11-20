import { describe, expect, test } from 'bun:test'
import { hasSopsAgeKey, sopsDecrypt, sopsEncrypt } from '../utils/sops'

describe('sops', () => {
  test('krypterar och dekrypterar en dotenv', async () => {
    if (!hasSopsAgeKey()) {
      console.warn('Hoppar över sops-testet: ingen age-nyckel hittades')
      return
    }

    const plain = 'KAMEO_EMAIL=user@example.com\nKAMEO_PASSWORD=hemligt\n'
    const encrypted = await sopsEncrypt(plain)

    expect(encrypted).not.toContain('hemligt')
    expect(encrypted).toContain('ENC[')

    const decrypted = await sopsDecrypt(encrypted)
    expect(decrypted).toContain('KAMEO_EMAIL=user@example.com')
    expect(decrypted).toContain('KAMEO_PASSWORD=hemligt')
  })
})
