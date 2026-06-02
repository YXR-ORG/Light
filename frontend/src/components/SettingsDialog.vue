<script setup lang="ts">
import { ref, computed, onMounted, watch, nextTick } from 'vue'
import { useSettingsStore } from '../stores/settings'
import MCPConfig from './MCPConfig.vue'
import KnowledgeConfig from './KnowledgeConfig.vue'
import {
  ListProviders, SaveProvider, DeleteProvider, ToggleProvider,
  ListModels, AddModel, DeleteModel, TestConnection,
} from '../../wailsjs/go/handler/ProviderHandler'
import { ListAgents, SaveAgent, DeleteAgent } from '../../wailsjs/go/handler/AgentHandler'
import { ListSkills, SaveSkill, ToggleSkill, DeleteSkill, ImportSkillZip } from '../../wailsjs/go/handler/SkillHandler'
import { Get as GetSetting, Set as SetSetting } from '../../wailsjs/go/handler/SettingsHandler'
import { SaveConfig, GetConfig, Backup, ListBackups, Restore, DeleteBackup } from '../../wailsjs/go/handler/BackupHandler'
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
import AboutPanel from './AboutPanel.vue'
import type { storage, handler } from '../../wailsjs/go/models'

const settingsStore = useSettingsStore()

type MainTab = 'providers' | 'agents' | 'skills' | 'mcp' | 'knowledge' | 'general' | 'about'
const mainTab = ref<MainTab>('providers')

const PROVIDER_TYPES = [
  { value: 'openai',  label: 'OpenAI 兼容',  placeholder: { key: 'sk-...', url: 'https://api.openai.com/v1' } },
  { value: 'google',  label: 'Google',        placeholder: { key: 'AIza...', url: 'https://generativelanguage.googleapis.com/v1beta/openai/' } },
  { value: 'claude',  label: 'Claude',        placeholder: { key: 'sk-ant-...', url: 'https://api.anthropic.com' } },
  { value: 'ollama',  label: 'Ollama',        placeholder: { key: '（可留空）', url: 'http://localhost:11434' } },
]

const providers = ref<storage.LLMProvider[]>([])
const selectedProviderID = ref<string | null>(null)
const models = ref<storage.LLMModel[]>([])
const newModelName = ref('')
const saving = ref(false)
const testConnState = ref<'idle' | 'loading' | 'ok' | 'fail'>('idle')
const testConnMsg = ref('')

async function doTestConnection() {
  const p = editingProvider.value
    ? { ...editingProvider.value, name: form.value.name, type: form.value.type, api_key: form.value.api_key, base_url: form.value.base_url }
    : { id: '', name: form.value.name, type: form.value.type, api_key: form.value.api_key, base_url: form.value.base_url, enabled: true, created_at: '', updated_at: '' } as storage.LLMProvider
  testConnState.value = 'loading'
  testConnMsg.value = ''
  const errMsg = await TestConnection(p as any).catch((e: any) => String(e))
  if (!errMsg) {
    testConnState.value = 'ok'
    testConnMsg.value = '连接成功'
  } else {
    testConnState.value = 'fail'
    testConnMsg.value = errMsg
  }
  setTimeout(() => { testConnState.value = 'idle'; testConnMsg.value = '' }, 4000)
}
const showProviderForm = ref(false)
const editingProvider = ref<storage.LLMProvider | null>(null)
const form = ref({ name: '', type: 'openai', api_key: '', base_url: '' })

const selectedProvider = computed(() =>
  providers.value.find(p => p.id === selectedProviderID.value) ?? null
)
const typeInfo = computed(() =>
  PROVIDER_TYPES.find(t => t.value === form.value.type)
)

onMounted(loadProviders)

async function loadProviders() {
  providers.value = await ListProviders().catch(() => [])
  // 每次都确保有选中项
  if (providers.value.length > 0) {
    const stillExists = providers.value.find(p => p.id === selectedProviderID.value)
    if (!stillExists) selectedProviderID.value = providers.value[0].id
  } else {
    selectedProviderID.value = null
  }
}

watch(selectedProviderID, async (id) => {
  models.value = id ? await ListModels(id).catch(() => []) : []
}, { immediate: true })

const formModels = ref<string[]>([])
const formModelInput = ref('')

function openAddProvider() {
  editingProvider.value = null
  form.value = { name: '', type: 'openai', api_key: '', base_url: '' }
  formModels.value = []
  formModelInput.value = ''
  showProviderForm.value = true
}

function openEditProvider(p: storage.LLMProvider) {
  editingProvider.value = p
  form.value = { name: p.name, type: p.type, api_key: p.api_key, base_url: p.base_url }
  formModels.value = []
  formModelInput.value = ''
  showProviderForm.value = true
}

function addFormModel() {
  const name = formModelInput.value.trim()
  if (!name || formModels.value.includes(name)) return
  formModels.value.push(name)
  formModelInput.value = ''
}

function removeFormModel(name: string) {
  formModels.value = formModels.value.filter(m => m !== name)
}

function cancelProviderForm() {
  showProviderForm.value = false
  editingProvider.value = null
}

async function saveProviderForm() {
  if (!form.value.name.trim()) return
  saving.value = true
  try {
    const p = editingProvider.value
      ? { ...editingProvider.value, name: form.value.name, type: form.value.type, api_key: form.value.api_key, base_url: form.value.base_url }
      : { id: '', name: form.value.name, type: form.value.type, api_key: form.value.api_key, base_url: form.value.base_url, enabled: true, created_at: '', updated_at: '' } as storage.LLMProvider
    console.log('[saveProvider] sending:', JSON.stringify(p))
    const saved = await SaveProvider(p)
    console.log('[saveProvider] saved:', JSON.stringify(saved))
    for (const name of formModels.value) {
      await AddModel({ id: '', provider_id: saved.id, name, created_at: '' } as any).catch(console.error)
    }
    await loadProviders()
    selectedProviderID.value = saved.id
    showProviderForm.value = false
    editingProvider.value = null
    formModels.value = []
  } catch (e) {
    console.error('[saveProvider] error:', e)
  } finally {
    saving.value = false
  }
}

async function deleteProvider(id: string) {
  await DeleteProvider(id).catch(console.error)
  if (selectedProviderID.value === id) selectedProviderID.value = null
  await loadProviders()
}

async function toggleProvider(p: storage.LLMProvider) {
  await ToggleProvider(p.id, !p.enabled).catch(console.error)
  await loadProviders()
}

async function addModel() {
  const name = newModelName.value.trim()
  if (!name || !selectedProviderID.value) return
  await AddModel({ id: '', provider_id: selectedProviderID.value, name, created_at: '', convertValues: undefined } as any)
  newModelName.value = ''
  models.value = await ListModels(selectedProviderID.value).catch(() => [])
}

async function deleteModel(id: string) {
  await DeleteModel(id).catch(console.error)
  if (selectedProviderID.value)
    models.value = await ListModels(selectedProviderID.value).catch(() => [])
}

// ── Agents ──
const agents = ref<storage.Agent[]>([])
const showAgentForm = ref(false)
const showEmojiPicker = ref(false)

const AGENT_EMOJIS = [
  '🤖','🧠','💡','🔬','📝','🎯','🚀','⚡','🌟','💎',
  '🎨','🖥️','📊','🔧','🛠️','⚙️','🔍','📚','🗂️','📋',
  '💼','🏆','🎓','🌐','🔐','🧪','🎭','🎵','📸','🌈',
  '🦾','🧬','🤝','💬','📡','🛡️','⚗️','🎲','🧩','🔮',
  '🐉','🦅','🐺','🦊','🐼','🦁','🐯','🦋','🌺','🍀',
]
const editingAgent = ref<storage.Agent | null>(null)
const agentForm = ref({ name: '', icon: '🤖', description: '', system_prompt: '' })
const agentSaving = ref(false)

onMounted(loadAgents)

async function loadAgents() {
  agents.value = await ListAgents().catch(() => [])
}

function openAddAgent() {
  editingAgent.value = null
  agentForm.value = { name: '', icon: '🤖', description: '', system_prompt: '' }
  showAgentForm.value = true
}

function openEditAgent(a: storage.Agent) {
  editingAgent.value = a
  agentForm.value = { name: a.name, icon: a.icon, description: a.description, system_prompt: a.system_prompt }
  showAgentForm.value = true
}

function cancelAgentForm() {
  showAgentForm.value = false
  editingAgent.value = null
}

async function saveAgentForm() {
  if (!agentForm.value.name.trim()) return
  agentSaving.value = true
  try {
    const a: storage.Agent = editingAgent.value
      ? { ...editingAgent.value, ...agentForm.value }
      : { id: '', name: agentForm.value.name, icon: agentForm.value.icon, description: agentForm.value.description, system_prompt: agentForm.value.system_prompt, sort_order: agents.value.length, builtin: false }
    await SaveAgent(a)
    await loadAgents()
    showAgentForm.value = false
    editingAgent.value = null
  } finally {
    agentSaving.value = false
  }
}

async function deleteAgent(id: string) {
  await DeleteAgent(id).catch(console.error)
  await loadAgents()
}

// ── Skills ──
const skills = ref<storage.Skill[]>([])
const skillImporting = ref(false)
const skillImportError = ref('')

onMounted(loadSkills)

async function loadSkills() {
  skills.value = await ListSkills().catch(() => [])
}

async function toggleSkill(id: string, enabled: boolean) {
  await ToggleSkill(id, enabled).catch(console.error)
  await loadSkills()
}

async function deleteSkill(id: string) {
  await DeleteSkill(id).catch(console.error)
  await loadSkills()
}

async function handleSkillZipUpload(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  skillImportError.value = ''
  skillImporting.value = true
  try {
    const buf = await file.arrayBuffer()
    const bytes = Array.from(new Uint8Array(buf))
    const imported = await ImportSkillZip(bytes, file.name)
    await loadSkills()
    skillImportError.value = `成功导入 ${imported.length} 个技能`
  } catch (e: any) {
    skillImportError.value = e?.message || String(e)
  } finally {
    skillImporting.value = false
    ;(e.target as HTMLInputElement).value = ''
  }
}

// ── 联网搜索 ──
const searchEngines = [
  { value: 'tavily',  label: 'Tavily' },
  { value: 'exa',     label: 'Exa' },
  { value: 'brave',   label: 'Brave' },
  { value: 'searxng', label: 'SearXNG' },
]
const searchEngine = ref('tavily')
const searchKeys = ref({ tavily: '', exa: '', brave: '', searxng: '' })
const searchKeySaved = ref(false)

async function selectEngine(engine: string) {
  searchEngine.value = engine
  await SetSetting('search_engine', engine)
}

async function saveSearchKey() {
  const engine = searchEngine.value
  const key = searchKeys.value[engine as keyof typeof searchKeys.value]
  const settingKey = engine === 'searxng' ? 'searxng_url' : `${engine}_api_key`
  await SetSetting(settingKey, key)
  searchKeySaved.value = true
  setTimeout(() => { searchKeySaved.value = false }, 2000)
}

async function saveTavilyKey() {
  searchEngine.value = 'tavily'
  await saveSearchKey()
}

// ── WebDAV 备份 ──
const webdavURL = ref('')
const webdavUsername = ref('')
const webdavPassword = ref('')
const webdavPath = ref('/Light/')
const webdavConfigSaved = ref(false)
const webdavBacking = ref(false)
const webdavBackupMsg = ref('')
const webdavBackups = ref<handler.BackupFile[]>([])
const webdavRestoring = ref('')
const webdavDeleting = ref('')
const webdavListMsg = ref('')
const webdavListLoading = ref(false)
const backupPage = ref(1)
const backupPageSize = 5
const backupPageCount = computed(() => Math.ceil(webdavBackups.value.length / backupPageSize))
const pagedBackups = computed(() =>
  webdavBackups.value.slice((backupPage.value - 1) * backupPageSize, backupPage.value * backupPageSize)
)
const showConfirm = ref(false)
const confirmMsg = ref('')
const confirmAction = ref<(() => void) | null>(null)
const confirmRef = ref<HTMLElement | null>(null)

function openConfirm(msg: string, action: () => void) {
  confirmMsg.value = msg
  confirmAction.value = action
  showConfirm.value = true
  nextTick(() => {
    confirmRef.value?.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
  })
}
function doConfirm() {
  showConfirm.value = false
  confirmAction.value?.()
}
function cancelConfirm() {
  showConfirm.value = false
  confirmAction.value = null
}

onMounted(async () => {
  const [engine, tavily, exa, brave, searxng, cfg] = await Promise.all([
    GetSetting('search_engine').catch(() => ''),
    GetSetting('tavily_api_key').catch(() => ''),
    GetSetting('exa_api_key').catch(() => ''),
    GetSetting('brave_api_key').catch(() => ''),
    GetSetting('searxng_url').catch(() => ''),
    GetConfig().catch(() => null),
  ])
  searchEngine.value = engine || 'tavily'
  searchKeys.value = { tavily, exa, brave, searxng }
  if (cfg) {
    webdavURL.value = cfg.url || ''
    webdavUsername.value = cfg.username || ''
    webdavPath.value = cfg.path || '/Light/'
  }
})

async function saveWebDAVConfig() {
  await SaveConfig(webdavURL.value, webdavUsername.value, webdavPassword.value, webdavPath.value)
  webdavConfigSaved.value = true
  setTimeout(() => { webdavConfigSaved.value = false }, 2000)
}

async function doBackup() {
  webdavBacking.value = true
  webdavBackupMsg.value = ''
  try {
    await Backup()
    webdavBackupMsg.value = '✓ 备份成功'
    await loadBackups()
  } catch (e: any) {
    webdavBackupMsg.value = '✗ ' + (e?.message || String(e))
  } finally {
    webdavBacking.value = false
  }
}

async function loadBackups() {
  webdavListLoading.value = true
  webdavListMsg.value = ''
  backupPage.value = 1
  try {
    webdavBackups.value = await ListBackups()
    if (webdavBackups.value.length === 0) webdavListMsg.value = '暂无备份文件'
  } catch (e: any) {
    webdavListMsg.value = '✗ 获取列表失败: ' + (e?.message || String(e))
    webdavBackups.value = []
  } finally {
    webdavListLoading.value = false
  }
}

async function doRestore(filename: string) {
  openConfirm(`确认从 ${filename} 恢复数据？\n当前所有数据将被覆盖，恢复完成后应用将自动重启。`, async () => {
    webdavRestoring.value = filename
    webdavBackupMsg.value = '恢复中，完成后将自动重启...'
    try {
      await Restore(filename)
      // 后端会自动重启，前端只需等待
    } catch (e: any) {
      webdavBackupMsg.value = '✗ 恢复失败: ' + (e?.message || String(e))
      webdavRestoring.value = ''
    }
  })
}

async function doDeleteBackup(filename: string) {
  openConfirm(`确认删除备份 ${filename}？此操作不可恢复。`, async () => {
    webdavDeleting.value = filename
    try {
      await DeleteBackup(filename)
      await loadBackups()
    } catch (e: any) {
      webdavBackupMsg.value = '✗ 删除失败: ' + (e?.message || String(e))
    } finally {
      webdavDeleting.value = ''
    }
  })
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / 1024 / 1024).toFixed(1) + ' MB'
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
        <div class="modal-body">
          <nav class="side-nav">
            <button class="nav-item" :class="{ active: mainTab === 'providers' }" @click="mainTab = 'providers'">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><circle cx="12" cy="12" r="3"/><path d="M12 2v3M12 19v3M4.22 4.22l2.12 2.12M17.66 17.66l2.12 2.12M2 12h3M19 12h3M4.22 19.78l2.12-2.12M17.66 6.34l2.12-2.12"/></svg>
              模型供应商
            </button>
            <button class="nav-item" :class="{ active: mainTab === 'general' }" @click="mainTab = 'general'">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>
              通用设置
            </button>
            <button class="nav-item" :class="{ active: mainTab === 'agents' }" @click="mainTab = 'agents'">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/></svg>
              智能体
            </button>
            <button class="nav-item" :class="{ active: mainTab === 'skills' }" @click="mainTab = 'skills'">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"/></svg>
              Skills 广场
            </button>
            <button class="nav-item" :class="{ active: mainTab === 'mcp' }" @click="mainTab = 'mcp'">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><rect x="2" y="3" width="20" height="14" rx="2"/><path d="M8 21h8M12 17v4"/></svg>
              MCP
            </button>
            <button class="nav-item" :class="{ active: mainTab === 'knowledge' }" @click="mainTab = 'knowledge'">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20"/><path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z"/></svg>
              知识库
            </button>
            <button class="nav-item" :class="{ active: mainTab === 'about' }" @click="mainTab = 'about'">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
              关于
            </button>
          </nav>
          <div class="panel">

            <!-- 通用设置 -->
            <div v-if="mainTab === 'general'" class="general-panel">
              <div class="setting-section">
                <div class="setting-section-title">联网搜索</div>
                <div class="setting-section-desc">开启联网搜索后，AI 可在对话中自动调用搜索工具获取最新信息。</div>

                <!-- 引擎选择 -->
                <div class="field" style="margin-top:var(--space-3)">
                  <label>搜索引擎</label>
                  <div class="engine-tabs">
                    <button v-for="e in searchEngines" :key="e.value"
                      class="engine-tab" :class="{ active: searchEngine === e.value }"
                      @click="selectEngine(e.value)">
                      <svg v-if="searchEngine === e.value" width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>
                      {{ e.label }}
                    </button>
                  </div>
                </div>

                <!-- Tavily -->
                <template v-if="searchEngine === 'tavily'">
                  <div class="field">
                    <label>Tavily API Key
                      <a href="#" class="setting-link" @click.prevent="BrowserOpenURL('https://app.tavily.com')">申请免费 Key →</a>
                    </label>
                    <div style="display:flex;gap:var(--space-2)">
                      <input v-model="searchKeys.tavily" type="password" placeholder="tvly-..." autocomplete="off" style="flex:1" />
                      <button class="btn btn-primary" @click="saveSearchKey" style="white-space:nowrap">
                        {{ searchKeySaved ? '已保存 ✓' : '保存' }}
                      </button>
                    </div>
                  </div>
                </template>

                <!-- Exa -->
                <template v-else-if="searchEngine === 'exa'">
                  <div class="field">
                    <label>Exa API Key
                      <a href="#" class="setting-link" @click.prevent="BrowserOpenURL('https://dashboard.exa.ai')">申请 Key →</a>
                    </label>
                    <div style="display:flex;gap:var(--space-2)">
                      <input v-model="searchKeys.exa" type="password" placeholder="exa-..." autocomplete="off" style="flex:1" />
                      <button class="btn btn-primary" @click="saveSearchKey" style="white-space:nowrap">
                        {{ searchKeySaved ? '已保存 ✓' : '保存' }}
                      </button>
                    </div>
                  </div>
                </template>

                <!-- Brave -->
                <template v-else-if="searchEngine === 'brave'">
                  <div class="field">
                    <label>Brave Search API Key
                      <a href="#" class="setting-link" @click.prevent="BrowserOpenURL('https://api.search.brave.com/app/keys')">申请 Key →</a>
                    </label>
                    <div style="display:flex;gap:var(--space-2)">
                      <input v-model="searchKeys.brave" type="password" placeholder="BSA..." autocomplete="off" style="flex:1" />
                      <button class="btn btn-primary" @click="saveSearchKey" style="white-space:nowrap">
                        {{ searchKeySaved ? '已保存 ✓' : '保存' }}
                      </button>
                    </div>
                  </div>
                </template>

                <!-- SearXNG -->
                <template v-else-if="searchEngine === 'searxng'">
                  <div class="field">
                    <label>SearXNG 实例地址
                      <a href="#" class="setting-link" @click.prevent="BrowserOpenURL('https://searx.space')">公共实例列表 →</a>
                    </label>
                    <div style="display:flex;gap:var(--space-2)">
                      <input v-model="searchKeys.searxng" type="text" placeholder="https://searx.be" autocomplete="off" style="flex:1" />
                      <button class="btn btn-primary" @click="saveSearchKey" style="white-space:nowrap">
                        {{ searchKeySaved ? '已保存 ✓' : '保存' }}
                      </button>
                    </div>
                    <div class="field-hint">无需 API Key，填写可用的 SearXNG 实例地址即可</div>
                  </div>
                </template>
              </div>

              <div class="setting-section">
                <div class="setting-section-title">数据备份（WebDAV）</div>
                <div class="setting-section-desc">备份内容包含全部聊天记录、API Key、模型配置、智能体、Skills 等所有本地数据。支持坚果云、Nextcloud、Alist 等 WebDAV 服务。</div>
                <div class="form-fields" style="margin-top:var(--space-3)">
                  <div class="field">
                    <label>服务器地址</label>
                    <input v-model="webdavURL" placeholder="https://dav.jianguoyun.com/dav/" />
                  </div>
                  <div class="field">
                    <label>用户名</label>
                    <input v-model="webdavUsername" placeholder="your@email.com" autocomplete="off" />
                  </div>
                  <div class="field">
                    <label>密码 <span class="optional">留空则不修改</span></label>
                    <input v-model="webdavPassword" type="password" placeholder="••••••••" autocomplete="new-password" />
                  </div>
                  <div class="field">
                    <label>远程路径</label>
                    <input v-model="webdavPath" placeholder="/Light/" />
                  </div>
                  <button class="btn btn-secondary" @click="saveWebDAVConfig">
                    {{ webdavConfigSaved ? '已保存 ✓' : '保存配置' }}
                  </button>
                </div>

                <div class="webdav-actions" style="margin-top:var(--space-4)">
                  <button class="btn btn-primary" @click="doBackup" :disabled="webdavBacking">
                    {{ webdavBacking ? '备份中...' : '立即备份' }}
                  </button>
                  <button class="btn btn-secondary" @click="loadBackups" :disabled="webdavListLoading">
                    {{ webdavListLoading ? '加载中...' : '查看备份列表' }}
                  </button>
                  <span v-if="webdavBackupMsg" class="backup-msg" :class="{ error: webdavBackupMsg.startsWith('✗') }">
                    {{ webdavBackupMsg }}
                  </span>
                </div>

                <div v-if="webdavListMsg && webdavBackups.length === 0" class="backup-empty">{{ webdavListMsg }}</div>

                <div v-if="webdavBackups.length > 0" class="backup-list">
                  <div class="backup-list-header">
                    <span>文件名</span>
                    <span>大小</span>
                    <span>时间</span>
                    <span>操作</span>
                  </div>
                  <div v-for="f in pagedBackups" :key="f.name" class="backup-item">
                    <span class="backup-filename">{{ f.name }}</span>
                    <span class="backup-size">{{ formatSize(f.size) }}</span>
                    <span class="backup-time">{{ f.mod_time }}</span>
                    <div class="backup-ops">
                      <button class="btn btn-sm btn-secondary" @click="doRestore(f.name)"
                        :disabled="!!webdavRestoring">
                        {{ webdavRestoring === f.name ? '恢复中...' : '恢复' }}
                      </button>
                      <button class="btn btn-sm btn-danger" @click="doDeleteBackup(f.name)"
                        :disabled="!!webdavDeleting">
                        {{ webdavDeleting === f.name ? '删除中...' : '删除' }}
                      </button>
                    </div>
                  </div>
                  <div v-if="backupPageCount > 1" class="backup-pagination">
                    <button class="btn btn-sm btn-secondary" :disabled="backupPage <= 1" @click="backupPage--">上一页</button>
                    <span class="page-info">{{ backupPage }} / {{ backupPageCount }}</span>
                    <button class="btn btn-sm btn-secondary" :disabled="backupPage >= backupPageCount" @click="backupPage++">下一页</button>
                  </div>
                </div>

                <!-- 内联确认弹窗 -->
                <div v-if="showConfirm" class="inline-confirm" ref="confirmRef">
                  <div class="inline-confirm-msg">{{ confirmMsg }}</div>
                  <div class="inline-confirm-actions">
                    <button class="btn btn-sm btn-danger" @click="doConfirm">确认</button>
                    <button class="btn btn-sm btn-secondary" @click="cancelConfirm">取消</button>
                  </div>
                </div>
              </div>
            </div>

            <!-- 模型供应商 -->
            <div v-if="mainTab === 'providers'" class="providers-layout">
              <div class="provider-list">
                <div
                  v-for="p in providers" :key="p.id"
                  class="provider-item" :class="{ active: selectedProviderID === p.id }"
                  @click="selectedProviderID = p.id; showProviderForm = false"
                >
                  <div class="provider-item-main">
                    <span class="provider-item-name">{{ p.name }}</span>
                    <span class="provider-item-type">{{ p.type }}</span>
                  </div>
                  <div class="provider-item-actions" @click.stop>
                    <label class="toggle-sm" :title="p.enabled ? '禁用' : '启用'">
                      <input type="checkbox" :checked="p.enabled" @change="toggleProvider(p)" />
                      <span class="slider-sm" />
                    </label>
                    <button class="icon-btn" @click="openEditProvider(p)" title="编辑">
                      <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>
                    </button>
                    <button class="icon-btn icon-btn-danger" @click="deleteProvider(p.id)" title="删除">
                      <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14H6L5 6M10 11v6M14 11v6M9 6V4h6v2"/></svg>
                    </button>
                  </div>
                </div>
                <button class="btn-add-provider" @click="openAddProvider">+ 自定义供应商</button>
              </div>

              <!-- 右侧：供应商详情 -->
              <div class="provider-detail" v-if="selectedProvider && !showProviderForm">
                <div class="detail-header">
                  <div>
                    <div class="detail-title">{{ selectedProvider.name }}</div>
                    <div class="detail-type">{{ PROVIDER_TYPES.find(t=>t.value===selectedProvider!.type)?.label ?? selectedProvider!.type }}</div>
                  </div>
                  <div style="display:flex;align-items:center;gap:8px">
                    <span class="status-badge" :class="selectedProvider.enabled ? 'enabled' : 'disabled'">
                      {{ selectedProvider.enabled ? '已启用' : '已禁用' }}
                    </span>
                    <button class="icon-btn" @click="openEditProvider(selectedProvider!)" title="编辑配置">
                      <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>
                    </button>
                  </div>
                </div>

                <!-- 模型列表（放在最上面，最重要） -->
                <div class="models-section">
                  <div class="models-header">
                    <span class="models-title">模型列表</span>
                    <span class="models-hint">问答时可在输入框选择</span>
                  </div>
                  <div class="model-add-row">
                    <input v-model="newModelName" placeholder="输入模型名称，如 gpt-4o" @keydown.enter="addModel" />
                    <button class="btn-add-model" @click="addModel" :disabled="!newModelName.trim()">添加</button>
                  </div>
                  <div v-if="models.length === 0" class="models-empty">暂无模型</div>
                  <div v-else class="models-list">
                    <div v-for="m in models" :key="m.id" class="model-row">
                      <span class="model-name">{{ m.name }}</span>
                      <button class="icon-btn icon-btn-danger" @click="deleteModel(m.id)">
                        <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14H6L5 6M10 11v6M14 11v6M9 6V4h6v2"/></svg>
                      </button>
                    </div>
                  </div>
                </div>

                <!-- 连接信息（折叠在下方） -->
                <details class="detail-info-collapse">
                  <summary>连接信息</summary>
                  <div class="detail-info">
                    <div class="info-row"><span class="info-label">Base URL</span><span class="info-val">{{ selectedProvider?.base_url || '（默认）' }}</span></div>
                    <div class="info-row"><span class="info-label">API Key</span><span class="info-val">{{ selectedProvider?.api_key ? '••••••••' : '（未设置）' }}</span></div>
                  </div>
                </details>
              </div>

              <!-- 右侧：添加/编辑表单 -->
              <div class="provider-detail" v-else-if="showProviderForm">
                <div class="detail-title">{{ editingProvider ? '编辑供应商' : '添加供应商' }}</div>
                <div class="form-fields">
                  <div class="field">
                    <label>名称</label>
                    <input v-model="form.name" placeholder="我的 OpenAI" />
                  </div>
                  <div class="field">
                    <label>风格</label>
                    <div class="type-grid">
                      <button v-for="t in PROVIDER_TYPES" :key="t.value"
                        class="type-btn" :class="{ active: form.type === t.value }"
                        @click="form.type = t.value">{{ t.label }}</button>
                    </div>
                  </div>
                  <div class="field">
                    <label>API Key<span v-if="form.type === 'ollama'" class="optional"> 可选</span></label>
                    <input v-model="form.api_key" type="password" :placeholder="typeInfo?.placeholder.key ?? 'sk-...'" autocomplete="off" />
                  </div>
                  <div class="field">
                    <label>Base URL <span class="optional">可选</span></label>
                    <input v-model="form.base_url" :placeholder="typeInfo?.placeholder.url ?? ''" />
                  </div>
                </div>

                <!-- 模型列表（在保存按钮上面） -->
                <div class="field">
                  <label>模型列表 <span class="optional">添加后可在问答中选择</span></label>
                  <div class="form-model-add">
                    <input v-model="formModelInput" placeholder="输入模型名称，如 gpt-4o" @keydown.enter.prevent="addFormModel" />
                    <button class="btn-add-model" @click="addFormModel" :disabled="!formModelInput.trim()">添加</button>
                  </div>
                  <div v-if="formModels.length > 0" class="form-model-tags">
                    <span v-for="m in formModels" :key="m" class="model-tag">
                      {{ m }}
                      <button @click="removeFormModel(m)">×</button>
                    </span>
                  </div>
                </div>

                <div class="form-actions">
                  <button class="btn btn-cancel" @click="cancelProviderForm">取消</button>
                  <!-- 测试连接按钮 -->
                  <button class="btn btn-test" :disabled="testConnState === 'loading'" @click="doTestConnection" title="测试连接">
                    <svg v-if="testConnState === 'loading'" class="spin-icon" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M21 12a9 9 0 1 1-6.219-8.56"/></svg>
                    <svg v-else-if="testConnState === 'ok'" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><polyline points="20 6 9 17 4 12"/></svg>
                    <svg v-else-if="testConnState === 'fail'" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
                    <svg v-else width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><path d="M5 12.55a11 11 0 0 1 14.08 0"/><path d="M1.42 9a16 16 0 0 1 21.16 0"/><path d="M8.53 16.11a6 6 0 0 1 6.95 0"/><circle cx="12" cy="20" r="1" fill="currentColor"/></svg>
                    <span :class="testConnState === 'ok' ? 'test-ok' : testConnState === 'fail' ? 'test-fail' : ''">
                      {{ testConnState === 'loading' ? '测试中…' : testConnState === 'ok' ? '成功' : testConnState === 'fail' ? '失败' : '测试' }}
                    </span>
                    <span v-if="testConnMsg && testConnState === 'fail'" class="test-msg" :title="testConnMsg">{{ testConnMsg.slice(0, 20) }}{{ testConnMsg.length > 20 ? '…' : '' }}</span>
                  </button>
                  <button class="btn btn-primary" @click="saveProviderForm" :disabled="saving || !form.name.trim()">
                    {{ saving ? '保存中...' : '保存' }}
                  </button>
                </div>
              </div>

              <!-- 右侧：空状态 -->
              <div class="provider-detail provider-empty" v-else>
                <div class="empty-icon">⚙️</div>
                <p>点击左侧供应商查看详情</p>
                <p class="hint">或点击「+ 自定义供应商」添加</p>
              </div>
            </div>

            <!-- 智能体 -->
            <div v-else-if="mainTab === 'agents'" class="agents-panel">
              <div v-if="!showAgentForm">
                <div class="agents-list">
                  <div v-for="a in agents" :key="a.id" class="agent-item">
                    <span class="agent-icon">{{ a.icon }}</span>
                    <div class="agent-info">
                      <span class="agent-name">{{ a.name }}</span>
                      <span class="agent-desc">{{ a.description }}</span>
                    </div>
                    <div class="agent-actions">
                      <span v-if="a.builtin" class="builtin-badge">内置</span>
                      <button class="icon-btn" @click="openEditAgent(a)" title="编辑">
                        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>
                      </button>
                      <button v-if="!a.builtin" class="icon-btn icon-btn-danger" @click="deleteAgent(a.id)" title="删除">
                        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14H6L5 6M10 11v6M14 11v6M9 6V4h6v2"/></svg>
                      </button>
                    </div>
                  </div>
                </div>
                <button class="btn-add-provider" style="margin-top:var(--space-3)" @click="openAddAgent">+ 新增智能体</button>
              </div>

              <!-- 添加/编辑表单 -->
              <div v-else class="agent-form">
                <div class="detail-title">{{ editingAgent ? '编辑智能体' : '新增智能体' }}</div>
                <div class="form-fields">
                  <div class="field">
                    <label>图标 <span class="optional">Emoji</span></label>
                    <div class="emoji-field">
                      <input v-model="agentForm.icon" placeholder="🤖" style="width:60px" />
                      <button class="btn-emoji-pick" type="button" @click.stop="showEmojiPicker = !showEmojiPicker" title="选择图标">
                        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><circle cx="12" cy="12" r="10"/><path d="M8 14s1.5 2 4 2 4-2 4-2"/><line x1="9" y1="9" x2="9.01" y2="9" stroke-width="3"/><line x1="15" y1="9" x2="15.01" y2="9" stroke-width="3"/></svg>
                      </button>
                      <div v-if="showEmojiPicker" class="emoji-picker" @click.stop>
                        <button
                          v-for="e in AGENT_EMOJIS" :key="e"
                          class="emoji-option"
                          :class="{ active: agentForm.icon === e }"
                          @click="agentForm.icon = e; showEmojiPicker = false"
                        >{{ e }}</button>
                      </div>
                    </div>
                  </div>
                  <div class="field">
                    <label>名称</label>
                    <input v-model="agentForm.name" placeholder="我的助手" />
                  </div>
                  <div class="field">
                    <label>简介 <span class="optional">可选</span></label>
                    <input v-model="agentForm.description" placeholder="一句话描述这个智能体的用途" />
                  </div>
                  <div class="field">
                    <label>系统提示词 <span class="optional">System Prompt</span></label>
                    <textarea v-model="agentForm.system_prompt" placeholder="你是一个专业的..." rows="6" class="agent-prompt-textarea" />
                  </div>
                </div>
                <div class="form-actions">
                  <button class="btn btn-cancel" @click="cancelAgentForm">取消</button>
                  <button class="btn btn-primary" @click="saveAgentForm" :disabled="agentSaving || !agentForm.name.trim()">
                    {{ agentSaving ? '保存中...' : '保存' }}
                  </button>
                </div>
              </div>
            </div>

            <!-- Skills 广场 -->
            <div v-else-if="mainTab === 'skills'" class="skills-market">
              <div class="skills-market-header">
                <div>
                  <div class="detail-title">Skills 广场</div>
                  <div class="detail-type">上传 ZIP 包导入技能，问答时可多选调用</div>
                </div>
                <label class="btn btn-primary upload-btn" :class="{ loading: skillImporting }">
                  <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" y1="3" x2="12" y2="15"/></svg>
                  {{ skillImporting ? '导入中...' : '导入 ZIP' }}
                  <input type="file" accept=".zip" style="display:none" @change="handleSkillZipUpload" :disabled="skillImporting" />
                </label>
              </div>
              <div v-if="skillImportError" class="skill-import-msg" :class="{ error: !skillImportError.startsWith('成功') }">
                {{ skillImportError }}
              </div>
              <div v-if="skills.length === 0" class="skills-empty">
                <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1" stroke-linecap="round"><path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"/></svg>
                <p>暂无技能，上传包含 SKILL.md 的 ZIP 文件导入</p>
              </div>
              <div v-else class="skill-list">
                <div v-for="s in skills" :key="s.id" class="skill-row">
                  <div class="skill-row-info">
                    <span class="skill-row-name">{{ s.name }}</span>
                    <span class="skill-row-desc">{{ s.description }}</span>
                  </div>
                  <div class="skill-row-actions">
                    <label class="toggle-sm" :title="s.enabled ? '禁用' : '启用'">
                      <input type="checkbox" :checked="s.enabled" @change="toggleSkill(s.id, !s.enabled)" />
                      <span class="slider-sm" />
                    </label>
                    <button class="icon-btn icon-btn-danger" @click="deleteSkill(s.id)" title="删除">
                      <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14H6L5 6M10 11v6M14 11v6M9 6V4h6v2"/></svg>
                    </button>
                  </div>
                </div>
              </div>
            </div>

            <!-- MCP 配置 -->
            <div v-else-if="mainTab === 'mcp'" class="mcp-panel">
              <MCPConfig />
            </div>
            <div v-else-if="mainTab === 'knowledge'" class="knowledge-panel">
              <KnowledgeConfig />
            </div>

            <!-- 关于 -->
            <AboutPanel v-else-if="mainTab === 'about'" />
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn btn-cancel" @click="settingsStore.setOpen(false)">关闭</button>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.overlay { position: fixed; inset: 0; background: oklch(0 0 0 / 0.35); display: flex; align-items: center; justify-content: center; z-index: 1000; animation: fadeIn var(--duration-normal) var(--ease-out); }
@keyframes fadeIn { from { opacity: 0; } to { opacity: 1; } }
.modal { background: var(--color-paper); border-radius: var(--radius-xl); width: 700px; height: 580px; display: flex; flex-direction: column; box-shadow: var(--shadow-lg); animation: slideUp var(--duration-slow) var(--ease-out); }
@keyframes slideUp { from { opacity: 0; transform: translateY(8px); } to { opacity: 1; transform: translateY(0); } }
.modal-header { display: flex; align-items: center; justify-content: space-between; padding: var(--space-4) var(--space-6); border-bottom: 1px solid var(--color-border); flex-shrink: 0; height: 56px; box-sizing: border-box; }
.modal-header h2 { font-size: var(--text-lg); font-weight: 600; margin: 0; }
.btn-close { width: 32px; height: 32px; display: flex; align-items: center; justify-content: center; border: none; border-radius: var(--radius-md); background: transparent; color: var(--color-text-3); cursor: pointer; transition: background var(--duration-fast) var(--ease-out); }
.btn-close:hover { background: var(--color-paper-3); color: var(--color-text); }
.modal-body { display: flex; flex: 1; overflow: hidden; }
.side-nav { width: 148px; flex-shrink: 0; display: flex; flex-direction: column; gap: var(--space-1); padding: var(--space-4) var(--space-3); border-right: 1px solid var(--color-border); background: var(--color-paper-2); overflow-y: auto; }
.nav-item { display: flex; align-items: center; gap: var(--space-2); padding: var(--space-2) var(--space-3); border: none; border-radius: var(--radius-md); background: transparent; font-size: var(--text-sm); font-family: inherit; color: var(--color-text-2); cursor: pointer; text-align: left; transition: background var(--duration-fast) var(--ease-out), color var(--duration-fast) var(--ease-out); }
.nav-item:hover:not(.active) { background: var(--color-paper-3); color: var(--color-text); }
.nav-item.active { background: var(--color-accent-soft); color: var(--color-accent); font-weight: 500; }
.panel { flex: 1; overflow: hidden; padding: var(--space-4) var(--space-5); display: flex; flex-direction: column; }

/* ── Providers layout ── */
.providers-layout { display: flex; gap: var(--space-4); flex: 1; overflow: hidden; }

.provider-list { width: 160px; flex-shrink: 0; display: flex; flex-direction: column; gap: var(--space-1); overflow-y: auto; }
.provider-item { padding: var(--space-2) var(--space-3); border-radius: var(--radius-md); cursor: pointer; border: 1px solid transparent; transition: background var(--duration-fast) var(--ease-out); flex-shrink: 0; }
.provider-item:hover:not(.active) { background: var(--color-paper-3); }
.provider-item.active { background: var(--color-accent-soft); border-color: var(--color-accent-soft); }
.provider-item-main { display: flex; align-items: center; gap: var(--space-1); margin-bottom: 2px; }
.provider-item-name { font-size: var(--text-sm); font-weight: 500; color: var(--color-text); }
.provider-item-type { font-size: 10px; color: var(--color-text-3); background: var(--color-paper-3); padding: 1px 4px; border-radius: var(--radius-sm); }
.provider-item-actions { display: flex; align-items: center; gap: 4px; margin-top: 4px; }

/* toggle-sm */
.toggle-sm { position: relative; display: inline-block; width: 28px; height: 16px; cursor: pointer; flex-shrink: 0; }
.toggle-sm input { opacity: 0; width: 0; height: 0; position: absolute; }
.slider-sm { position: absolute; inset: 0; background: var(--color-border-2); border-radius: var(--radius-full); transition: background var(--duration-fast) var(--ease-out); }
.slider-sm::before { content: ''; position: absolute; width: 10px; height: 10px; left: 3px; top: 3px; background: white; border-radius: 50%; transition: transform var(--duration-fast) var(--ease-out); }
.toggle-sm input:checked + .slider-sm { background: var(--color-accent); }
.toggle-sm input:checked + .slider-sm::before { transform: translateX(12px); }

.icon-btn { width: 22px; height: 22px; display: flex; align-items: center; justify-content: center; border: none; border-radius: var(--radius-sm); background: transparent; color: var(--color-text-3); cursor: pointer; transition: background var(--duration-fast) var(--ease-out), color var(--duration-fast) var(--ease-out); }
.icon-btn:hover { background: var(--color-paper-4); color: var(--color-text); }
.icon-btn-danger:hover { background: oklch(0.95 0.05 25); color: var(--color-danger); }

.btn-add-provider { margin-top: var(--space-2); padding: var(--space-2) var(--space-3); border: 1px dashed var(--color-border-2); border-radius: var(--radius-md); background: transparent; color: var(--color-text-3); font-size: var(--text-xs); font-family: inherit; cursor: pointer; text-align: left; transition: border-color var(--duration-fast) var(--ease-out), color var(--duration-fast) var(--ease-out); }
.btn-add-provider:hover { border-color: var(--color-accent); color: var(--color-accent); }

/* ── Provider detail ── */
.provider-detail { flex: 1; display: flex; flex-direction: column; gap: var(--space-3); min-width: 0; overflow-y: auto; padding-right: 2px; }
.provider-empty { align-items: center; justify-content: center; color: var(--color-text-3); font-size: var(--text-sm); text-align: center; gap: var(--space-2); }
.provider-empty .empty-icon { font-size: 28px; }
.provider-empty .hint { font-size: var(--text-xs); }

.detail-header { display: flex; align-items: flex-start; justify-content: space-between; }
.detail-title { font-size: var(--text-base); font-weight: 600; color: var(--color-text); }
.detail-type { font-size: var(--text-xs); color: var(--color-text-3); margin-top: 2px; }
.status-badge { font-size: var(--text-xs); padding: 2px 8px; border-radius: var(--radius-full); font-weight: 500; flex-shrink: 0; }
.status-badge.enabled { background: oklch(0.92 0.08 145); color: var(--color-success); }
.status-badge.disabled { background: var(--color-paper-3); color: var(--color-text-3); }

.detail-info { display: flex; flex-direction: column; gap: var(--space-2); padding: var(--space-3); background: var(--color-paper-2); border-radius: var(--radius-md); border: 1px solid var(--color-border); margin-top: var(--space-2); }
.detail-info-collapse { flex-shrink: 0; }
.detail-info-collapse summary { font-size: var(--text-xs); color: var(--color-text-3); cursor: pointer; padding: var(--space-1) 0; user-select: none; }
.detail-info-collapse summary:hover { color: var(--color-text-2); }
.info-row { display: flex; align-items: center; gap: var(--space-3); }
.form-model-add { display: flex; gap: var(--space-2); margin-top: var(--space-1); }
.form-model-add input { flex: 1; padding: var(--space-2) var(--space-3); border: 1px solid var(--color-border); border-radius: var(--radius-md); font-size: var(--text-sm); font-family: inherit; color: var(--color-text); background: var(--color-paper); outline: none; box-sizing: border-box; }
.form-model-add input:focus { border-color: var(--color-accent); box-shadow: 0 0 0 3px var(--color-accent-soft); }
.form-model-tags { display: flex; flex-wrap: wrap; gap: var(--space-1); margin-top: var(--space-2); }
.model-tag { display: inline-flex; align-items: center; gap: 4px; padding: 2px 8px 2px 10px; background: var(--color-accent-soft); color: var(--color-accent); border-radius: var(--radius-full); font-size: var(--text-xs); font-family: var(--font-mono); }
.model-tag button { border: none; background: none; color: var(--color-accent); cursor: pointer; font-size: 14px; line-height: 1; padding: 0; opacity: 0.7; }
.model-tag button:hover { opacity: 1; }
.info-label { font-size: var(--text-xs); font-weight: 500; color: var(--color-text-3); width: 60px; flex-shrink: 0; }
.info-val { font-size: var(--text-xs); color: var(--color-text-2); font-family: var(--font-mono); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

/* ── Models section ── */
.models-section { display: flex; flex-direction: column; gap: var(--space-2); flex-shrink: 0; }
.models-header { display: flex; align-items: baseline; gap: var(--space-2); }
.models-title { font-size: var(--text-sm); font-weight: 600; color: var(--color-text); }
.models-hint { font-size: var(--text-xs); color: var(--color-text-3); }
.models-empty { font-size: var(--text-xs); color: var(--color-text-3); padding: var(--space-3) 0; }
.models-list { display: flex; flex-direction: column; gap: 4px; max-height: 160px; overflow-y: auto; }
.model-row { display: flex; align-items: center; justify-content: space-between; padding: var(--space-1) var(--space-2); border-radius: var(--radius-sm); background: var(--color-paper-2); }
.model-name { font-size: var(--text-xs); font-family: var(--font-mono); color: var(--color-text-2); }
.model-add-row { display: flex; gap: var(--space-2); margin-top: var(--space-1); }
.model-add-row input { flex: 1; padding: var(--space-2) var(--space-3); border: 1px solid var(--color-border); border-radius: var(--radius-md); font-size: var(--text-sm); font-family: inherit; color: var(--color-text); background: var(--color-paper); outline: none; }
.model-add-row input:focus { border-color: var(--color-accent); box-shadow: 0 0 0 3px var(--color-accent-soft); }
.btn-add-model { padding: var(--space-2) var(--space-3); border: none; border-radius: var(--radius-md); background: var(--color-accent); color: #fff; font-size: var(--text-sm); font-family: inherit; cursor: pointer; white-space: nowrap; transition: background var(--duration-fast) var(--ease-out), opacity var(--duration-fast) var(--ease-out); }
.btn-add-model:hover:not(:disabled) { background: var(--color-accent-2); }
.btn-add-model:disabled { opacity: 0.4; cursor: not-allowed; }

/* ── Provider form ── */
.form-fields { display: flex; flex-direction: column; gap: var(--space-3); }
.field label { display: block; font-size: var(--text-xs); font-weight: 500; color: var(--color-text-2); margin-bottom: var(--space-1); }
.optional { color: var(--color-text-3); font-weight: 400; }
.field input { width: 100%; padding: var(--space-2) var(--space-3); border: 1px solid var(--color-border); border-radius: var(--radius-md); font-size: var(--text-sm); font-family: inherit; color: var(--color-text); background: var(--color-paper); outline: none; box-sizing: border-box; transition: border-color var(--duration-fast) var(--ease-out); }
.field input:focus { border-color: var(--color-accent); box-shadow: 0 0 0 3px var(--color-accent-soft); }
.type-grid { display: grid; grid-template-columns: 1fr 1fr; gap: var(--space-2); }
.type-btn { padding: var(--space-2) var(--space-3); border: 1px solid var(--color-border); border-radius: var(--radius-md); background: transparent; font-size: var(--text-sm); font-family: inherit; color: var(--color-text-2); cursor: pointer; transition: border-color var(--duration-fast) var(--ease-out), background var(--duration-fast) var(--ease-out), color var(--duration-fast) var(--ease-out); }
.type-btn:hover:not(.active) { background: var(--color-paper-3); }
.type-btn.active { border-color: var(--color-accent); background: var(--color-accent-soft); color: var(--color-accent); font-weight: 500; }
.form-actions { display: flex; justify-content: flex-end; gap: var(--space-2); padding-top: var(--space-2); }

.placeholder-panel { display: flex; flex-direction: column; align-items: center; justify-content: center; flex: 1; gap: var(--space-3); color: var(--color-text-3); }
.placeholder-icon { opacity: 0.35; }
.placeholder-title { font-size: var(--text-lg); font-weight: 600; color: var(--color-text-2); }
.placeholder-desc { font-size: var(--text-sm); text-align: center; max-width: 260px; line-height: var(--leading-relaxed); }

/* ── Footer ── */
.modal-footer { display: flex; justify-content: flex-end; gap: var(--space-2); padding: var(--space-4) var(--space-6); border-top: 1px solid var(--color-border); flex-shrink: 0; }
.btn { padding: var(--space-2) var(--space-5); border-radius: var(--radius-md); font-size: var(--text-sm); font-family: inherit; cursor: pointer; border: none; transition: background var(--duration-fast) var(--ease-out), opacity var(--duration-fast) var(--ease-out); }
.btn-cancel { background: var(--color-paper-3); color: var(--color-text-2); }
.btn-cancel:hover { background: var(--color-paper-4); }
.btn-primary { background: var(--color-accent); color: #fff; }
.btn-primary:hover:not(:disabled) { background: var(--color-accent-2); }
.btn-primary:disabled { opacity: 0.5; cursor: not-allowed; }
.btn-test { display: flex; align-items: center; gap: 4px; background: var(--color-paper-3); color: var(--color-text-2); padding: var(--space-2) var(--space-3); }
.btn-test:hover:not(:disabled) { background: var(--color-paper-4); color: var(--color-text); }
.btn-test:disabled { opacity: 0.6; cursor: not-allowed; }
.test-ok { color: var(--color-success); }
.test-fail { color: oklch(0.55 0.18 25); }
.test-msg { font-size: 10px; color: oklch(0.55 0.18 25); max-width: 120px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
@keyframes spin { to { transform: rotate(360deg); } }
.spin-icon { animation: spin 0.8s linear infinite; }
.mcp-panel { height: 100%; }
.knowledge-panel { height: 100%; overflow-y: auto; }

/* ── 通用设置 ── */
.general-panel { display: flex; flex-direction: column; gap: var(--space-5); overflow-y: auto; }
.setting-section { display: flex; flex-direction: column; gap: var(--space-1); }
.setting-section-title { font-size: var(--text-sm); font-weight: 600; color: var(--color-text); }
.setting-section-desc { font-size: var(--text-xs); color: var(--color-text-3); line-height: 1.6; }
.setting-link { font-size: var(--text-xs); color: var(--color-accent); text-decoration: none; margin-left: var(--space-2); }
.setting-link:hover { text-decoration: underline; }

.engine-tabs { display: flex; gap: var(--space-1); flex-wrap: wrap; margin-top: var(--space-1); }
.engine-tab {
  display: flex; align-items: center; gap: 5px;
  padding: var(--space-1) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-full);
  background: transparent;
  font-size: var(--text-xs);
  color: var(--color-text-2);
  cursor: pointer;
  transition: all var(--duration-fast) var(--ease-out);
}
.engine-tab:hover { border-color: var(--color-accent); color: var(--color-accent); }
.engine-tab.active { border-color: var(--color-accent); background: var(--color-accent-soft); color: var(--color-accent); font-weight: 600; }

.field-hint { font-size: var(--text-xs); color: var(--color-text-3); margin-top: var(--space-1); }

.webdav-actions { display: flex; align-items: center; gap: var(--space-2); flex-wrap: wrap; }
.backup-msg { font-size: var(--text-xs); color: var(--color-success); }
.backup-msg.error { color: var(--color-danger); }

.backup-list { margin-top: var(--space-3); border: 1px solid var(--color-border); border-radius: var(--radius-md); overflow: hidden; }
.backup-list-header {
  display: grid; grid-template-columns: 1fr 80px 160px 140px;
  padding: var(--space-2) var(--space-3);
  background: var(--color-paper-3);
  font-size: var(--text-xs); font-weight: 600; color: var(--color-text-3);
  border-bottom: 1px solid var(--color-border);
}
.backup-item {
  display: grid; grid-template-columns: 1fr 80px 160px 140px;
  align-items: center;
  padding: var(--space-2) var(--space-3);
  border-bottom: 1px solid var(--color-border);
  transition: background var(--duration-fast) var(--ease-out);
}
.backup-item:last-child { border-bottom: none; }
.backup-item:hover { background: var(--color-paper-2); }
.backup-filename { font-size: var(--text-xs); color: var(--color-text-2); font-family: var(--font-mono); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.backup-size { font-size: var(--text-xs); color: var(--color-text-3); }
.backup-time { font-size: var(--text-xs); color: var(--color-text-3); }
.backup-ops { display: flex; gap: var(--space-1); }
.btn-sm { padding: 3px var(--space-2); font-size: var(--text-xs); }

.backup-empty { font-size: var(--text-xs); color: var(--color-text-3); padding: var(--space-3) 0; }
.backup-pagination { display: flex; align-items: center; gap: var(--space-2); padding: var(--space-2) var(--space-3); border-top: 1px solid var(--color-border); }
.page-info { font-size: var(--text-xs); color: var(--color-text-3); flex: 1; text-align: center; }

.inline-confirm {
  margin-top: var(--space-3);
  padding: var(--space-3) var(--space-4);
  background: oklch(0.98 0.02 25);
  border: 1px solid oklch(0.85 0.08 25);
  border-radius: var(--radius-md);
}
[data-theme="dark"] .inline-confirm { background: oklch(0.2 0.03 25); border-color: oklch(0.35 0.08 25); }
.inline-confirm-msg { font-size: var(--text-sm); color: var(--color-text); margin-bottom: var(--space-2); line-height: 1.6; }
.inline-confirm-actions { display: flex; gap: var(--space-2); }

/* ── Skills 广场 ── */
.skills-market { display: flex; flex-direction: column; gap: var(--space-3); height: 100%; overflow-y: auto; }
.skills-market-header { display: flex; align-items: flex-start; justify-content: space-between; }
.upload-btn { display: flex; align-items: center; gap: var(--space-1); cursor: pointer; flex-shrink: 0; }
.upload-btn.loading { opacity: 0.6; pointer-events: none; }
.skill-import-msg { font-size: var(--text-xs); padding: var(--space-2) var(--space-3); border-radius: var(--radius-md); background: oklch(0.92 0.08 145); color: var(--color-success); }
.skill-import-msg.error { background: oklch(0.95 0.05 25); color: var(--color-danger); }
.skills-empty { display: flex; flex-direction: column; align-items: center; gap: var(--space-2); padding: var(--space-8) 0; color: var(--color-text-3); font-size: var(--text-sm); text-align: center; opacity: 0.6; }
.skill-list { display: flex; flex-direction: column; gap: var(--space-2); }
.skill-row { display: flex; align-items: center; gap: var(--space-3); padding: var(--space-3); border: 1px solid var(--color-border); border-radius: var(--radius-md); background: var(--color-paper-2); }
.skill-row-info { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
.skill-row-name { font-size: var(--text-sm); font-weight: 500; color: var(--color-text); }
.skill-row-desc { font-size: var(--text-xs); color: var(--color-text-3); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.skill-row-actions { display: flex; align-items: center; gap: var(--space-2); flex-shrink: 0; }

/* ── Agents ── */
.agents-panel { display: flex; flex-direction: column; height: 100%; overflow-y: auto; }
.agents-list { display: flex; flex-direction: column; gap: var(--space-2); }
.agent-item { display: flex; align-items: center; gap: var(--space-3); padding: var(--space-3); border: 1px solid var(--color-border); border-radius: var(--radius-md); background: var(--color-paper-2); }
.agent-icon { font-size: 22px; flex-shrink: 0; width: 32px; text-align: center; }
.agent-info { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
.agent-name { font-size: var(--text-sm); font-weight: 500; color: var(--color-text); }
.agent-desc { font-size: var(--text-xs); color: var(--color-text-3); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.agent-actions { display: flex; align-items: center; gap: var(--space-1); flex-shrink: 0; }
.builtin-badge { font-size: 10px; padding: 1px 6px; background: var(--color-paper-3); color: var(--color-text-3); border-radius: var(--radius-full); }
.agent-form { display: flex; flex-direction: column; gap: var(--space-4); overflow-y: auto; }
.agent-prompt-textarea { width: 100%; padding: var(--space-2) var(--space-3); border: 1px solid var(--color-border); border-radius: var(--radius-md); font-size: var(--text-sm); font-family: var(--font-mono); color: var(--color-text); background: var(--color-paper); outline: none; resize: vertical; box-sizing: border-box; line-height: 1.6; }
.agent-prompt-textarea:focus { border-color: var(--color-accent); box-shadow: 0 0 0 3px var(--color-accent-soft); }

/* emoji 选择器 */
.emoji-field { display: flex; align-items: center; gap: var(--space-2); position: relative; }
.btn-emoji-pick { display: flex; align-items: center; justify-content: center; width: 28px; height: 28px; border: 1px solid var(--color-border); border-radius: var(--radius-md); background: var(--color-paper-2); color: var(--color-text-2); cursor: pointer; flex-shrink: 0; }
.btn-emoji-pick:hover { background: var(--color-paper-3); color: var(--color-text); }
.emoji-picker { position: absolute; top: calc(100% + 4px); left: 0; z-index: 200; background: var(--color-paper); border: 1px solid var(--color-border); border-radius: var(--radius-md); box-shadow: 0 4px 16px rgba(0,0,0,.12); padding: var(--space-2); display: flex; flex-wrap: wrap; gap: 2px; width: 260px; }
.emoji-option { width: 32px; height: 32px; display: flex; align-items: center; justify-content: center; font-size: 18px; border: none; background: transparent; border-radius: var(--radius-sm); cursor: pointer; transition: background var(--duration-fast); }
.emoji-option:hover { background: var(--color-paper-3); }
.emoji-option.active { background: var(--color-accent-soft); }
</style>
