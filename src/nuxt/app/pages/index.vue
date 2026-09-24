<script setup lang="ts">
import { useEmailProcessing } from '~/composables/useEmailProcessing'

const config = useAppConfig()

const {
  emails, loading, processing, error, processResults,
  showResults, folder, folders, fetchEmails,
  processEmails, closeResults
} = useEmailProcessing()
</script>

<template>
  <div class="container">
    <header>
      <h1><span class="emoji">📬</span> MailMaster</h1>
      <p class="subtitle">Account: {{ config.account }}</p>
    </header>

    <EmailControls
      :folder="folder"
      :folders="folders"
      :loading="loading"
      :processing="processing"
      @update:folder="folder = $event as any"
      @fetch="fetchEmails"
      @process="processEmails"
    />

    <div v-if="error" class="error">
      ❌ {{ error }}
    </div>

    <EmailList
      v-if="emails.length > 0"
      :emails="emails"
      :folder="folder"
    />

    <div v-else-if="!loading && !error" class="empty">
      <p>No emails loaded yet</p>
      <p class="hint">Click the button above to fetch emails</p>
    </div>

    <ResultsModal
      :show="showResults"
      :results="processResults"
      @close="closeResults"
    />
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
  max-width: 800px;
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
