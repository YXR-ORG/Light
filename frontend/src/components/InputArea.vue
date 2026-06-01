<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useChatStore } from '../stores/chat'
import { StreamChat, CancelStream } from '../../wailsjs/go/handler/ChatHandler'
import { Get } from '../../wailsjs/go/handler/SettingsHandler'
import { SetModel } from '../../wailsjs/go/handler/ConversationHandler'
import { ListEnabledModels, ListProviders } from '../../wailsjs/go/handler/ProviderHandler'
import { ListSkills } from '../../wailsjs/go/handler/SkillHandler'
import SkillsPanel from './SkillsPanel.vue'
import { handler as handlerModels, type storage } from '../../wailsjs/go/models'

const store = useChatStore()
const input = ref('')
const showSkills = ref(false)
const showModelPicker = ref(false)
const showSkillPicker = ref(false)
const selectedSkillIDs = ref<string[]>([])
const webSearch = ref(false)
const ignoreContext = computed(() => store.contextCutoffId !== null)

interface Attachment { name: string; mime_type: string; data: string }
const attachments = ref<Attachment[]>([])
const fileInputRef = ref<HTMLInputElement | null>(null)

const MAX_FILE_SIZE = 10 * 1024 * 1024 // 10MB

async function handleFileSelect(e: Event) {
  const files = (e.target as HTMLInputElement).files
  if (!files) return
  for (const file of Array.from(files)) {
    if (file.size > MAX_FILE_SIZE) {
      alert(`文件 ${file.name} 超过 10MB 限制`)
      continue
    }
    const buf = await file.arrayBuffer()
    const bytes = new Uint8Array(buf)
    let binary = ''
    for (let i = 0; i < bytes.byteLength; i++) binary += String.fromCharCode(bytes[i])
    const b64 = btoa(binary)
    attachments.value.push({ name: file.name, mime_type: file.type || 'application/octet-stream', data: b64 })
  }
  ;(e.target as HTMLInputElement).value = ''
}

function removeAttachment(idx: number) {
  attachments.value.splice(idx, 1)
}

// 已启用供应商的模型列表
const enabledModels = ref<storage.LLMModel[]>([])
const providerMap = ref<Record<string, storage.LLMProvider>>({})
const availableSkills = ref<storage.Skill[]>([])

async function loadEnabledModels() {
  const [models, providers] = await Promise.all([
    ListEnabledModels().catch(() => []),
    ListProviders().catch(() => []),
  ])
  enabledModels.value = models
  providerMap.value = Object.fromEntries(providers.map(p => [p.id, p]))
}

async function loadSkills() {
  availableSkills.value = await ListSkills().catch(() => [])
}

function toggleSkillID(id: string) {
  const idx = selectedSkillIDs.value.indexOf(id)
  if (idx >= 0) selectedSkillIDs.value.splice(idx, 1)
  else selectedSkillIDs.value.push(id)
}

function toggleSkillPicker() {
  showSkillPicker.value = !showSkillPicker.value
  showSkills.value = false
  showModelPicker.value = false
  if (showSkillPicker.value) loadSkills()
}

onMounted(() => {
  loadEnabledModels()
  document.addEventListener('mousedown', onClickOutside)
})
onUnmounted(() => document.removeEventListener('mousedown', onClickOutside))

// 按供应商分组
const groupedModels = computed(() => {
  const groups: Record<string, { provider: storage.LLMProvider; models: storage.LLMModel[] }> = {}
  for (const m of enabledModels.value) {
    const p = providerMap.value[m.provider_id]
    if (!p) continue
    if (!groups[p.id]) groups[p.id] = { provider: p, models: [] }
    groups[p.id].models.push(m)
  }
  return Object.values(groups)
})

const SKILL_NAMES: Record<string, string> = {
  'You are an expert programmer with deep knowledge of software engineering, algorithms, and system design. Provide clear, efficient, and well-documented code. Explain your reasoning and highlight potential edge cases or improvements.': '💻 代码专家',
  "You are a professional writing assistant. Help users craft clear, engaging, and well-structured content. Offer suggestions for improving clarity, tone, and style while preserving the author's voice.": '✍️ 写作助手',
  'You are a professional translator with expertise in multiple languages. Provide accurate, natural-sounding translations that preserve the original meaning, tone, and cultural nuances. When ambiguous, offer alternative translations with brief explanations.': '🌐 翻译专家',
  'You are an expert data analyst. Help users interpret data, identify patterns, and draw meaningful insights. Suggest appropriate statistical methods, visualization techniques, and provide clear explanations of analytical findings.': '📊 数据分析师',
}

const currentConv = computed(() => store.conversations.find(c => c.id === store.currentConvId))
const currentProvider = computed(() => currentConv.value?.provider || '')
const currentModel = computed(() => currentConv.value?.model || '')

const modelLabel = computed(() => {
  if (!currentModel.value) return '选择模型'
  const p = providerMap.value[Object.keys(providerMap.value).find(id => providerMap.value[id].id === currentProvider.value) ?? '']
  // currentProvider 存的是 provider type（如 openai），找匹配的 provider name
  const matchedProvider = Object.values(providerMap.value).find(p => p.type === currentProvider.value || p.id === currentProvider.value)
  return matchedProvider ? `${matchedProvider.name} · ${currentModel.value}` : currentModel.value
})

const activeSkillLabel = computed(() => {
  const prompt = (currentConv.value as any)?.system_prompt || ''
  return SKILL_NAMES[prompt] || null
})

async function selectModel(provider: storage.LLMProvider, modelName: string) {
  if (!store.currentConvId) return
  showModelPicker.value = false
  const conv = store.conversations.find(c => c.id === store.currentConvId)
  if (conv) { conv.provider = provider.type; conv.model = modelName }
  await SetModel(store.currentConvId, provider.type, modelName).catch(console.error)
}

function toggleSkills() { showSkills.value = !showSkills.value; showModelPicker.value = false }
function toggleModelPicker() {
  showModelPicker.value = !showModelPicker.value
  showSkills.value = false
  if (showModelPicker.value) loadEnabledModels()
}

function onClickOutside(e: MouseEvent) {
  const el = document.querySelector('.input-area')
  if (el && !el.contains(e.target as Node)) {
    showSkills.value = false
    showModelPicker.value = false
    showSkillPicker.value = false
    return
  }
  // 点击 input-area 内部但在 skill-picker 外部时关闭 skill-picker
  const skillPicker = document.querySelector('.skill-picker')
  const skillBtn = document.querySelector('.btn-skill-picker')
  if (showSkillPicker.value && skillPicker && !skillPicker.contains(e.target as Node) && !skillBtn?.contains(e.target as Node)) {
    showSkillPicker.value = false
  }
  // 同理 model-picker
  const modelPicker = document.querySelector('.model-picker')
  const modelBtn = document.querySelector('.btn-model')
  if (showModelPicker.value && modelPicker && !modelPicker.contains(e.target as Node) && !modelBtn?.contains(e.target as Node)) {
    showModelPicker.value = false
  }
}

async function send() {
  const text = input.value.trim()
  if (!text || !store.currentConvId) return

  const provider = currentProvider.value || 'openai'

  if (provider !== 'ollama') {
    const hasKey = await Get(`${provider}_api_key`).catch(() => '') || ''
    // 也检查新的 provider 表里的 api_key
    const matchedProvider = Object.values(providerMap.value).find(p => p.type === provider)
    if (!hasKey && !matchedProvider?.api_key) {
      store.resetStream()
      store.appendStream(`⚠️ 请先在「设置」中配置 ${provider.toUpperCase()} 的 API Key，然后重试。`)
      return
    }
  }

  // 有图片附件时检查模型是否支持 vision
  const hasImages = attachments.value.some(a => a.mime_type.startsWith('image/'))
  if (hasImages) {
    const model = currentModel.value || ''
    const visionModels = ['gpt-4o', 'gpt-4-turbo', 'gpt-4-vision', 'claude-3', 'claude-3-5', 'gemini', 'qwen-vl', 'glm-4v']
    const supported = visionModels.some(v => model.toLowerCase().includes(v))
    if (!supported) {
      store.resetStream()
      store.appendStream(`⚠️ 当前模型 **${model}** 不支持图片识别，请切换到支持多模态的模型（如 gpt-4o、claude-3 等）后重试。`)
      return
    }
  }

  input.value = ''
  showSkills.value = false
  showModelPicker.value = false
  showSkillPicker.value = false
  const sentAttachments = [...attachments.value]
  attachments.value = []
  store.resetStream()

  store.appendMessage({
    id: `local-${Date.now()}`,
    conversation_id: store.currentConvId,
    role: 'user',
    content: text,
    tool_calls: '',
    tool_result: '',
    attachments: sentAttachments.length > 0
      ? JSON.stringify(sentAttachments.map(a => ({ name: a.name, mime_type: a.mime_type })))
      : '',
    created_at: new Date().toISOString(),
  } as any)

  store.setStreaming(true)

  const safetyTimer = setTimeout(() => {
    if (store.streaming) {
      store.setStreaming(false)
      store.appendStream('\n\n⚠️ 请求超时，请检查网络或服务状态')
    }
  }, 60000)

  try {
    await StreamChat(handlerModels.SendMessageRequest.createFrom({
      conversation_id: store.currentConvId,
      content: text,
      provider,
      model: currentModel.value || 'gpt-4o',
      skill_ids: selectedSkillIDs.value,
      web_search: webSearch.value,
      ignore_context: false,
      context_cutoff_id: store.contextCutoffId ?? '',
      attachments: sentAttachments,
    }))
  } catch (e: any) {
    const raw = typeof e === 'object' ? JSON.stringify(e) : String(e)
    const msg = e?.message || e?.Message || raw
    console.error('chat error:', raw)
    store.appendStream(`\n\n**错误:** ${msg}`)
    store.setStreaming(false)
  } finally {
    clearTimeout(safetyTimer)
  }
}

async function stop() {
  await CancelStream().catch(() => {})
  store.setStreaming(false)
  store.appendStream('\n\n_已停止_')
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    send()
  }
}
</script>

<template>
  <div class="input-area">
    <!-- Skills 面板 -->
    <transition name="slide-up">
      <div v-if="showSkills" class="popup-panel">
        <SkillsPanel />
      </div>
    </transition>

    <!-- 模型选择面板 -->
    <transition name="slide-up">
      <div v-if="showModelPicker" class="model-picker">
        <div v-if="groupedModels.length === 0" class="model-empty">
          请先在「设置 → 模型供应商」中添加并启用供应商和模型
        </div>
        <div v-for="g in groupedModels" :key="g.provider.id" class="provider-group">
          <div class="provider-label">{{ g.provider.name }}</div>
          <button
            v-for="m in g.models"
            :key="m.id"
            class="model-option"
            :class="{ active: currentProvider === g.provider.type && currentModel === m.name }"
            @click="selectModel(g.provider, m.name)"
          >{{ m.name }}</button>
        </div>
      </div>
    </transition>

    <!-- 技能选择面板 -->
    <transition name="slide-up">
      <div v-if="showSkillPicker" class="skill-picker">
        <div class="skill-picker-header">
          <span class="skill-picker-title">选择技能</span>
          <span class="skill-picker-hint">多选，LLM 自动调用</span>
        </div>
        <div v-if="availableSkills.length === 0" class="model-empty">
          请先在「设置 → Skills 广场」中导入技能
        </div>
        <div v-else class="skill-picker-list">
          <button
            v-for="s in availableSkills"
            :key="s.id"
            class="skill-picker-item"
            :class="{ active: selectedSkillIDs.includes(s.id) }"
            @click="toggleSkillID(s.id)"
          >
            <span class="skill-picker-icon">🔧</span>
            <div class="skill-picker-info">
              <span class="skill-picker-name">{{ s.name }}</span>
              <span class="skill-picker-desc">{{ s.description }}</span>
            </div>
            <span v-if="selectedSkillIDs.includes(s.id)" class="skill-picker-check">
              <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>
            </span>
          </button>
        </div>
        <div v-if="selectedSkillIDs.length > 0" class="skill-picker-footer">
          <button class="skill-clear-btn" @click="selectedSkillIDs = []">清除选择</button>
        </div>
      </div>
    </transition>

    <!-- 附件预览区 -->
    <div v-if="attachments.length > 0" class="attachment-preview">
      <div v-for="(a, idx) in attachments" :key="idx" class="attachment-chip">
        <svg v-if="a.mime_type.startsWith('image/')" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="8.5" cy="8.5" r="1.5"/><polyline points="21 15 16 10 5 21"/></svg>
        <svg v-else width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>
        <span class="attachment-name">{{ a.name }}</span>
        <button class="attachment-remove" @click="removeAttachment(idx)" title="移除">
          <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><path d="M18 6 6 18M6 6l12 12"/></svg>
        </button>
      </div>
    </div>

    <!-- 顶部状态行 -->
    <div class="input-meta" v-if="activeSkillLabel || true">
      <span v-if="activeSkillLabel" class="skill-badge">{{ activeSkillLabel }}</span>
    </div>

    <div class="input-inner">
      <!-- 附件上传按钮：textarea 内左下角 -->
      <input ref="fileInputRef" type="file" multiple
        accept="image/*,.txt,.md,.csv,.json,.py,.js,.ts,.go,.java,.html,.css,.xml,.yaml,.yml,.sh,.sql"
        style="display:none" @change="handleFileSelect" />
      <button class="btn-attach-inner" @click="fileInputRef?.click()"
        :title="'上传文件或图片（最大 10MB）'"
        :class="{ active: attachments.length > 0 }">
        <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
          <path d="M21.44 11.05l-9.19 9.19a6 6 0 0 1-8.49-8.49l9.19-9.19a4 4 0 0 1 5.66 5.66l-9.2 9.19a2 2 0 0 1-2.83-2.83l8.49-8.48"/>
        </svg>
      </button>
      <textarea
        v-model="input"
        placeholder="输入消息..."
        :disabled="store.streaming"
        @keydown="onKeydown"
        @focus="showSkills = false; showModelPicker = false"
        rows="1"
        class="has-attach-btn"
      />
      <div class="input-actions">
        <!-- 技能按钮 -->
        <button class="btn-icon" :class="{ 'btn-icon--active': showSkills }" @click="toggleSkills" title="技能">
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
            <path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/>
          </svg>
        </button>
        <!-- 忽略上下文按钮 -->
        <button
          class="btn-ignore-ctx"
          :class="{ active: ignoreContext }"
          @click="store.toggleContextCutoff()"
          :title="ignoreContext ? '取消清除上下文' : '清除上下文（从此处开始新话题）'"
        >
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
            <path d="M3 12h18M3 6h18M3 18h18"/>
            <line x1="18" y1="3" x2="6" y2="21" stroke-width="1.5"/>
          </svg>
        </button>
        <!-- 联网搜索按钮 -->
        <button
          class="btn-web-search"
          :class="{ active: webSearch }"
          @click="webSearch = !webSearch"
          :title="webSearch ? '关闭联网搜索' : '开启联网搜索'"
        >
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
            <circle cx="12" cy="12" r="10"/>
            <line x1="2" y1="12" x2="22" y2="12"/>
            <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/>
          </svg>
        </button>
        <!-- Skills 选择按钮 -->
        <button class="btn-skill-picker" :class="{ active: showSkillPicker || selectedSkillIDs.length > 0 }" @click="toggleSkillPicker" title="选择技能">
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"/></svg>
          <span v-if="selectedSkillIDs.length > 0" class="skill-count">{{ selectedSkillIDs.length }}</span>
        </button>
        <!-- 模型选择按钮 -->
        <button class="btn-model" :class="{ 'btn-model--active': showModelPicker }" @click="toggleModelPicker" title="切换模型">
          <span>{{ modelLabel }}</span>
          <svg width="9" height="9" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" style="flex-shrink:0">
            <polyline points="6 9 12 15 18 9"/>
          </svg>
        </button>
        <!-- 停止/发送 -->
        <button v-if="store.streaming" class="btn-stop" @click="stop" title="停止">
          <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor"><rect x="3" y="3" width="10" height="10" rx="1.5"/></svg>
        </button>
        <button v-else class="btn-send" :disabled="!input.trim()" @click="send">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor"><path d="M2.01 21 23 12 2.01 3 2 10l15 2-15 2z"/></svg>
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.input-area {
  padding: 0 var(--space-6) var(--space-5);
  position: relative;
}

.popup-panel {
  position: absolute;
  bottom: calc(100% - var(--space-5));
  left: var(--space-6);
  right: var(--space-6);
  z-index: 100;
  margin-bottom: var(--space-2);
}

.model-picker {
  position: absolute;
  bottom: calc(100% - var(--space-5));
  right: var(--space-6);
  z-index: 100;
  margin-bottom: var(--space-2);
  width: 200px;
  background: var(--color-paper);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-lg);
  padding: var(--space-3);
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  max-height: 320px;
  overflow-y: auto;
}

.btn-attach {
  display: flex; align-items: center; justify-content: center;
  width: 28px; height: 28px;
  border: 1px solid var(--color-border); border-radius: var(--radius-md);
  background: transparent; cursor: pointer;
  color: var(--color-text-3);
  transition: border-color var(--duration-fast) var(--ease-out), color var(--duration-fast) var(--ease-out), background var(--duration-fast) var(--ease-out);
}
.btn-attach:hover { border-color: var(--color-accent); color: var(--color-accent); background: var(--color-accent-soft); }
.btn-attach.active { border-color: var(--color-accent); color: var(--color-accent); background: var(--color-accent-soft); }

.btn-attach-inner {
  position: absolute;
  left: var(--space-3);
  bottom: var(--space-3);
  z-index: 1;
  display: flex; align-items: center; justify-content: center;
  width: 26px; height: 26px;
  border: none;
  border-radius: var(--radius-md);
  background: transparent;
  cursor: pointer;
  color: var(--color-text-3);
  transition: color var(--duration-fast) var(--ease-out), background var(--duration-fast) var(--ease-out);
}
.btn-attach-inner:hover { color: var(--color-accent); background: var(--color-accent-soft); }
.btn-attach-inner.active { color: var(--color-accent); }

.attachment-preview {
  display: flex; flex-wrap: wrap; gap: var(--space-2);
  padding: var(--space-2) var(--space-4) 0;
}
.attachment-chip {
  display: flex; align-items: center; gap: 5px;
  padding: 3px var(--space-2);
  background: var(--color-paper-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-full);
  font-size: var(--text-xs);
  color: var(--color-text-2);
  max-width: 200px;
}
.attachment-name {
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  max-width: 140px;
}
.attachment-remove {
  flex-shrink: 0; display: flex; align-items: center; justify-content: center;
  width: 14px; height: 14px; border: none; background: transparent;
  color: var(--color-text-3); cursor: pointer; padding: 0; border-radius: 50%;
}
.attachment-remove:hover { color: var(--color-danger); background: var(--color-paper-4); }

.btn-ignore-ctx {
  display: flex; align-items: center; justify-content: center;
  width: 28px; height: 28px;
  border: 1px solid var(--color-border); border-radius: var(--radius-md);
  background: transparent; cursor: pointer;
  color: var(--color-text-3);
  transition: border-color var(--duration-fast) var(--ease-out), color var(--duration-fast) var(--ease-out), background var(--duration-fast) var(--ease-out);
}
.btn-ignore-ctx:hover { border-color: var(--color-accent); color: var(--color-accent); background: var(--color-accent-soft); }
.btn-ignore-ctx.active { border-color: oklch(0.65 0.15 25); color: oklch(0.65 0.15 25); background: oklch(0.96 0.03 25); }

.btn-web-search {
  display: flex; align-items: center; justify-content: center;
  width: 28px; height: 28px;
  border: 1px solid var(--color-border); border-radius: var(--radius-md);
  background: transparent; cursor: pointer;
  color: var(--color-text-3);
  transition: border-color var(--duration-fast) var(--ease-out), color var(--duration-fast) var(--ease-out), background var(--duration-fast) var(--ease-out);
}
.btn-web-search:hover { border-color: var(--color-accent); color: var(--color-accent); background: var(--color-accent-soft); }
.btn-web-search.active { border-color: var(--color-accent); color: var(--color-accent); background: var(--color-accent-soft); }

.btn-skill-picker {
  display: flex; align-items: center; gap: 3px;
  height: 28px; padding: 0 var(--space-2);
  border: 1px solid var(--color-border); border-radius: var(--radius-md);
  background: transparent; cursor: pointer;
  color: var(--color-text-3);
  transition: border-color var(--duration-fast) var(--ease-out), color var(--duration-fast) var(--ease-out), background var(--duration-fast) var(--ease-out);
}
.btn-skill-picker:hover, .btn-skill-picker.active { border-color: var(--color-accent); color: var(--color-accent); background: var(--color-accent-soft); }
.skill-count { font-size: 10px; font-weight: 700; background: var(--color-accent); color: #fff; border-radius: var(--radius-full); padding: 0 4px; min-width: 14px; text-align: center; }

.skill-picker {
  position: absolute;
  bottom: calc(100% - var(--space-5));
  right: calc(var(--space-6) + 160px);
  z-index: 100;
  margin-bottom: var(--space-2);
  width: 220px;
  background: var(--color-paper);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-lg);
  overflow: hidden;
}
.skill-picker-header { padding: var(--space-2) var(--space-3); font-size: var(--text-xs); font-weight: 600; color: var(--color-text-2); background: var(--color-paper-2); border-bottom: 1px solid var(--color-border); display: flex; align-items: baseline; gap: var(--space-2); }
.skill-picker-hint { font-weight: 400; color: var(--color-text-3); }
.skill-picker-list { max-height: 240px; overflow-y: auto; padding: var(--space-1); display: flex; flex-direction: column; gap: 2px; }
.skill-picker-item { display: flex; align-items: center; gap: var(--space-2); padding: var(--space-2) var(--space-2); border: none; border-radius: var(--radius-sm); background: transparent; cursor: pointer; text-align: left; width: 100%; transition: background var(--duration-fast) var(--ease-out); }
.skill-picker-item:hover { background: var(--color-paper-3); }
.skill-picker-item.active { background: var(--color-accent-soft); }
.skill-picker-check { width: 16px; height: 16px; border: 1px solid var(--color-border); border-radius: var(--radius-sm); display: flex; align-items: center; justify-content: center; flex-shrink: 0; color: var(--color-accent); }
.skill-picker-item.active .skill-picker-check { background: var(--color-accent); border-color: var(--color-accent); color: #fff; }
.skill-picker-info { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
.skill-picker-name { font-size: var(--text-xs); font-weight: 500; color: var(--color-text); }
.skill-picker-desc { font-size: 10px; color: var(--color-text-3); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.skill-picker-footer { padding: var(--space-2) var(--space-3); border-top: 1px solid var(--color-border); }
.skill-clear-btn { font-size: var(--text-xs); color: var(--color-text-3); background: none; border: none; cursor: pointer; padding: 0; }
.skill-clear-btn:hover { color: var(--color-danger); }

.model-empty {
  font-size: var(--text-xs);
  color: var(--color-text-3);
  padding: var(--space-2);
  text-align: center;
  line-height: var(--leading-relaxed);
}

.provider-group { display: flex; flex-direction: column; gap: var(--space-1); }

.provider-label {
  font-size: var(--text-xs);
  font-weight: 600;
  color: var(--color-text-3);
  padding: 0 var(--space-2);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.model-option {
  text-align: left;
  padding: var(--space-1) var(--space-2);
  border: none;
  border-radius: var(--radius-md);
  background: transparent;
  font-size: var(--text-sm);
  font-family: var(--font-mono);
  color: var(--color-text-2);
  cursor: pointer;
  transition: background var(--duration-fast) var(--ease-out), color var(--duration-fast) var(--ease-out);
}
.model-option:hover { background: var(--color-paper-3); color: var(--color-text); }
.model-option.active { background: var(--color-accent-soft); color: var(--color-accent); font-weight: 500; }

.slide-up-enter-active,
.slide-up-leave-active {
  transition: opacity var(--duration-normal) var(--ease-out), transform var(--duration-normal) var(--ease-out);
}
.slide-up-enter-from,
.slide-up-leave-to { opacity: 0; transform: translateY(8px); }

.input-meta {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  margin-bottom: var(--space-2);
  min-height: 20px;
}

.skill-badge {
  display: inline-flex; align-items: center;
  padding: 2px var(--space-2);
  background: var(--color-accent-soft);
  border-radius: var(--radius-full);
  font-size: var(--text-xs);
  color: var(--color-accent-2);
  font-weight: 500;
}

.input-inner {
  position: relative;
  display: flex;
  align-items: flex-end;
  gap: var(--space-2);
  background: var(--color-paper-2);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-xl);
  padding: var(--space-2) var(--space-2) var(--space-2) var(--space-2);
  transition: border-color var(--duration-fast) var(--ease-out);
}

.input-inner:focus-within {
  border-color: var(--color-accent);
}

textarea {
  flex: 1;
  border: none;
  background: transparent;
  resize: none;
  font-family: var(--font-body);
  font-size: var(--text-base);
  line-height: var(--leading-relaxed);
  color: var(--color-text);
  outline: none;
  max-height: 200px;
  overflow-y: auto;
  padding: var(--space-1) 0;
}

textarea.has-attach-btn {
  padding-left: 32px;
}
.input-inner:focus-within {
  border-color: var(--color-accent);
  box-shadow: 0 0 0 3px var(--color-accent-soft);
}

textarea {
  flex: 1;
  border: none; resize: none;
  padding: var(--space-2) var(--space-3);
  font-family: var(--font-body);
  font-size: var(--text-sm);
  line-height: var(--leading-normal);
  color: var(--color-text);
  background: transparent;
  outline: none;
  max-height: 200px;
}
textarea::placeholder { color: var(--color-text-3); }

.input-actions { display: flex; align-items: center; gap: var(--space-1); }

.btn-icon {
  width: 32px; height: 32px;
  display: flex; align-items: center; justify-content: center;
  border: none; border-radius: var(--radius-md); cursor: pointer;
  background: transparent; color: var(--color-text-3);
  transition: background var(--duration-fast) var(--ease-out), color var(--duration-fast) var(--ease-out);
  flex-shrink: 0;
}
.btn-icon:hover { background: var(--color-paper-3); color: var(--color-text-2); }
.btn-icon--active { background: var(--color-accent-soft); color: var(--color-accent); }
.btn-icon--active:hover { background: var(--color-accent-soft); color: var(--color-accent-2); }

.btn-model {
  display: flex; align-items: center; gap: 3px;
  height: 28px; padding: 0 var(--space-2);
  border: 1px solid var(--color-border); border-radius: var(--radius-md);
  background: transparent; cursor: pointer;
  font-size: 11px; font-family: var(--font-mono);
  color: var(--color-text-3); white-space: nowrap;
  max-width: 140px; min-width: 0;
  overflow: hidden;
  transition: border-color var(--duration-fast) var(--ease-out), color var(--duration-fast) var(--ease-out), background var(--duration-fast) var(--ease-out);
}
.btn-model span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.btn-model:hover { border-color: var(--color-accent); color: var(--color-accent); background: var(--color-accent-soft); }
.btn-model--active { border-color: var(--color-accent); color: var(--color-accent); background: var(--color-accent-soft); }

.btn-send {
  width: 36px; height: 36px;
  display: flex; align-items: center; justify-content: center;
  border: none; border-radius: var(--radius-md); cursor: pointer;
  background: var(--color-accent); color: #fff;
  transition: background var(--duration-fast) var(--ease-out);
  flex-shrink: 0;
}
.btn-send:hover:not(:disabled) { background: var(--color-accent-2); }
.btn-send:disabled { opacity: 0.35; cursor: not-allowed; }

.btn-stop {
  width: 36px; height: 36px;
  display: flex; align-items: center; justify-content: center;
  border: none; border-radius: var(--radius-md); cursor: pointer;
  background: var(--color-paper-4); color: var(--color-text);
  transition: background var(--duration-fast) var(--ease-out);
  flex-shrink: 0;
}
.btn-stop:hover { background: var(--color-danger); color: #fff; }
</style>
