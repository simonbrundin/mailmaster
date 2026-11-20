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
  classification: 'kortsiktig' | 'langsiktig' | 'okand'
  classificationBasis: string
}

export interface KameoLookupResult {
  query: string
  account: {
    name: string | null
    email: string | null
    accountNo: string | null
  }
  source: '1password' | 'env' | 'fil' | 'krypterad'
  results: KameoLoan[]
}

export function useKameoLoans() {
  const query = ref('')
  const loading = ref(false)
  const error = ref<string | null>(null)
  const result = ref<KameoLookupResult | null>(null)

  async function search(loanNumber: string = query.value) {
    loading.value = true
    error.value = null

    try {
      result.value = await $fetch<KameoLookupResult>('/api/kameo/loans', {
        method: 'POST',
        body: { loanNumber },
      })
    } catch (e: any) {
      error.value =
        e?.data?.statusMessage || e?.message || 'Kunde inte hämta uppgifter från Kameo.'
      result.value = null
    } finally {
      loading.value = false
    }
  }

  return { query, loading, error, result, search }
}
