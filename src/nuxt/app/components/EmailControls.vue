<script setup lang="ts">
defineProps<{
  folder: string
  folders: readonly string[]
  loading: boolean
  analyzing: boolean
  disabled?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:folder', value: string): void
  (e: 'fetch'): void
  (e: 'analyze'): void
}>()
</script>

<template>
  <div class="controls">
    <select
      :value="folder"
      class="folder-select"
      :disabled="disabled || folders.length === 0"
      @change="emit('update:folder', ($event.target as HTMLSelectElement).value)"
    >
      <option v-for="f in folders" :key="f" :value="f">{{ f }}</option>
    </select>

    <button
      @click="emit('fetch')"
      :disabled="loading || disabled"
      class="btn btn-primary"
    >
      {{ loading ? '⏳ Loading...' : '📥 Hämta mejl' }}
    </button>

    <button
      @click="emit('analyze')"
      :disabled="analyzing || disabled"
      class="btn btn-orange"
    >
      {{ analyzing ? '🤖 Analyserar...' : '🤖 Processa mejl' }}
    </button>

    <NuxtLink to="/rules" class="btn btn-secondary">
      📝 Redigera regler
    </NuxtLink>
  </div>
</template>

<style scoped>
.controls {
  display: flex;
  gap: 1rem;
  justify-content: center;
  margin-bottom: 2rem;
}

.folder-select {
  padding: 0.75rem 1rem;
  border-radius: 8px;
  border: 1px solid #333;
  background: #1a1a1a;
  color: #fff;
  font-size: 1rem;
  cursor: pointer;
}

.folder-select:focus {
  outline: none;
  border-color: #667eea;
}

.btn {
  padding: 0.75rem 1.5rem;
  border-radius: 8px;
  border: none;
  color: #fff;
  font-size: 1rem;
  font-weight: 600;
  cursor: pointer;
  transition: transform 0.2s, opacity 0.2s;
}

.btn:disabled {
  opacity: 0.7;
  cursor: not-allowed;
}

.btn-primary {
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
}

.btn-primary:hover:not(:disabled) {
  transform: translateY(-2px);
}

.btn-orange {
  background: linear-gradient(135deg, #f97316 0%, #ea580c 100%);
}

.btn-orange:hover:not(:disabled) {
  transform: translateY(-2px);
}

.btn-secondary {
  background: #1a1a1a;
  border: 1px solid #333;
}

.btn-secondary:hover {
  border-color: #667eea;
  transform: translateY(-2px);
}
</style>
