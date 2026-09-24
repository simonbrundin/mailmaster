<script setup lang="ts">
import type { ProcessResult } from '~/composables/useEmailProcessing'

defineProps<{
  show: boolean
  results: ProcessResult[]
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()
</script>

<template>
  <Teleport to="body">
    <div v-if="show" class="overlay" @click.self="emit('close')">
      <div class="modal">
        <div class="header">
          <h2>📊 Resultat</h2>
          <button class="close-btn" @click="emit('close')">✕</button>
        </div>

        <div class="summary">
          <span class="success">✅ {{ results.filter(r => r.success).length }} lyckades</span>
          <span class="error">❌ {{ results.filter(r => !r.success).length }} misslyckades</span>
        </div>

        <div class="results">
          <div
            v-for="result in results"
            :key="result.uid"
            class="card"
            :class="{ error: !result.success }"
          >
            <div class="card-header">
              <span class="uid">#{{ result.uid }}</span>
              <span class="action" :class="'action-' + result.action">
                {{ result.action }}
                <span class="confidence">({{ result.confidence }})</span>
              </span>
            </div>
            <div class="subject">{{ result.subject }}</div>
            <div class="motivation">{{ result.motivation }}</div>
            <div v-if="result.error" class="error-text">⚠️ {{ result.error }}</div>
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.8);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  padding: 2rem;
}

.modal {
  background: #1a1a1a;
  border: 1px solid #333;
  border-radius: 16px;
  max-width: 700px;
  width: 100%;
  max-height: 80vh;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 1.25rem 1.5rem;
  border-bottom: 1px solid #333;
}

.header h2 {
  font-size: 1.25rem;
  color: #fff;
}

.close-btn {
  background: transparent;
  border: none;
  color: #888;
  font-size: 1.25rem;
  cursor: pointer;
  padding: 0.5rem;
}

.close-btn:hover {
  color: #fff;
}

.summary {
  padding: 1rem 1.5rem;
  border-bottom: 1px solid #333;
  display: flex;
  gap: 1.5rem;
}

.success {
  color: #22c55e;
}

.error {
  color: #ef4444;
}

.results {
  padding: 1rem 1.5rem;
  overflow-y: auto;
  flex: 1;
}

.card {
  background: #252525;
  border: 1px solid #333;
  border-radius: 8px;
  padding: 1rem;
  margin-bottom: 0.75rem;
}

.card.error {
  border-color: #ef4444;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.5rem;
}

.uid {
  color: #666;
  font-size: 0.8rem;
}

.action {
  font-weight: 600;
  padding: 0.25rem 0.5rem;
  border-radius: 4px;
  font-size: 0.85rem;
  text-transform: uppercase;
}

.action-read { background: #3b82f6; color: #fff; }
.action-archive { background: #8b5cf6; color: #fff; }
.action-reply { background: #22c55e; color: #fff; }
.action-forward { background: #f59e0b; color: #fff; }
.action-move { background: #ec4899; color: #fff; }
.action-uncertain { background: #6b7280; color: #fff; }

.confidence {
  font-weight: 400;
  opacity: 0.8;
}

.subject {
  color: #e0e0e0;
  margin-bottom: 0.5rem;
  font-size: 0.95rem;
}

.motivation {
  color: #888;
  font-size: 0.85rem;
  line-height: 1.4;
}

.error-text {
  color: #ef4444;
  font-size: 0.85rem;
  margin-top: 0.5rem;
}
</style>
