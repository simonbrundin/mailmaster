<script setup lang="ts">
import { useKameoLoans } from '~/composables/useKameoLoans'

useHead({ title: 'Kameo utbetalningar' })

const router = useRouter()
const { query, loading, error, result, search } = useKameoLoans()

const hasSearched = ref(false)

async function onSearch() {
  hasSearched.value = true
  await search()
}

function formatDate(value: string | null): string {
  if (!value) {
    return '–'
  }
  return new Date(value).toLocaleDateString('sv-SE', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })
}

function formatAmount(value: number): string {
  return value.toLocaleString('sv-SE', { minimumFractionDigits: 2, maximumFractionDigits: 2 }) + ' kr'
}

function classificationLabel(value: string): string {
  if (value === 'kortsiktig') return 'Kortsiktig tillgång'
  if (value === 'langsiktig') return 'Långsiktig tillgång'
  return 'Okänd'
}

function sourceLabel(value: string): string {
  if (value === '1password') return '1Password'
  if (value === 'krypterad') return 'SOPS-krypterad fil'
  if (value === 'fil') return 'fil'
  return 'miljövariabler'
}

function handleBack() {
  router.push('/')
}

onMounted(() => {
  search('')
})
</script>

<template>
  <div class="container">
    <header>
      <button class="back-btn" @click="handleBack">← Tillbaka</button>
      <h1><span class="emoji">💸</span> Kameo utbetalningar</h1>
      <p class="subtitle">
        Sök ett lånnummer hos Kameo och se om tillgången är kortsiktig eller långsiktig.
      </p>
    </header>

    <section class="search-box">
      <input
        v-model="query"
        type="text"
        inputmode="numeric"
        placeholder="Lånnummer, t.ex. 5133"
        @keyup.enter="onSearch"
      />
      <button class="primary-btn" :disabled="loading" @click="onSearch">
        {{ loading ? 'Söker…' : 'Sök' }}
      </button>
    </section>

    <p class="rule">
      Regel: <strong>kortsiktig</strong> om lånet har mindre än 12 månader kvar att löpa,
      annars <strong>långsiktig</strong>.
    </p>

    <div v-if="error" class="error">❌ {{ error }}</div>

    <div v-if="result" class="account-info">
      <span>🏦 {{ result.account.name || 'Okänt konto' }}</span>
      <span v-if="result.account.email"> ({{ result.account.email }})</span>
      <span v-if="result.account.accountNo" class="muted">· konto {{ result.account.accountNo }}</span>
      <span class="muted">· inloggning via {{ sourceLabel(result.source) }}</span>
    </div>

    <div v-if="result && result.results.length === 0 && !loading" class="empty">
      <p>Inga lån matchade {{ result.query }}.</p>
      <p class="hint">Kontrollera att lånnumret stämmer med det som visas i Kameo.</p>
    </div>

    <ul v-if="result && result.results.length > 0" class="loan-list">
      <li v-for="loan in result.results" :key="loan.loanNumber" class="loan-card">
        <div class="loan-head">
          <div>
            <span class="loan-number">{{ loan.loanNumber }}</span>
            <span class="loan-name">{{ loan.loanName }}</span>
          </div>
          <span
            class="badge"
            :class="loan.status === 'repaid' ? 'unknown' : loan.classification === 'kortsiktig' ? 'short' : loan.classification === 'langsiktig' ? 'long' : 'unknown'"
          >
            {{ loan.status === 'repaid' ? 'Återbetalt' : classificationLabel(loan.classification) }}
          </span>
        </div>

        <dl class="facts">
          <div>
            <dt>Status</dt>
            <dd>{{ loan.statusLabel }}</dd>
          </div>
          <div>
            <dt>Löptid</dt>
            <dd>{{ loan.termMonths !== null ? loan.termMonths + ' mån' : '–' }}</dd>
          </div>
          <div>
            <dt>Återstående löptid</dt>
            <dd>
              {{ loan.monthsRemaining !== null ? loan.monthsRemaining + ' mån' : '–' }}
              <span v-if="loan.monthsRemaining !== null" class="muted">({{ loan.classificationBasis }})</span>
            </dd>
          </div>
          <div>
            <dt>Slutförsdatum</dt>
            <dd>{{ formatDate(loan.maturityDate) }}</dd>
          </div>
          <div>
            <dt>Ränta</dt>
            <dd>{{ loan.interestRate ? loan.interestRate + ' %' : '–' }}</dd>
          </div>
          <div>
            <dt>Återstående belopp</dt>
            <dd>{{ formatAmount(loan.remainingAmount) }}</dd>
          </div>
          <div>
            <dt>Nästa ränta</dt>
            <dd>{{ formatDate(loan.nextInterestPaymentDate) }}</dd>
          </div>
          <div>
            <dt>Investerat belopp</dt>
            <dd>{{ formatAmount(loan.investedAmount) }}</dd>
          </div>
        </dl>
      </li>
    </ul>
  </div>
</template>

<style>
.container {
  max-width: 1000px;
  margin: 0 auto;
  padding: 2rem;
}

header {
  text-align: center;
  margin-bottom: 2rem;
}

h1 {
  font-size: 2.2rem;
  margin: 0.5rem 0 0.25rem;
}

.subtitle {
  color: #888;
  margin: 0;
}

.back-btn {
  background: none;
  border: none;
  color: #888;
  cursor: pointer;
  font-size: 0.95rem;
  padding: 0.25rem 0;
}

.back-btn:hover {
  color: #fff;
}

.emoji {
  background: none;
}

.search-box {
  display: flex;
  gap: 0.75rem;
  margin-bottom: 1rem;
}

.search-box input {
  flex: 1;
  padding: 0.75rem 1rem;
  font-size: 1.1rem;
  border-radius: 8px;
  border: 1px solid #333;
  background: #1a1a1a;
  color: #fff;
}

.search-box input:focus {
  outline: none;
  border-color: #667eea;
}

.primary-btn {
  padding: 0.75rem 1.5rem;
  font-size: 1.05rem;
  border: none;
  border-radius: 8px;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  color: #fff;
  cursor: pointer;
}

.primary-btn:disabled {
  opacity: 0.5;
  cursor: default;
}

.rule {
  text-align: center;
  color: #999;
  font-size: 0.9rem;
  margin: 0 0 1.5rem;
}

.account-info {
  text-align: center;
  color: #bbb;
  margin-bottom: 1.5rem;
  font-size: 0.95rem;
}

.muted {
  color: #777;
}

.error {
  background: rgba(239, 68, 68, 0.1);
  border: 1px solid #ef4444;
  border-radius: 8px;
  padding: 1rem;
  text-align: center;
  color: #fff;
  margin-bottom: 1rem;
}

.empty {
  text-align: center;
  color: #666;
}

.loan-list {
  list-style: none;
  padding: 0;
  margin: 0;
  display: grid;
  gap: 1rem;
}

.loan-card {
  background: #171717;
  border: 1px solid #2a2a2a;
  border-radius: 10px;
  padding: 1rem 1.25rem;
}

.loan-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 1rem;
  flex-wrap: wrap;
}

.loan-number {
  font-weight: 700;
  font-size: 1.3rem;
  margin-right: 0.5rem;
}

.loan-name {
  color: #bbb;
}

.badge {
  padding: 0.3rem 0.75rem;
  border-radius: 999px;
  font-size: 0.85rem;
  font-weight: 600;
  white-space: nowrap;
}

.badge.short {
  background: rgba(245, 158, 11, 0.15);
  color: #f59e0b;
  border: 1px solid rgba(245, 158, 11, 0.4);
}

.badge.long {
  background: rgba(34, 197, 94, 0.15);
  color: #22c55e;
  border: 1px solid rgba(34, 197, 94, 0.4);
}

.badge.unknown {
  background: rgba(148, 163, 184, 0.15);
  color: #94a3b8;
  border: 1px solid rgba(148, 163, 184, 0.4);
}

.facts {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 0.75rem 1.5rem;
  margin: 1rem 0 0;
}

.facts dt {
  color: #777;
  font-size: 0.8rem;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.facts dd {
  margin: 0.15rem 0 0;
  font-size: 1rem;
}
</style>
