<script setup lang="ts">
import type { Email } from '~/composables/useEmailProcessing'

defineProps<{
  emails: Email[]
  folder: string
}>()

const emit = defineEmits<{
  (e: 'formatDate', date: string): string
}>()
</script>

<template>
  <div class="email-list">
    <div class="email-count">{{ emails.length }} emails in {{ folder }}</div>

    <div
      v-for="email in emails"
      :key="email.uid"
      class="email-card"
    >
      <div class="email-header">
        <span class="email-from">{{ email.from }}</span>
        <span class="email-date">{{ email.date }}</span>
      </div>
      <div class="email-subject">{{ email.subject }}</div>
      <div v-if="email.has_attachment" class="email-attachment">
        📎 Attachment
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
}

.email-card:hover {
  border-color: #444;
  transform: translateX(4px);
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
</style>
