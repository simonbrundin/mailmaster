<script setup lang="ts">
import { useRulesEditor } from '~/composables/useRulesEditor'

const router = useRouter()

const {
  content,
  loading,
  saving,
  error,
  success,
  open,
  close,
  save,
  handleTab
} = useRulesEditor()

onMounted(() => {
  open()
})

function handleBack() {
  router.push('/')
}

async function handleSave() {
  await save()
  if (!error.value && success.value) {
    // Navigate after showing success message
    setTimeout(() => {
      router.push('/')
    }, 1000)
  }
}

function onContentUpdate(value: string) {
  content.value = value
}
</script>

<template>
  <div class="container">
    <header>
      <button class="back-btn" @click="handleBack">
        ← Tillbaka
      </button>
      <h1>📝 Redigera Regler</h1>
    </header>

    <div v-if="loading" class="loading">
      Laddar...
    </div>

    <div v-else class="editor-container">
      <div class="editor-layout">
        <div class="editor-pane">
          <h3>Regler (Markdown)</h3>
          <textarea
            :value="content"
            class="textarea"
            placeholder="Skriv dina regler här..."
            spellcheck="false"
            @input="onContentUpdate(($event.target as HTMLTextAreaElement).value)"
            @keydown.tab.prevent="handleTab"
          ></textarea>
        </div>

        <div class="preview-pane">
          <h3>Förhandsvisning</h3>
          <div class="preview" v-html="renderMarkdown(content)"></div>
        </div>
      </div>

      <div v-if="error" class="error">
        ❌ {{ error }}
      </div>

      <div v-if="success" class="success">
        ✅ {{ success }}
      </div>

      <div class="actions">
        <button @click="handleSave" :disabled="saving" class="btn-save">
          {{ saving ? 'Sparar...' : '💾 Spara regler' }}
        </button>
        <button @click="handleBack" class="btn-cancel">
          Avbryt
        </button>
      </div>
    </div>
  </div>
</template>

<script lang="ts">
import { useMarkdownRenderer } from '~/composables/useMarkdownRenderer'

// Add renderMarkdown to the component
const { renderMarkdown } = useMarkdownRenderer()
</script>

<style scoped>
.container {
  max-width: 1200px;
  margin: 0 auto;
  padding: 2rem;
}

header {
  display: flex;
  align-items: center;
  gap: 1.5rem;
  margin-bottom: 2rem;
}

.back-btn {
  padding: 0.5rem 1rem;
  border-radius: 8px;
  border: 1px solid #333;
  background: transparent;
  color: #888;
  font-size: 0.9rem;
  cursor: pointer;
  transition: border-color 0.2s, color 0.2s;
}

.back-btn:hover {
  border-color: #667eea;
  color: #fff;
}

h1 {
  font-size: 1.75rem;
  color: #fff;
}

.loading {
  text-align: center;
  padding: 4rem;
  color: #888;
}

.editor-container {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

.editor-layout {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 1.5rem;
  min-height: 500px;
}

.editor-pane,
.preview-pane {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.editor-pane h3,
.preview-pane h3 {
  color: #888;
  font-size: 0.85rem;
  font-weight: 500;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.textarea {
  flex: 1;
  border: 1px solid #333;
  border-radius: 12px;
  background: #1a1a1a;
  padding: 1.25rem;
  color: #ccc;
  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
  font-size: 0.9rem;
  line-height: 1.7;
  resize: none;
  outline: none;
  white-space: pre-wrap;
  word-wrap: break-word;
}

.textarea:focus {
  border-color: #667eea;
}

.textarea::placeholder {
  color: #444;
  font-style: italic;
}

.preview {
  flex: 1;
  border: 1px solid #333;
  border-radius: 12px;
  background: #1a1a1a;
  padding: 1.25rem;
  overflow-y: auto;
  color: #ccc;
  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
  font-size: 0.9rem;
  line-height: 1.7;
}

.preview :deep(h1) {
  font-size: 1.75rem;
  color: #c6a0f6;
  margin: 1.5rem 0 1rem 0;
  font-weight: 700;
  border-bottom: 1px solid #313244;
  padding-bottom: 0.5rem;
}

.preview :deep(h2) {
  font-size: 1.35rem;
  color: #89b4fa;
  margin: 1.75rem 0 0.75rem 0;
  font-weight: 600;
}

.preview :deep(h3) {
  font-size: 1.1rem;
  color: #89dceb;
  margin: 1.25rem 0 0.5rem 0;
  font-weight: 600;
}

.preview :deep(strong) {
  color: #a6e3a1;
  font-weight: 700;
}

.preview :deep(em) {
  color: #fab387;
  font-style: italic;
}

.preview :deep(code) {
  background: #1e1e2e;
  padding: 0.2rem 0.5rem;
  border-radius: 6px;
  color: #f5c2e7;
}

.preview :deep(li) {
  margin-left: 1.75rem;
  margin-bottom: 0.35rem;
  list-style-type: disc;
  color: #cdd6f4;
}

.preview :deep(.placeholder) {
  color: #444;
  font-style: italic;
}

.preview :deep(.md-syntax) {
  color: #6c7086;
}

.error {
  background: rgba(239, 68, 68, 0.1);
  border: 1px solid #ef4444;
  border-radius: 8px;
  padding: 0.75rem 1rem;
  color: #ef4444;
}

.success {
  background: rgba(34, 197, 94, 0.1);
  border: 1px solid #22c55e;
  border-radius: 8px;
  padding: 0.75rem 1rem;
  color: #22c55e;
}

.actions {
  display: flex;
  gap: 1rem;
  justify-content: flex-end;
}

.btn-save {
  padding: 0.75rem 1.5rem;
  border-radius: 8px;
  border: none;
  background: linear-gradient(135deg, #22c55e 0%, #16a34a 100%);
  color: #fff;
  font-size: 1rem;
  font-weight: 600;
  cursor: pointer;
  transition: transform 0.2s;
}

.btn-save:hover:not(:disabled) {
  transform: translateY(-2px);
}

.btn-save:disabled {
  opacity: 0.7;
  cursor: not-allowed;
}

.btn-cancel {
  padding: 0.75rem 1.5rem;
  border-radius: 8px;
  border: 1px solid #333;
  background: transparent;
  color: #888;
  font-size: 1rem;
  cursor: pointer;
  transition: border-color 0.2s, color 0.2s;
}

.btn-cancel:hover {
  border-color: #666;
  color: #fff;
}
</style>
