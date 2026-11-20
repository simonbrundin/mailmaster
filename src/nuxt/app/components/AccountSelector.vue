<script setup lang="ts">
const {
  accounts,
  currentAccount,
  current,
  folders,
  loadingAccounts,
  accountError,
  loginState,
  loginChallenge,
  loginMessage,
  needsLogin,
  loadAccounts,
  selectAccount,
  startLogin,
} = useAccounts()

const emit = defineEmits<{
  (e: 'select', name: string): void
}>()

onMounted(() => {
  loadAccounts()
})

async function onSelect(event: Event) {
  const name = (event.target as HTMLSelectElement).value
  await selectAccount(name)
  emit('select', name)
}
</script>

<template>
  <div class="account-bar">
    <div class="account-row">
      <label class="account-label" for="account-select">Konto</label>
      <select
        id="account-select"
        class="account-select"
        :value="currentAccount"
        :disabled="loadingAccounts || accounts.length === 0"
        @change="onSelect"
      >
        <option v-for="account in accounts" :key="account.name" :value="account.name">
          {{ account.name }}
        </option>
      </select>

      <span v-if="current" class="account-meta">
        {{ current.username }}
        <span class="dot">·</span>
        {{ current.server }}
      </span>

      <button
        v-if="needsLogin"
        class="btn btn-login"
        :disabled="loginState === 'waiting'"
        @click="startLogin"
      >
        {{ loginState === 'waiting' ? '⏳ Väntar på inloggning...' : '🔑 Logga in' }}
      </button>
      <span v-else-if="current?.logged_in" class="badge-ok">✓ Inloggad</span>
    </div>

    <div v-if="loginChallenge && loginState === 'waiting'" class="challenge">
      <p class="challenge-text">
        Öppna <a :href="loginChallenge.verification_uri" target="_blank" rel="noopener">
          {{ loginChallenge.verification_uri }}
        </a> och mata in koden:
      </p>
      <code class="challenge-code">{{ loginChallenge.user_code }}</code>
    </div>

    <p v-if="loginMessage" class="login-message">{{ loginMessage }}</p>
    <p v-if="accountError" class="login-message error">{{ accountError }}</p>
  </div>
</template>

<style scoped>
.account-bar {
  margin-bottom: 1.5rem;
}

.account-row {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  flex-wrap: wrap;
  justify-content: center;
}

.account-label {
  color: #888;
  font-size: 0.9rem;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.account-select {
  padding: 0.75rem 1rem;
  border-radius: 8px;
  border: 1px solid #333;
  background: #1a1a1a;
  color: #fff;
  font-size: 1rem;
  cursor: pointer;
}

.account-select:focus {
  outline: none;
  border-color: #667eea;
}

.account-meta {
  color: #666;
  font-size: 0.85rem;
}

.dot {
  margin: 0 0.25rem;
}

.badge-ok {
  color: #4ade80;
  font-size: 0.85rem;
}

.btn-login {
  padding: 0.6rem 1.1rem;
  border-radius: 8px;
  border: none;
  background: linear-gradient(135deg, #f97316, #ea580c);
  color: #fff;
  font-size: 0.95rem;
  font-weight: 600;
  cursor: pointer;
}

.btn-login:hover:not(:disabled) {
  transform: translateY(-2px);
}

.btn-login:disabled {
  opacity: 0.7;
  cursor: not-allowed;
}

.challenge {
  margin-top: 1rem;
  text-align: center;
  background: rgba(102, 126, 234, 0.1);
  border: 1px solid #667eea;
  border-radius: 8px;
  padding: 1rem;
}

.challenge-text {
  color: #ccc;
  font-size: 0.9rem;
  margin-bottom: 0.5rem;
}

.challenge-text a {
  color: #b9a7ff;
}

.challenge-code {
  display: inline-block;
  font-size: 1.4rem;
  letter-spacing: 0.15em;
  background: #0f0f0f;
  border: 1px solid #333;
  border-radius: 6px;
  padding: 0.5rem 1rem;
  color: #fff;
}

.login-message {
  margin-top: 0.75rem;
  text-align: center;
  color: #f97316;
  font-size: 0.9rem;
}

.login-message.error {
  color: #ef4444;
}
</style>
