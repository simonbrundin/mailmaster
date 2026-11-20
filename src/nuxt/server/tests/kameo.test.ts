import { describe, expect, test } from 'bun:test'
import { classifyLoan, filterLoans, monthsUntil, parseEnvFile } from '../utils/kameo'
import { generateTotp } from '../utils/totp'

describe('monthsUntil', () => {
  const from = new Date('2026-10-29T00:00:00Z')

  test('räknar hela månader mellan datum', () => {
    expect(monthsUntil(from, new Date('2026-11-29T00:00:00Z'))).toBe(1)
    expect(monthsUntil(from, new Date('2027-10-29T00:00:00Z'))).toBe(12)
    expect(monthsUntil(from, new Date('2028-06-03T00:00:00Z'))).toBe(19)
  })

  test('en månad är inte avslutad före dagen i månaden', () => {
    expect(monthsUntil(from, new Date('2027-10-15T00:00:00Z'))).toBe(11)
    expect(monthsUntil(from, new Date('2027-12-15T00:00:00Z'))).toBe(13)
  })
})

describe('classifyLoan', () => {
  const now = new Date('2026-10-29T00:00:00Z')

  test('mindre än 12 månader kvar är kortsiktig', () => {
    const result = classifyLoan(
      { status: 'ongoing', termMonths: 24, maturityDate: '2027-09-01T00:00:00.000Z' },
      now,
    )
    expect(result.classification).toBe('kortsiktig')
    expect(result.monthsRemaining).toBe(10)
    expect(result.basis).toBe('återstående löptid')
  })

  test('12 månader eller mer kvar är långsiktig', () => {
    const result = classifyLoan(
      { status: 'ongoing', termMonths: 24, maturityDate: '2027-12-15T00:00:00.000Z' },
      now,
    )
    expect(result.classification).toBe('langsiktig')
    expect(result.monthsRemaining).toBe(13)
  })

  test('återbetalt lån utan slutdatum klassificeras efter lånets löptid', () => {
    const result = classifyLoan(
      { status: 'repaid', termMonths: 9, maturityDate: null },
      now,
    )
    expect(result.classification).toBe('kortsiktig')
    expect(result.monthsRemaining).toBeNull()
    expect(result.basis).toBe('lånets löptid')
  })

  test('saknad information ger okänd klassificering', () => {
    const result = classifyLoan({ status: 'ongoing', termMonths: null, maturityDate: null }, now)
    expect(result.classification).toBe('okand')
  })
})

describe('filterLoans', () => {
  const loans = [
    { loanNumber: 5133 } as any,
    { loanNumber: 5046 } as any,
    { loanNumber: 5012 } as any,
  ]

  test('exakt träff listas först', () => {
    const result = filterLoans(loans, '5046')
    expect(result.map((loan) => loan.loanNumber)).toEqual([5046])
  })

  test('deltäffar listas efter exakta träffar', () => {
    const result = filterLoans(loans, '50')
    expect(result.map((loan) => loan.loanNumber)).toEqual([5046, 5012])
  })

  test('tom sökning ger hela portföljen', () => {
    expect(filterLoans(loans, '')).toHaveLength(3)
  })
})

describe('generateTotp', () => {
  // RFC 6238 testvektor: hemligheten är "12345678901234567890" i base32.
  const secret = 'GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ'

  test('matchar RFC 6238', () => {
    expect(generateTotp(secret, 59_000, 30, 8)).toBe('94287082')
  })

  test('sex siffror som används av Kameo', () => {
    expect(generateTotp(secret, 59_000)).toBe('287082')
  })
})

describe('parseEnvFile', () => {
  test('läser nyckel=värde och hoppar över kommentarer', () => {
    const values = parseEnvFile(
      [
        '# kommentar',
        'KAMEO_EMAIL=user@example.com',
        'KAMEO_PASSWORD=hemligt',
        'KAMEO_TOTP_SECRET=JBSWY3DPEHPK3PXP',
        '',
      ].join('\n'),
    )

    expect(values.KAMEO_EMAIL).toBe('user@example.com')
    expect(values.KAMEO_PASSWORD).toBe('hemligt')
    expect(values.KAMEO_TOTP_SECRET).toBe('JBSWY3DPEHPK3PXP')
  })

  test('tar bort citattecken', () => {
    const values = parseEnvFile('KAMEO_PASSWORD="hemligt"\nKAMEO_EMAIL=\'a@b.se\'\n')
    expect(values.KAMEO_PASSWORD).toBe('hemligt')
    expect(values.KAMEO_EMAIL).toBe('a@b.se')
  })
})
