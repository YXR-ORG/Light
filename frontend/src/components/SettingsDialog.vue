<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useSettingsStore } from '../stores/settings'
import { Get, Set } from '../../wailsjs/go/handler/SettingsHandler'

const settingsStore = useSettingsStore()
const tabs = ['openai', 'claude', 'ollama'] as const
type Provider = typeof tabs[number]

const activeTab = ref<Provider>('openai')
const openaiKey = ref(''); const openaiBaseURL = ref('')
const claudeKey = ref(''); const claudeBaseURL = ref('')
const ollamaBaseURL = ref('http://localhost:11434')
const defaultModel = ref('gpt-4o')
const saving = ref(false)

const providerLabels: Record<Provider, string> = {
  openai: 'OpenAI', claude: 'Claude', ollama: 'Ollama',
}

onMounted(loadSettings)

async function loadSettings() {
  openaiKey.value = await Get('openai_api_key').catch(() => '')
  openaiBaseURL.value = await Get('openai_base_url').catch(() => '')
  claudeKey.value = await Get('claude_api_key').catch(() => '')
  claudeBaseURL.value = await Get('claude_base_url').catch(() => '')
  ollamaBaseURL.value = await Get('ollama_base_url').catch(() => 'http://localhost:11434')
  defaultModel.value = await Get('default_model').catch(() => 'gpt-4o')
}

async function save() {
  saving.value = true
  try {
    if (openaiKey.value) await Set('openai_api_key', openaiKey.value)
    if (openaiBaseURL.value) await Set('openai_base_url', openaiBaseURL.value)
    if (claudeKey.value) await Set('claude_api_key', claudeKey.value)
    if (claudeBaseURL.value) await Set('claude_base_url', claudeBaseURL.value)
    if (ollamaBaseURL.value) await Set('ollama_base_url', ollamaBaseURL.value)
    await Set('default_model', defaultModel.value)
    settingsStore.setOpen(false)
  } catch (e: any) {
    console.error('保存失败', e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <Teleport to="body">
    <div v-if="settingsStore.open" class="overlay" @click.self="settingsStore.setOpen(false)">
      <div class="modal">
        <div class="modal-header">
          <h2>设置</h2>
          <button class="btn-close" @click="settingsStore.setOpen(false)">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M18 6 6 18M6 6l12 12"/></svg>
          </button>
        </div>

        <div class="tabs">
          <button v-for="t in tabs" :key="t" class="tab" :class="{ active: activeTab === t }" @click="activeTab = t">
            {{ providerLabels[t] }}
          </button>
        </div>

        <div class="modal-body">
          <!-- OpenAI -->
          <div v-if="activeTab === 'openai'" class="tab-content">
            <div class="field">
              <label>API Key</label>
              <input v-model="openaiKey" type="password" placeholder="sk-..." />
            </div>
            <div class="field">
              <label>Base URL <span class="optional">可选</span></label>
              <input v-model="openaiBaseURL" placeholder="https://api.openai.com/v1" />
            </div>
          </div>

          <!-- Claude -->
          <div v-if="activeTab === 'claude'" class="tab-content">
            <div class="field">
              <label>API Key</label>
              <input v-model="claudeKey" type="password" placeholder="sk-ant-..." />
            </div>
            <div class="field">
              <label>Base URL <span class="optional">可选</span></label>
              <input v-model="claudeBaseURL" placeholder="https://api.anthropic.com" />
            </div>
          </div>

          <!-- Ollama -->
          <div v-if="activeTab === 'ollama'" class="tab-content">
            <div class="field">
              <label>服务地址</label>
              <input v-model="ollamaBaseURL" placeholder="http://localhost:11434" />
            </div>
          </div>

      <div class="divider" />

      <div class="field">
        <label>默认模型</label>
        <input v-model="defaultModel" placeholder="gpt-4o, claude-3-opus, qwen2.5" />
      </div>
        </div>

        <div class="modal-footer">
          <button class="btn btn-cancel" @click="settingsStore.setOpen(false)">取消</button>
          <button class="btn btn-primary" @click="save" :disabled="saving">
            {{ saving ? '保存中...' : '保存' }}
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.overlay {
  position: fixed; inset: 0;
  background: oklch(0 0 0 / 0.35);
  display: flex; align-items: center; justify-content: center;
  z-index: 1000;
  animation: fadeIn var(--duration-normal) var(--ease-out);
}

@keyframes fadeIn { from { opacity: 0; } to { opacity: 1; } }

.modal {
  background: var(--color-paper);
  border-radius: var(--radius-xl);
  width: 480px;
  max-height: 80vh;
  display: flex;
  flex-direction: column;
  box-shadow: var(--shadow-lg);
  animation: slideUp var(--duration-slow) var(--ease-out);
}

@keyframes slideUp { from { opacity: 0; transform: translateY(8px); } to { opacity: 1; transform: translateY(0); } }

.modal-header {
  display: flex; align-items: center; justify-content: space-between;
  padding: var(--space-5) var(--space-6);
}

.modal-header h2 { font-size: var(--text-lg); font-weight: 600; margin: 0; }

.btn-close {
  width: 32px; height: 32px; display: flex; align-items: center; justify-content: center;
  border: none; border-radius: var(--radius-md); background: transparent;
  color: var(--color-text-3); cursor: pointer; transition: background var(--duration-fast) var(--ease-out);
}
.btn-close:hover { background: var(--color-paper-3); color: var(--color-text); }

.tabs {
  display: flex; gap: var(--space-1); padding: 0 var(--space-6);
  border-bottom: 1px solid var(--color-border);
}

.tab {
  padding: var(--space-3) var(--space-4);
  border: none; background: transparent; cursor: pointer;
  font-size: var(--text-sm); color: var(--color-text-3); font-family: inherit;
  border-bottom: 2px solid transparent;
  margin-bottom: -1px;
  transition: color var(--duration-fast) var(--ease-out), border-color var(--duration-fast) var(--ease-out);
}

.tab.active { color: var(--color-text); border-bottom-color: var(--color-accent); }
.tab:hover:not(.active) { color: var(--color-text-2); }

.modal-body {
  padding: var(--space-5) var(--space-6);
  overflow-y: auto; flex: 1;
}

.tab-content { display: flex; flex-direction: column; gap: var(--space-4); }

.field label {
  display: block; font-size: var(--text-xs); font-weight: 500;
  color: var(--color-text-2); margin-bottom: var(--space-1);
}

.optional { color: var(--color-text-3); font-weight: 400; }

.field input, .field select {
  width: 100%; padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border); border-radius: var(--radius-md);
  font-size: var(--text-sm); font-family: inherit; color: var(--color-text);
  background: var(--color-paper); outline: none;
  transition: border-color var(--duration-fast) var(--ease-out);
}

.field input:focus, .field select:focus { border-color: var(--color-accent); box-shadow: 0 0 0 3px var(--color-accent-soft); }

.divider { height: 1px; background: var(--color-border); margin: var(--space-5) 0; }


.modal-footer {
  display: flex; justify-content: flex-end; gap: var(--space-2);
  padding: var(--space-4) var(--space-6);
  border-top: 1px solid var(--color-border);
}

.btn {
  padding: var(--space-2) var(--space-5);
  border-radius: var(--radius-md); font-size: var(--text-sm);
  font-family: inherit; cursor: pointer; border: none;
  transition: background var(--duration-fast) var(--ease-out), opacity var(--duration-fast) var(--ease-out);
}

.btn-cancel {
  background: var(--color-paper-3); color: var(--color-text-2);
}
.btn-cancel:hover { background: var(--color-paper-4); }

.btn-primary {
  background: var(--color-accent); color: #fff;
}
.btn-primary:hover { background: var(--color-accent-2); }
.btn-primary:disabled { opacity: 0.5; cursor: not-allowed; }
</style>
