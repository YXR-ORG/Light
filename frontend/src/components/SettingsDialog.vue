<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useSettingsStore } from '../stores/settings'
import { Get, Set } from '../../wailsjs/go/handler/SettingsHandler'

const settingsStore = useSettingsStore()

const openaiKey = ref('')
const openaiBaseURL = ref('')
const claudeKey = ref('')
const claudeBaseURL = ref('')
const ollamaBaseURL = ref('http://localhost:11434')
const defaultProvider = ref('openai')
const defaultModel = ref('gpt-4o')
const saving = ref(false)

onMounted(() => {
  loadSettings()
})

async function loadSettings() {
  openaiKey.value = await Get('openai_api_key').catch(() => '')
  openaiBaseURL.value = await Get('openai_base_url').catch(() => '')
  claudeKey.value = await Get('claude_api_key').catch(() => '')
  claudeBaseURL.value = await Get('claude_base_url').catch(() => '')
  ollamaBaseURL.value = await Get('ollama_base_url').catch(() => 'http://localhost:11434')
  defaultProvider.value = await Get('default_provider').catch(() => 'openai')
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
    await Set('default_provider', defaultProvider.value)
    await Set('default_model', defaultModel.value)
    settingsStore.setOpen(false)
  } catch (e: any) {
    console.error('save settings failed', e)
  } finally {
    saving.value = false
  }
}

function close() {
  settingsStore.setOpen(false)
}
</script>

<template>
  <Teleport to="body">
    <div v-if="settingsStore.open" class="modal-overlay" @click.self="close">
      <div class="modal">
        <div class="modal-header">
          <h2>设置</h2>
          <button class="close-btn" @click="close">×</button>
        </div>
        <div class="modal-body">
          <section>
            <h3>OpenAI</h3>
            <div class="field">
              <label>API Key</label>
              <input v-model="openaiKey" type="password" placeholder="sk-..." />
            </div>
            <div class="field">
              <label>Base URL（可选）</label>
              <input v-model="openaiBaseURL" placeholder="https://api.openai.com/v1" />
            </div>
          </section>
          <section>
            <h3>Anthropic Claude</h3>
            <div class="field">
              <label>API Key</label>
              <input v-model="claudeKey" type="password" placeholder="sk-ant-..." />
            </div>
            <div class="field">
              <label>Base URL（可选）</label>
              <input v-model="claudeBaseURL" placeholder="https://api.anthropic.com" />
            </div>
          </section>
          <section>
            <h3>Ollama（本地）</h3>
            <div class="field">
              <label>服务地址</label>
              <input v-model="ollamaBaseURL" placeholder="http://localhost:11434" />
            </div>
          </section>
          <section>
            <h3>默认配置</h3>
            <div class="field">
              <label>默认供应商</label>
              <select v-model="defaultProvider">
                <option value="openai">OpenAI</option>
                <option value="claude">Claude</option>
                <option value="ollama">Ollama</option>
              </select>
            </div>
            <div class="field">
              <label>默认模型</label>
              <input v-model="defaultModel" placeholder="gpt-4o / claude-3-opus / qwen2.5" />
            </div>
          </section>
        </div>
        <div class="modal-footer">
          <button class="btn-cancel" @click="close">取消</button>
          <button class="btn-save" @click="save" :disabled="saving">
            {{ saving ? '保存中...' : '保存' }}
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.modal-overlay {
  position: fixed; inset: 0; background: rgba(0,0,0,0.4);
  display: flex; align-items: center; justify-content: center; z-index: 1000;
}
.modal {
  background: #fff; border-radius: 12px; width: 520px; max-height: 80vh;
  display: flex; flex-direction: column; box-shadow: 0 20px 60px rgba(0,0,0,0.15);
}
.modal-header {
  display: flex; align-items: center; justify-content: space-between;
  padding: 16px 20px; border-bottom: 1px solid var(--border-color);
}
.modal-header h2 { margin: 0; font-size: 18px; }
.close-btn {
  border: none; background: none; font-size: 24px; cursor: pointer;
  color: var(--text-secondary);
}
.modal-body {
  padding: 20px; overflow-y: auto; flex: 1;
}
section { margin-bottom: 20px; }
section h3 { font-size: 14px; font-weight: 600; margin-bottom: 12px; color: var(--text); }
.field { margin-bottom: 12px; }
.field label { display: block; font-size: 12px; color: var(--text-secondary); margin-bottom: 4px; }
.field input, .field select {
  width: 100%; padding: 8px 12px; border: 1px solid var(--border-color);
  border-radius: 6px; font-size: 13px; font-family: inherit; outline: none;
}
.field input:focus, .field select:focus { border-color: var(--accent); }
.modal-footer {
  display: flex; justify-content: flex-end; gap: 8px;
  padding: 12px 20px; border-top: 1px solid var(--border-color);
}
.btn-cancel, .btn-save {
  padding: 8px 20px; border-radius: 6px; font-size: 13px; cursor: pointer; border: none;
}
.btn-cancel { background: #f3f4f6; color: var(--text); }
.btn-save { background: var(--accent); color: #fff; }
.btn-save:disabled { opacity: 0.6; cursor: not-allowed; }
</style>
