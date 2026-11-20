<script setup lang="ts">
import type { Analysis, Decision, Email } from '~/composables/useEmailProcessing'

const props = defineProps<{
  emails: Email[]
  folder: string
  analyses: Record<number, Analysis>
  decisions: Record<number, Decision>
  decidingUid: number | null
}>()

const emit = defineEmits<{
  (e: 'decide', uid: number, decision: 'approve' | 'deny'): void
}>()

function analysisFor(uid: number): Analysis | undefined {
  return props.analyses[uid]
}

function decisionFor(uid: number): Decision | undefined {
  return props.decisions[uid]
}
</script>

<template>
  <div class="email-list">
    <div class="email-count">{{ emails.length }} emails in {{ folder }}</div>

    <div
      v-for="email in emails"
      :key="email.uid"
      class="email-card"
      :class="{ analyzed: analysisFor(email.uid) }"
    >
      <div class="email-main">
        <div class="email-header">
          <span class="email-from">{{ email.from }}</span>
          <span class="email-date">{{ email.date }}</span>
        </div>
        <div class="email-subject">{{ email.subject }}</div>
        <div v-if="email.has_attachment" class="email-attachment">
          📎 Attachment
        </div>

        <template v-if="analysisFor(email.uid)">
          <p class="analysis-description">
            {{ analysisFor(email.uid)!.description }}
          </p>

          <div class="suggestions">
            <div class="suggestions-header">
              <span class="suggestions-title">Rekommenderade åtgärder</span>
              <span
                v-if="analysisFor(email.uid)!.rule"
                class="rule-chip"
              >{{ analysisFor(email.uid)!.rule }}</span>
              <span class="confidence">{{ analysisFor(email.uid)!.confidence }}</span>
            </div>

            <ul v-if="analysisFor(email.uid)!.actions.length > 0" class="suggestion-list">
              <li
                v-for="action in analysisFor(email.uid)!.actions"
                :key="action.type"
                :class="'suggestion-' + action.type"
              >
                {{ action.description }}
              </li>
            </ul>
            <p v-else class="no-actions">Inga åtgärder föreslagna.</p>

            <p
              v-if="analysisFor(email.uid)!.motivation"
              class="motivation"
            >{{ analysisFor(email.uid)!.motivation }}</p>

            <p
              v-if="analysisFor(email.uid)!.error"
              class="analysis-error"
            >⚠️ {{ analysisFor(email.uid)!.error }}</p>
          </div>
        </template>
      </div>

      <div
        v-if="analysisFor(email.uid)"
        class="email-actions"
      >
        <button
          class="btn btn-ok"
          :disabled="decidingUid !== null || decisionFor(email.uid)?.state === 'approved' || decisionFor(email.uid)?.state === 'denied'"
          @click="emit('decide', email.uid, 'approve')"
        >
          ✓ OK
        </button>
        <button
          class="btn btn-deny"
          :disabled="decidingUid !== null || decisionFor(email.uid)?.state === 'approved' || decisionFor(email.uid)?.state === 'denied'"
          @click="emit('decide', email.uid, 'deny')"
        >
          ✕ Neka
        </button>

        <div
          v-if="decisionFor(email.uid)"
          class="decision"
          :class="'decision-' + decisionFor(email.uid)!.state"
        >
          <template v-if="decisionFor(email.uid)!.state === 'approved'">✅ Godkänd</template>
          <template v-else-if="decisionFor(email.uid)!.state === 'denied'">🚫 Avböjd</template>
          <template v-else-if="decisionFor(email.uid)!.state === 'error'">
            ❌ {{ decisionFor(email.uid)!.message }}
          </template>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.email-list {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.email-count {
  color: #888;
  font-size: 0.85rem;
  margin-bottom: 1rem;
  text-align: center;
}

.email-card {
  background: #1a1a1a;
  border: 1px solid #2a2a2a;
  border-radius: 12px;
  padding: 1rem 1.25rem;
  transition: border-color 0.2s, transform 0.2s;
  display: flex;
  gap: 1.25rem;
  align-items: flex-start;
}

.email-card:hover {
  border-color: #444;
  transform: translateX(4px);
}

.email-card.analyzed {
  border-color: #3f3f56;
}

.email-main {
  flex: 1;
  min-width: 0;
}

.email-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.5rem;
}

.email-from {
  font-weight: 600;
  color: #e0e0e0;
  font-size: 0.95rem;
}

.email-date {
  color: #666;
  font-size: 0.8rem;
}

.email-subject {
  color: #aaa;
  font-size: 0.9rem;
  line-height: 1.4;
}

.email-attachment {
  margin-top: 0.5rem;
  color: #667eea;
  font-size: 0.8rem;
}

.analysis-description {
  margin-top: 0.75rem;
  color: #d8d8e0;
  font-size: 0.9rem;
  line-height: 1.45;
}

.suggestions {
  margin-top: 0.6rem;
  background: #232323;
  border: 1px solid #333;
  border-left: 3px solid #667eea;
  border-radius: 8px;
  padding: 0.75rem 0.9rem;
}

.suggestions-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-wrap: wrap;
  margin-bottom: 0.5rem;
}

.suggestions-title {
  color: #fff;
  font-size: 0.8rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.rule-chip {
  background: #3b82f6;
  color: #fff;
  border-radius: 999px;
  padding: 0.1rem 0.6rem;
  font-size: 0.75rem;
}

.confidence {
  color: #777;
  font-size: 0.75rem;
  margin-left: auto;
}

.suggestion-list {
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
}

.suggestion-list li {
  color: #cfcfd8;
  font-size: 0.87rem;
  padding-left: 1rem;
  position: relative;
}

.suggestion-list li::before {
  content: '▸';
  position: absolute;
  left: 0;
  color: #667eea;
}

.no-actions {
  color: #777;
  font-size: 0.85rem;
}

.motivation {
  margin-top: 0.6rem;
  color: #888;
  font-size: 0.8rem;
  line-height: 1.4;
  font-style: italic;
}

.analysis-error {
  margin-top: 0.5rem;
  color: #ef4444;
  font-size: 0.8rem;
}

.email-actions {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  align-self: flex-start;
  min-width: 8rem;
}

.btn {
  padding: 0.5rem 1.1rem;
  border-radius: 8px;
  border: none;
  color: #fff;
  font-size: 0.9rem;
  font-weight: 600;
  cursor: pointer;
  transition: transform 0.2s, opacity 0.2s;
}

.btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn-ok {
  background: linear-gradient(135deg, #22c55e 0%, #16a34a 100%);
}

.btn-ok:hover:not(:disabled) {
  transform: translateY(-2px);
}

.btn-deny {
  background: linear-gradient(135deg, #ef4444 0%, #dc2626 100%);
}

.btn-deny:hover:not(:disabled) {
  transform: translateY(-2px);
}

.decision {
  font-size: 0.78rem;
  text-align: center;
  line-height: 1.3;
}

.decision-approved {
  color: #22c55e;
}

.decision-denied {
  color: #888;
}

.decision-error {
  color: #ef4444;
}
</style>
