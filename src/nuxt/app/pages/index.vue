<script setup lang="ts">
import { useEmailProcessing } from '~/composables/useEmailProcessing'

const {
  emails, loading, analyzing, error,
  analyses, decisions, decidingUid,
  fetchEmails, analyzeEmails, decideEmail
} = useEmailProcessing()

const { current, folders, folder, needsLogin, selectFolder } = useAccounts()
</script>

<template>
  <div class="container">
    <header>
      <h1><span class="emoji">📬</span> MailMaster</h1>
      <p class="subtitle">Account: {{ current?.name ?? '—' }}</p>
      <nav class="nav-links">
        <NuxtLink to="/kameo-utbetalningar">💸 Kameo utbetalningar</NuxtLink>
      </nav>
    </header>

    <AccountSelector />

    <EmailControls
      :folder="folder"
      :folders="folders"
      :loading="loading"
      :analyzing="analyzing"
      :disabled="needsLogin"
      @update:folder="selectFolder"
      @fetch="fetchEmails"
      @analyze="analyzeEmails"
    />

    <div v-if="error" class="error">
      ❌ {{ error }}
    </div>

    <EmailList
      v-if="emails.length > 0"
      :emails="emails"
      :folder="folder"
      :analyses="analyses"
      :decisions="decisions"
      :deciding-uid="decidingUid"
      @decide="decideEmail"
    />

    <div v-else-if="!loading && !error" class="empty">
      <p>No emails loaded yet</p>
      <p class="hint">Click the button above to fetch emails</p>
    </div>
  </div>
</template>

<style>
* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

body {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  background: #0f0f0f;
  color: #fff;
  min-height: 100vh;
}

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
  font-size: 2.5rem;
  margin-bottom: 0.5rem;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.emoji {
  background: none;
  -webkit-background-clip: unset;
  -webkit-text-fill-color: initial !important;
  background-clip: unset;
  color: #fff;
}

.subtitle {
  color: #888;
  font-size: 0.9rem;
}

.nav-links {
  margin-top: 0.75rem;
}

.nav-links a {
  color: #b9a7ff;
  text-decoration: none;
  font-size: 0.95rem;
}

.nav-links a:hover {
  text-decoration: underline;
}

.error {
  background: rgba(239, 68, 68, 0.1);
  border: 1px solid #ef4444;
  border-radius: 8px;
  padding: 1rem;
  text-align: center;
  color: #ef4444;
  margin-bottom: 1rem;
}

.empty {
  text-align: center;
  padding: 4rem 2rem;
  color: #666;
}

.hint {
  margin-top: 0.5rem;
  font-size: 0.85rem;
  color: #444;
}
</style>
