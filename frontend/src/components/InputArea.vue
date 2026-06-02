<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { useChatStore } from '../stores/chat'
import { StreamChat, CancelStream, PickAttachments } from '../../wailsjs/go/handler/ChatHandler'
import { Get } from '../../wailsjs/go/handler/SettingsHandler'
import { SetModel } from '../../wailsjs/go/handler/ConversationHandler'
import { ListEnabledModels, ListProviders } from '../../wailsjs/go/handler/ProviderHandler'
import { ListSkills } from '../../wailsjs/go/handler/SkillHandler'
import { List as ListMCPServers } from '../../wailsjs/go/handler/MCPHandler'
import { ListKnowledgeBases } from '../../wailsjs/go/handler/KnowledgeHandler'
import { handler as handlerModels, type storage } from '../../wailsjs/go/models'

const store = useChatStore()
const input = ref('')
const showModelPicker = ref(false)
const showSkillPicker = ref(false)
const showMCPPicker = ref(false)
const selectedSkillIDs = ref<string[]>([])
const selectedMCPIDs = ref<string[]>([])
const webSearch = ref(false)
const ignoreContext = computed(() => store.contextCutoffId !== null)

// 对话模式
type ChatMode = 'chat' | 'knowledge'
const chatMode = ref<ChatMode>('chat')
const showModePicker = ref(false)
const availableKBs = ref<storage.KnowledgeBase[]>([])
const selectedKBID = ref('')
const showKBPicker = ref(false)

async function loadKBs() {
  availableKBs.value = await ListKnowledgeBases().catch(() => [])
}

// 模式/知识库 picker 的 body 绝对定位样式
const modPickerStyle = ref<Record<string, string>>({})
const kbPickerStyle = ref<Record<string, string>>({})

function calcPickerPos(btnClass: string): Record<string, string> {
  const btn = document.querySelector(btnClass) as HTMLElement
  if (!btn) return {}
  const r = btn.getBoundingClientRect()
  return {
    position: 'fixed',
    left: r.left + 'px',
    bottom: (window.innerHeight - r.top + 6) + 'px',
    zIndex: '9999',
  }
}

function toggleModePicker() {
  showModePicker.value = !showModePicker.value
  showKBPicker.value = false
  showModelPicker.value = false
  showSkillPicker.value = false
  showMCPPicker.value = false
  if (showModePicker.value) {
    nextTick(() => { modPickerStyle.value = calcPickerPos('.btn-mode') })
  }
}

function toggleKBPicker() {
  showKBPicker.value = !showKBPicker.value
  showModePicker.value = false
  showModelPicker.value = false
  showSkillPicker.value = false
  showMCPPicker.value = false
  if (showKBPicker.value) {
    loadKBs()
    nextTick(() => { kbPickerStyle.value = calcPickerPos('.btn-kb') })
  }
}

function selectMode(mode: ChatMode) {
  chatMode.value = mode
  showModePicker.value = false
  if (mode === 'knowledge') {
    loadKBs()
    if (!selectedKBID.value && availableKBs.value.length > 0) {
      selectedKBID.value = availableKBs.value[0].id
    }
  }
}

const selectedKBName = computed(() => {
  const kb = availableKBs.value.find(k => k.id === selectedKBID.value)
  return kb?.name ?? '选择知识库'
})

interface Attachment { name: string; mime_type: string; data: string }
const attachments = ref<Attachment[]>([])

async function pickAttachments() {
  const picked = await PickAttachments().catch(() => null)
  if (!picked) return
  attachments.value.push(...picked)
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

const availableMCPs = ref<storage.MCPServer[]>([])

async function loadMCPs() {
  availableMCPs.value = await ListMCPServers().catch(() => [])
}

function toggleMCPID(id: string) {
  const idx = selectedMCPIDs.value.indexOf(id)
  if (idx >= 0) selectedMCPIDs.value.splice(idx, 1)
  else selectedMCPIDs.value.push(id)
}

function toggleMCPPicker() {
  showMCPPicker.value = !showMCPPicker.value
  showModelPicker.value = false
  showSkillPicker.value = false
  if (showMCPPicker.value) loadMCPs()
}

function toggleSkillPicker() {
  showSkillPicker.value = !showSkillPicker.value
  showMCPPicker.value = false
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
  // currentProvider 存的是 provider.id（UUID）
  const p = providerMap.value[currentProvider.value]
  return p ? `${p.name} · ${currentModel.value}` : currentModel.value
})

const activeSkillLabel = computed(() => {
  const prompt = (currentConv.value as any)?.system_prompt || ''
  return SKILL_NAMES[prompt] || null
})

async function selectModel(provider: storage.LLMProvider, modelName: string) {
  if (!store.currentConvId) return
  showModelPicker.value = false
  const conv = store.conversations.find(c => c.id === store.currentConvId)
  // 存 provider.id（UUID），不存 type，避免多个同类 provider 无法区分
  if (conv) { conv.provider = provider.id; conv.model = modelName }
  await SetModel(store.currentConvId, provider.id, modelName).catch(console.error)
}

function toggleModelPicker() {
  showModelPicker.value = !showModelPicker.value
  showSkillPicker.value = false
  showMCPPicker.value = false
  if (showModelPicker.value) loadEnabledModels()
}

function onClickOutside(e: MouseEvent) {
  const el = document.querySelector('.input-area')
  if (el && !el.contains(e.target as Node)) {
    // 点击在 input-area 外，但 Teleport 的 picker 在 body 里，需要单独检查
    const modePicker = document.querySelector('.mode-picker')
    const kbPicker = document.querySelector('.kb-picker')
    if (!modePicker?.contains(e.target as Node)) showModePicker.value = false
    if (!kbPicker?.contains(e.target as Node)) showKBPicker.value = false
    showModelPicker.value = false
    showSkillPicker.value = false
    showMCPPicker.value = false
    return
  }
  // 点击在 input-area 内部
  const skillPicker = document.querySelector('.skill-picker')
  const skillBtn = document.querySelector('.btn-skill-picker')
  if (showSkillPicker.value && skillPicker && !skillPicker.contains(e.target as Node) && !skillBtn?.contains(e.target as Node)) {
    showSkillPicker.value = false
  }
  const modelPicker = document.querySelector('.model-picker')
  const modelBtn = document.querySelector('.btn-model')
  if (showModelPicker.value && modelPicker && !modelPicker.contains(e.target as Node) && !modelBtn?.contains(e.target as Node)) {
    showModelPicker.value = false
  }
  const mcpPicker = document.querySelector('.mcp-picker')
  const mcpBtn = document.querySelector('.btn-mcp-picker')
  if (showMCPPicker.value && mcpPicker && !mcpPicker.contains(e.target as Node) && !mcpBtn?.contains(e.target as Node)) {
    showMCPPicker.value = false
  }
  // mode-picker 和 kb-picker 在 body（Teleport），点击按钮本身不关闭（由 toggle 处理）
  const modePicker = document.querySelector('.mode-picker')
  const modeBtn = document.querySelector('.btn-mode')
  if (showModePicker.value && modePicker && !modePicker.contains(e.target as Node) && !modeBtn?.contains(e.target as Node)) {
    showModePicker.value = false
  }
  const kbPicker = document.querySelector('.kb-picker')
  const kbBtn = document.querySelector('.btn-kb')
  if (showKBPicker.value && kbPicker && !kbPicker.contains(e.target as Node) && !kbBtn?.contains(e.target as Node)) {
    showKBPicker.value = false
  }
}

async function send() {
  const text = input.value.trim()
  if (!text || !store.currentConvId) return

  // currentProvider 存的是 provider.id（UUID），从 providerMap 取完整 provider 对象
  const providerID = currentProvider.value
  const providerObj = providerMap.value[providerID]

  if (!providerObj) {
    store.resetStream()
    store.appendStream('⚠️ 请先在输入框右下角选择模型供应商')
    return
  }

  if (providerObj.type !== 'ollama' && !providerObj.api_key) {
    store.resetStream()
    store.appendStream(`⚠️ 请先在「设置」中配置 ${providerObj.name} 的 API Key，然后重试。`)
    return
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
  showModelPicker.value = false
  showSkillPicker.value = false
  showMCPPicker.value = false
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
      provider: providerID,    // provider.id（UUID），后端按 id 查 llm_providers 表
      model: currentModel.value || 'gpt-4o',
      agent_id: store.activeAgentId ?? '',
      mcp_server_ids: selectedMCPIDs.value,
      skill_ids: selectedSkillIDs.value,
      web_search: webSearch.value,
      ignore_context: false,
      context_cutoff_id: store.contextCutoffId ?? '',
      attachments: sentAttachments,
      mode: chatMode.value,
      knowledge_base_id: chatMode.value === 'knowledge' ? selectedKBID.value : '',
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
            :class="{ active: currentProvider === g.provider.id && currentModel === m.name }"
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

    <!-- MCP 选择面板 -->
    <transition name="slide-up">
      <div v-if="showMCPPicker" class="mcp-picker">
        <div class="mcp-picker-header">
          <span class="mcp-picker-title">选择 MCP 工具</span>
          <span class="mcp-picker-hint">多选，发送时启用</span>
        </div>
        <div v-if="availableMCPs.filter(m => m.enabled).length === 0" class="model-empty">
          请先在「设置 → MCP」中添加并启用 MCP 服务器
        </div>
        <div v-else class="mcp-picker-list">
          <button
            v-for="m in availableMCPs.filter(s => s.enabled)"
            :key="m.id"
            class="mcp-picker-item"
            :class="{ active: selectedMCPIDs.includes(m.id) }"
            @click="toggleMCPID(m.id)"
          >
            <span class="mcp-picker-type">{{ m.type.toUpperCase() }}</span>
            <div class="mcp-picker-info">
              <span class="mcp-picker-name">{{ m.name }}</span>
              <span class="mcp-picker-addr">{{ m.type === 'sse' ? m.url : m.command }}</span>
            </div>
            <span v-if="selectedMCPIDs.includes(m.id)" class="mcp-picker-check">
              <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><polyline points="20 6 9 17 4 12"/></svg>
            </span>
          </button>
        </div>
        <div v-if="selectedMCPIDs.length > 0" class="mcp-picker-footer">
          <button class="skill-clear-btn" @click="selectedMCPIDs = []">清除选择</button>
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
      <!-- 工具栏（上行） -->
      <div class="input-actions">
        <!-- 模式选择器 -->
        <div class="mode-selector-wrap">
          <button class="btn-mode" :class="{ 'btn-mode--active': showModePicker || chatMode !== 'chat' }" @click.stop="toggleModePicker" title="切换对话模式">
            <span v-if="chatMode === 'chat'">
              <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>
              问答
            </span>
            <span v-else-if="chatMode === 'knowledge'">
              <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20"/><path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z"/></svg>
              知识
            </span>
            <svg width="8" height="8" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><polyline points="6 9 12 15 18 9"/></svg>
          </button>
        </div>
        <!-- 知识库选择器（仅 knowledge 模式） -->
        <div v-if="chatMode === 'knowledge'" class="kb-selector-wrap">
          <button class="btn-kb" :class="{ 'btn-kb--active': showKBPicker }" @click.stop="toggleKBPicker">
            <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20"/><path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z"/></svg>
            <span>{{ selectedKBName }}</span>
            <svg width="8" height="8" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><polyline points="6 9 12 15 18 9"/></svg>
          </button>
        </div>
        <!-- 分隔线 -->
        <div class="actions-sep"></div>
        <!-- 附件按钮 -->
        <button class="btn-tool" @click="pickAttachments()" :title="'上传文件或图片'" :class="{ active: attachments.length > 0 }">
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M21.44 11.05l-9.19 9.19a6 6 0 0 1-8.49-8.49l9.19-9.19a4 4 0 0 1 5.66 5.66l-9.2 9.19a2 2 0 0 1-2.83-2.83l8.49-8.48"/></svg>
        </button>
        <!-- 忽略上下文 -->
        <button class="btn-tool" :class="{ active: ignoreContext }" @click="store.toggleContextCutoff()" :title="ignoreContext ? '取消清除上下文' : '清除上下文'">
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M3 12h18M3 6h18M3 18h18"/><line x1="18" y1="3" x2="6" y2="21" stroke-width="1.5"/></svg>
        </button>
        <!-- 联网搜索 -->
        <button class="btn-tool" :class="{ active: webSearch }" @click="webSearch = !webSearch" :title="webSearch ? '关闭联网搜索' : '开启联网搜索'">
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/></svg>
        </button>
        <!-- Skills -->
        <button class="btn-tool btn-skill-picker" :class="{ active: showSkillPicker || selectedSkillIDs.length > 0 }" @click="toggleSkillPicker" title="选择技能">
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"/></svg>
          <span v-if="selectedSkillIDs.length > 0" class="skill-count">{{ selectedSkillIDs.length }}</span>
        </button>
        <!-- MCP -->
        <button class="btn-tool btn-mcp-picker" :class="{ active: showMCPPicker || selectedMCPIDs.length > 0 }" @click="toggleMCPPicker" title="选择 MCP 工具">
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M12 2L2 7l10 5 10-5-10-5z"/><path d="M2 17l10 5 10-5"/><path d="M2 12l10 5 10-5"/></svg>
          <span v-if="selectedMCPIDs.length > 0" class="mcp-count">{{ selectedMCPIDs.length }}</span>
        </button>
        <!-- 模型选择 -->
        <button class="btn-model" :class="{ 'btn-model--active': showModelPicker }" @click="toggleModelPicker" title="切换模型">
          <span>{{ modelLabel }}</span>
          <svg width="9" height="9" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" style="flex-shrink:0"><polyline points="6 9 12 15 18 9"/></svg>
        </button>
        <!-- 停止/发送 -->
        <button v-if="store.streaming" class="btn-stop" @click="stop" title="停止">
          <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor"><rect x="3" y="3" width="10" height="10" rx="1.5"/></svg>
        </button>
        <button v-else class="btn-send" :disabled="!input.trim()" @click="send">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor"><path d="M2.01 21 23 12 2.01 3 2 10l15 2-15 2z"/></svg>
        </button>
      </div>
      <!-- textarea（下行） -->
      <textarea
        v-model="input"
        placeholder="输入消息..."
        :disabled="store.streaming"
        @keydown="onKeydown"
        rows="1"
      />
    </div>

    <!-- mode-picker 和 kb-picker 渲染在 input-area 层，避免被 input-inner 裁剪 -->
    <Teleport to="body">
      <div v-if="showModePicker" class="mode-picker" :style="modPickerStyle">
        <button class="mode-option" :class="{ active: chatMode === 'chat' }" @click="selectMode('chat')">
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>
          <div><div class="mode-name">问答</div><div class="mode-desc">默认对话模式</div></div>
        </button>
        <button class="mode-option" :class="{ active: chatMode === 'knowledge' }" @click="selectMode('knowledge')">
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20"/><path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z"/></svg>
          <div><div class="mode-name">知识</div><div class="mode-desc">挂载知识库问答</div></div>
        </button>
        <button class="mode-option mode-option--disabled" disabled>
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>
          <div><div class="mode-name">任务 <span class="mode-soon">即将推出</span></div><div class="mode-desc">自主 Agent 模式</div></div>
        </button>
      </div>
      <div v-if="showKBPicker" class="kb-picker" :style="kbPickerStyle">
        <div v-if="availableKBs.length === 0" class="kb-picker-empty">还没有知识库，请先在设置中创建</div>
        <button v-for="kb in availableKBs" :key="kb.id"
          class="kb-picker-item" :class="{ active: selectedKBID === kb.id }"
          @click="selectedKBID = kb.id; showKBPicker = false">
          <span class="kb-picker-name">{{ kb.name }}</span>
          <span class="kb-picker-count">{{ kb.doc_count }} 文档</span>
        </button>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.input-area {
  padding: 0 var(--space-6) var(--space-5);
  position: relative;
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

.btn-mcp-picker {
  display: flex; align-items: center; gap: 3px;
  height: 28px; padding: 0 var(--space-2);
  border: 1px solid var(--color-border); border-radius: var(--radius-md);
  background: transparent; cursor: pointer;
  color: var(--color-text-3);
  transition: border-color var(--duration-fast) var(--ease-out),
              color var(--duration-fast) var(--ease-out),
              background var(--duration-fast) var(--ease-out);
}
.btn-mcp-picker:hover, .btn-mcp-picker.active {
  border-color: var(--color-accent); color: var(--color-accent);
  background: var(--color-accent-soft);
}
.mcp-count {
  font-size: 10px; font-weight: 700;
  background: var(--color-accent); color: #fff;
  border-radius: var(--radius-full); padding: 0 4px;
  min-width: 14px; text-align: center;
}

.mcp-picker {
  position: absolute;
  bottom: calc(100% - var(--space-5));
  right: calc(var(--space-6) + 100px);
  z-index: 100;
  margin-bottom: var(--space-2);
  width: 240px;
  background: var(--color-paper);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-lg);
  overflow: hidden;
}
.mcp-picker-header {
  padding: var(--space-2) var(--space-3);
  font-size: var(--text-xs); font-weight: 600; color: var(--color-text-2);
  background: var(--color-paper-2); border-bottom: 1px solid var(--color-border);
  display: flex; align-items: baseline; gap: var(--space-2);
}
.mcp-picker-hint { font-weight: 400; color: var(--color-text-3); }
.mcp-picker-list {
  max-height: 240px; overflow-y: auto;
  padding: var(--space-1); display: flex; flex-direction: column; gap: 2px;
}
.mcp-picker-item {
  display: flex; align-items: center; gap: var(--space-2);
  padding: var(--space-2); border: none; border-radius: var(--radius-sm);
  background: transparent; cursor: pointer; text-align: left; width: 100%;
  transition: background var(--duration-fast) var(--ease-out);
}
.mcp-picker-item:hover { background: var(--color-paper-3); }
.mcp-picker-item.active { background: var(--color-accent-soft); }
.mcp-picker-type {
  font-size: 10px; font-weight: 700; padding: 1px 5px;
  background: var(--color-accent-soft); color: var(--color-accent);
  border-radius: var(--radius-sm); flex-shrink: 0;
}
.mcp-picker-info { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
.mcp-picker-name { font-size: var(--text-xs); font-weight: 500; color: var(--color-text); }
.mcp-picker-addr {
  font-size: 10px; color: var(--color-text-3);
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
}
.mcp-picker-check {
  width: 16px; height: 16px; border: 1px solid var(--color-border);
  border-radius: var(--radius-sm); display: flex; align-items: center;
  justify-content: center; flex-shrink: 0; color: var(--color-accent);
}
.mcp-picker-item.active .mcp-picker-check {
  background: var(--color-accent); border-color: var(--color-accent); color: #fff;
}
.mcp-picker-footer { padding: var(--space-2) var(--space-3); border-top: 1px solid var(--color-border); }

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
  display: flex;
  flex-direction: column;
  background: var(--color-paper-2);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-xl);
  padding: var(--space-2) var(--space-3);
  transition: border-color var(--duration-fast) var(--ease-out);
}
.input-inner:focus-within {
  border-color: var(--color-accent);
  box-shadow: 0 0 0 3px var(--color-accent-soft);
}

.input-actions {
  display: flex;
  align-items: center;
  gap: 2px;
  padding-bottom: var(--space-1);
  border-bottom: 1px solid var(--color-border);
  margin-bottom: var(--space-1);
  flex-wrap: nowrap;
  overflow: hidden;
}

textarea {
  width: 100%;
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
textarea::placeholder { color: var(--color-text-3); }

.actions-sep {
  flex: 1;
  min-width: var(--space-1);
}

/* 通用工具按钮 */
.btn-tool {
  display: flex; align-items: center; justify-content: center;
  width: 28px; height: 28px; flex-shrink: 0;
  border: none; border-radius: var(--radius-md); cursor: pointer;
  background: transparent; color: var(--color-text-3);
  transition: background var(--duration-fast), color var(--duration-fast);
  position: relative;
}
.btn-tool:hover { background: var(--color-paper-3); color: var(--color-text-2); }
.btn-tool.active { color: var(--color-accent); background: var(--color-accent-soft); }

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
  padding-left: 0;
}

.btn-icon {
  width: 28px; height: 28px;
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

/* 模式选择器 */
.mode-selector-wrap { position: relative; }
.btn-mode { display: flex; align-items: center; gap: 3px; padding: 3px var(--space-2); border: 1px solid var(--color-border); border-radius: var(--radius-full); background: transparent; color: var(--color-text-2); font-size: 11px; font-family: inherit; cursor: pointer; transition: all var(--duration-fast); white-space: nowrap; height: 26px; }
.btn-mode span { display: flex; align-items: center; gap: 3px; }
.btn-mode:hover, .btn-mode--active { border-color: var(--color-accent); color: var(--color-accent); background: var(--color-accent-soft); }
.mode-picker { background: var(--color-paper); border: 1px solid var(--color-border); border-radius: var(--radius-md); box-shadow: 0 -4px 20px rgba(0,0,0,.12); min-width: 180px; overflow: hidden; }
.mode-option { display: flex; align-items: flex-start; gap: var(--space-2); width: 100%; padding: var(--space-2) var(--space-3); border: none; background: transparent; color: var(--color-text); font-size: var(--text-sm); font-family: inherit; cursor: pointer; text-align: left; }
.mode-option:hover:not(:disabled) { background: var(--color-paper-2); }
.mode-option.active { color: var(--color-accent); }
.mode-option--disabled { opacity: 0.45; cursor: not-allowed; }
.mode-name { font-weight: 500; font-size: 12px; display: flex; align-items: center; gap: 4px; }
.mode-desc { font-size: 11px; color: var(--color-text-3); margin-top: 1px; }
.mode-soon { font-size: 10px; background: var(--color-paper-3); color: var(--color-text-3); padding: 1px 5px; border-radius: var(--radius-full); }

/* 知识库选择器 */
.kb-selector-wrap { position: relative; }
.btn-kb { display: flex; align-items: center; gap: 3px; padding: 3px var(--space-2); border: 1px solid var(--color-accent); border-radius: var(--radius-full); background: var(--color-accent-soft); color: var(--color-accent); font-size: 11px; font-family: inherit; cursor: pointer; max-width: 140px; height: 26px; }
.btn-kb span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.btn-kb:hover, .btn-kb--active { background: var(--color-accent); color: #fff; }
.kb-picker { background: var(--color-paper); border: 1px solid var(--color-border); border-radius: var(--radius-md); box-shadow: 0 -4px 20px rgba(0,0,0,.12); min-width: 200px; max-height: 240px; overflow-y: auto; }
.kb-picker-empty { padding: var(--space-3) var(--space-4); font-size: var(--text-xs); color: var(--color-text-3); }
.kb-picker-item { display: flex; align-items: center; justify-content: space-between; width: 100%; padding: var(--space-2) var(--space-3); border: none; background: transparent; color: var(--color-text); font-size: var(--text-sm); font-family: inherit; cursor: pointer; text-align: left; }
.kb-picker-item:hover { background: var(--color-paper-2); }
.kb-picker-item.active { color: var(--color-accent); background: var(--color-accent-soft); }
.kb-picker-name { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.kb-picker-count { font-size: 11px; color: var(--color-text-3); flex-shrink: 0; margin-left: var(--space-2); }
</style>
