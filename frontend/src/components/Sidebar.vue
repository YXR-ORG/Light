<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useChatStore } from '../stores/chat'
import { useSettingsStore } from '../stores/settings'
import { useThemeStore } from '../stores/theme'
import ConversationItem from './ConversationItem.vue'
import { Create, Delete, List, Search, GetMessages, Rename, ToggleFavorite } from '../../wailsjs/go/handler/ConversationHandler'
import { Get } from '../../wailsjs/go/handler/SettingsHandler'
import { ListProviders } from '../../wailsjs/go/handler/ProviderHandler'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { ListAgents } from '../../wailsjs/go/handler/AgentHandler'
import { SetAgent } from '../../wailsjs/go/handler/ConversationHandler'
import type { storage } from '../../wailsjs/go/models'

const store = useChatStore()
const settingsStore = useSettingsStore()
const themeStore = useThemeStore()

const themeTitle = computed(() => ({
  light: '浅色模式（点击切换）',
  dark: '深色模式（点击切换）',
  system: '跟随系统（点击切换）',
}[themeStore.mode]))

const searchQuery = ref('')
const convTab = ref<'all' | 'starred'>('all')
let debounceTimer: ReturnType<typeof setTimeout> | null = null
let unsubConvUpdated: (() => void) | null = null

const agents = ref<storage.Agent[]>([])
const showAgentDropdown = ref(false)
const agentSelectRef = ref<HTMLElement | null>(null)

const activeAgent = computed(() =>
  agents.value.find(a => a.id === store.activeAgentId) ?? null
)

// 按 tab 过滤展示列表
const visibleConvs = computed(() => {
  if (convTab.value === 'starred') {
    return store.conversations.filter(c => c.starred)
  }
  return store.conversations
})

onMounted(() => {
  loadConversations()
  loadAgents()
  unsubConvUpdated = EventsOn('conversation:updated', handleConversationUpdated)
  document.addEventListener('mousedown', onDocClick)
})

onUnmounted(() => {
  if (unsubConvUpdated) unsubConvUpdated()
  document.removeEventListener('mousedown', onDocClick)
})

function onDocClick(e: MouseEvent) {
  if (agentSelectRef.value && !agentSelectRef.value.contains(e.target as Node)) {
    showAgentDropdown.value = false
  }
}

async function handleConversationUpdated(_convId: string) {
  try {
    store.setConversations(await List())
  } catch { /* ignore */ }
}

async function loadConversations() {
  try {
    store.setConversations(await List())
  } catch { /* ignore */ }
}

async function loadAgents() {
  agents.value = await ListAgents().catch(() => [])
}

async function selectAgent(agent: storage.Agent) {
  store.setActiveAgent(agent.id)
  if (store.currentConvId) {
    await SetAgent(store.currentConvId, agent.id).catch(console.error)
  }
}

watch(searchQuery, (q) => {
  if (debounceTimer) clearTimeout(debounceTimer)
  debounceTimer = setTimeout(async () => {
    try {
      store.setConversations(await Search(q))
    } catch {
      try { store.setConversations(await List()) } catch { /* ignore */ }
    }
  }, 300)
})

async function selectConv(id: string) {
  store.setCurrentConv(id)
  const msgs = await GetMessages(id)
  store.setMessages(msgs)
  const conv = store.conversations.find(c => c.id === id)
  store.setActiveAgent((conv as any)?.agent_id || null)
}

async function deleteConv(id: string) {
  try { await Delete(id) } catch { /* ignore */ }
  if (store.currentConvId === id) {
    store.setCurrentConv(null)
    store.setMessages([])
  }
  try {
    store.setConversations(await Search(searchQuery.value))
  } catch { /* ignore */ }
}

async function renameConv(id: string, title: string) {
  try {
    await Rename(id, title)
    // 本地乐观更新
    store.setConversations(store.conversations.map(c =>
      c.id === id ? { ...c, title } : c
    ))
  } catch { /* ignore */ }
}

async function toggleFavorite(id: string) {
  try {
    const newVal = await ToggleFavorite(id)
    store.setConversations(store.conversations.map(c =>
      c.id === id ? { ...c, starred: newVal } : c
    ))
  } catch { /* ignore */ }
}

async function newChat() {
  let providerID = ''
  let model = ''
  try {
    const providers = await ListProviders()
    const enabled = providers.filter((p: storage.LLMProvider) => p.enabled)
    if (enabled.length > 0) {
      providerID = enabled[0].id
      model = await Get('default_model').catch(() => '') || 'gpt-4o'
    }
  } catch { /* ignore */ }

  if (!providerID) {
    providerID = await Get('default_provider').catch(() => 'openai') || 'openai'
    model = await Get('default_model').catch(() => 'gpt-4o') || 'gpt-4o'
  }

  const conv = await Create(providerID, model)
  store.setConversations([conv, ...store.conversations])
  store.setCurrentConv(conv.id)
  store.setMessages([])
}
</script>

<template>
  <aside class="sidebar">
    <div class="sidebar-header">
      <div class="app-brand">
        <!-- Light icon: radiating rays -->
        <svg class="app-logo" width="22" height="22" viewBox="0 0 64 64" fill="none" xmlns="http://www.w3.org/2000/svg">
          <circle cx="32" cy="32" r="10" fill="#ffe066"/>
          <circle cx="32" cy="32" r="6" fill="#fff"/>
          <g stroke="#ffe066" stroke-linecap="round">
            <line x1="32" y1="4"  x2="32" y2="14" stroke-width="3.5" opacity="0.9"/>
            <line x1="32" y1="50" x2="32" y2="60" stroke-width="3.5" opacity="0.9"/>
            <line x1="4"  y1="32" x2="14" y2="32" stroke-width="3.5" opacity="0.9"/>
            <line x1="50" y1="32" x2="60" y2="32" stroke-width="3.5" opacity="0.9"/>
            <line x1="11.5" y1="11.5" x2="18.5" y2="18.5" stroke-width="2.5" opacity="0.6"/>
            <line x1="45.5" y1="45.5" x2="52.5" y2="52.5" stroke-width="2.5" opacity="0.6"/>
            <line x1="52.5" y1="11.5" x2="45.5" y2="18.5" stroke-width="2.5" opacity="0.6"/>
            <line x1="18.5" y1="45.5" x2="11.5" y2="52.5" stroke-width="2.5" opacity="0.6"/>
          </g>
        </svg>
        <span class="app-name">Light</span>
      </div>
      <button class="btn-new" @click="newChat" title="新建对话">
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none"><path d="M8 3v10M3 8h10" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>
      </button>
    </div>
    <div class="search-wrap">
      <svg class="search-icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <circle cx="11" cy="11" r="8"/><path d="m21 21-4.35-4.35"/>
      </svg>
      <input
        v-model="searchQuery"
        class="search-input"
        type="text"
        placeholder="搜索对话…"
        autocomplete="off"
        spellcheck="false"
      />
      <button v-if="searchQuery" class="search-clear" @click="searchQuery = ''" title="清除">
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M18 6 6 18M6 6l12 12"/></svg>
      </button>
    </div>

    <!-- 智能体下拉选择器 -->
    <div v-if="agents.length > 0" class="agent-select-wrap" ref="agentSelectRef">
      <button class="agent-select-btn" @click="showAgentDropdown = !showAgentDropdown">
        <span class="agent-select-icon">{{ activeAgent?.icon ?? '🤖' }}</span>
        <span class="agent-select-name">{{ activeAgent?.name ?? '选择智能体' }}</span>
        <svg class="agent-select-arrow" :class="{ open: showAgentDropdown }" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><polyline points="6 9 12 15 18 9"/></svg>
      </button>
      <transition name="dropdown">
        <div v-if="showAgentDropdown" class="agent-dropdown">
          <button
            v-for="a in agents"
            :key="a.id"
            class="agent-dropdown-item"
            :class="{ active: store.activeAgentId === a.id }"
            @click="selectAgent(a); showAgentDropdown = false"
          >
            <span class="agent-dropdown-icon">{{ a.icon }}</span>
            <div class="agent-dropdown-info">
              <span class="agent-dropdown-name">{{ a.name }}</span>
              <span class="agent-dropdown-desc">{{ a.description }}</span>
            </div>
            <svg v-if="store.activeAgentId === a.id" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" class="agent-dropdown-check"><polyline points="20 6 9 17 4 12"/></svg>
          </button>
        </div>
      </transition>
    </div>
    <!-- 全部 / 收藏 tab -->
    <div class="conv-tabs">
      <button class="conv-tab" :class="{ active: convTab === 'all' }" @click="convTab = 'all'">全部</button>
      <button class="conv-tab" :class="{ active: convTab === 'starred' }" @click="convTab = 'starred'">
        <svg width="11" height="11" viewBox="0 0 24 24" fill="currentColor" stroke="none"><polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"/></svg>
        收藏
      </button>
    </div>

    <div class="conv-list">
      <ConversationItem
        v-for="c in visibleConvs" :key="c.id"
        :conv="c" :active="c.id === store.currentConvId"
        :highlight="searchQuery"
        @select="selectConv"
        @delete="deleteConv"
        @rename="renameConv"
        @toggle-favorite="toggleFavorite"
      />
      <div v-if="!visibleConvs.length" class="empty-list">
        {{ convTab === 'starred' ? '还没有收藏的对话' : searchQuery ? '无匹配对话' : '暂无对话' }}
      </div>
    </div>
    <div class="sidebar-footer">
      <button class="btn-settings" @click="settingsStore.setOpen(true)">
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none"><path d="M8 10a2 2 0 1 0 0-4 2 2 0 0 0 0 4Z" stroke="currentColor" stroke-width="1.3"/><path d="M13.5 8a5.5 5.5 0 0 1-.1 1l1.5 1.2-1.3 2.3-1.8-.4a5.5 5.5 0 0 1-1.8 1L9.5 15h-3l-.5-1.9a5.5 5.5 0 0 1-1.8-1l-1.8.4L1 10.2 2.6 9A5.5 5.5 0 0 1 2.5 8c0-.34.03-.67.1-1L1 5.8l1.3-2.3 1.8.4a5.5 5.5 0 0 1 1.8-1L6.5 1h3l.5 1.9a5.5 5.5 0 0 1 1.8 1l1.8-.4L15 5.8 13.4 7c.07.33.1.66.1 1Z" stroke="currentColor" stroke-width="1.3"/></svg>
        设置
      </button>
      <button class="btn-icon" @click="themeStore.toggle()" :title="themeTitle">
        <svg v-if="themeStore.mode === 'light'" width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41"/></svg>
        <svg v-else-if="themeStore.mode === 'dark'" width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>
        <svg v-else width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><rect x="2" y="3" width="20" height="14" rx="2"/><path d="M8 21h8M12 17v4"/></svg>
      </button>
    </div>
  </aside>
</template>

<style scoped>
.sidebar {
  width: 280px;
  height: 100vh;
  display: flex;
  flex-direction: column;
  background: var(--color-sidebar);
  border-right: 1px solid var(--color-border);
  flex-shrink: 0;
}

.sidebar-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--space-4) var(--space-5);
  padding-top: calc(var(--space-4) + 20px); /* leave room for macOS traffic lights */
  border-bottom: 1px solid var(--color-border);
}

.app-brand {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.app-logo {
  flex-shrink: 0;
  filter: drop-shadow(0 0 4px oklch(0.85 0.18 85 / 0.5));
}

.app-name {
  font-size: var(--text-lg);
  font-weight: 700;
  letter-spacing: -0.02em;
  background: linear-gradient(135deg, #ffe066 0%, #ff9500 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.btn-new {
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: var(--radius-md);
  background: var(--color-accent);
  color: #fff;
  cursor: pointer;
  transition: background var(--duration-fast) var(--ease-out);
}

.btn-new:hover { background: var(--color-accent-2); }

/* ── Search bar ── */
.search-wrap {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  margin: var(--space-2) var(--space-3);
  padding: 0 var(--space-2);
  height: 32px;
  background: var(--color-paper-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  transition: border-color var(--duration-fast) var(--ease-out),
              background var(--duration-fast) var(--ease-out);
}

.search-wrap:focus-within {
  border-color: var(--color-accent);
  background: var(--color-paper);
}

.search-icon {
  flex-shrink: 0;
  color: var(--color-text-3);
}

.search-input {
  flex: 1;
  min-width: 0;
  border: none;
  background: transparent;
  font-family: var(--font-body);
  font-size: var(--text-sm);
  color: var(--color-text);
  outline: none;
}

.search-input::placeholder {
  color: var(--color-text-3);
}

.search-clear {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-text-3);
  cursor: pointer;
  padding: 0;
  transition: background var(--duration-fast) var(--ease-out);
}

.search-clear:hover {
  background: var(--color-paper-4);
  color: var(--color-text);
}

/* ── 对话 tab 切换 ── */
.conv-tabs {
  display: flex;
  gap: 2px;
  margin: var(--space-1) var(--space-3) var(--space-1);
  flex-shrink: 0;
}
.conv-tab {
  flex: 1;
  display: flex; align-items: center; justify-content: center; gap: 4px;
  padding: var(--space-1) var(--space-2);
  border: none; background: transparent;
  border-radius: var(--radius-md);
  font-size: var(--text-xs); font-weight: 500;
  color: var(--color-text-3);
  cursor: pointer;
  transition: background var(--duration-fast), color var(--duration-fast);
  font-family: var(--font-body);
}
.conv-tab:hover { background: var(--color-paper-3); color: var(--color-text-2); }
.conv-tab.active {
  background: var(--color-accent-soft);
  color: var(--color-accent);
}

.conv-list {
  flex: 1;
  overflow-y: auto;
  padding: var(--space-2);
}

/* ── Agent selector ── */
.agent-select-wrap {
  position: relative;
  margin: 0 var(--space-3) var(--space-2);
  flex-shrink: 0;
}

.agent-select-btn {
  width: 100%;
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-paper-2);
  cursor: pointer;
  font-family: var(--font-body);
  font-size: var(--text-sm);
  color: var(--color-text-2);
  transition: border-color var(--duration-fast) var(--ease-out),
              background var(--duration-fast) var(--ease-out);
}
.agent-select-btn:hover {
  border-color: var(--color-accent);
  background: var(--color-paper);
}
.agent-select-icon { font-size: 15px; line-height: 1; flex-shrink: 0; }
.agent-select-name { flex: 1; text-align: left; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.agent-select-arrow {
  flex-shrink: 0;
  color: var(--color-text-3);
  transition: transform var(--duration-fast) var(--ease-out);
}
.agent-select-arrow.open { transform: rotate(180deg); }

.agent-dropdown {
  position: absolute;
  top: calc(100% + 4px);
  left: 0; right: 0;
  z-index: 200;
  background: var(--color-paper);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-lg);
  overflow: hidden;
  padding: var(--space-1);
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.agent-dropdown-item {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-2);
  border: none;
  border-radius: var(--radius-md);
  background: transparent;
  cursor: pointer;
  text-align: left;
  width: 100%;
  font-family: var(--font-body);
  transition: background var(--duration-fast) var(--ease-out);
}
.agent-dropdown-item:hover { background: var(--color-paper-3); }
.agent-dropdown-item.active { background: var(--color-accent-soft); }
.agent-dropdown-icon { font-size: 16px; line-height: 1; flex-shrink: 0; width: 24px; text-align: center; }
.agent-dropdown-info { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
.agent-dropdown-name { font-size: var(--text-sm); font-weight: 500; color: var(--color-text); }
.agent-dropdown-desc { font-size: var(--text-xs); color: var(--color-text-3); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.agent-dropdown-check { flex-shrink: 0; color: var(--color-accent); }

.dropdown-enter-active, .dropdown-leave-active {
  transition: opacity var(--duration-fast) var(--ease-out), transform var(--duration-fast) var(--ease-out);
}
.dropdown-enter-from, .dropdown-leave-to { opacity: 0; transform: translateY(-4px); }

.empty-list {
  text-align: center;
  padding: var(--space-8) var(--space-4);
  color: var(--color-text-3);
  font-size: var(--text-sm);
}

.sidebar-footer {
  padding: var(--space-3) var(--space-4);
  border-top: 1px solid var(--color-border);
  display: flex;
  align-items: center;
  gap: var(--space-1);
}

.btn-settings {
  flex: 1;
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3);
  border: none;
  border-radius: var(--radius-md);
  background: transparent;
  color: var(--color-text-2);
  font-size: var(--text-sm);
  cursor: pointer;
  transition: background var(--duration-fast) var(--ease-out);
}

.btn-settings:hover { background: var(--color-sidebar-hover); color: var(--color-text); }

.btn-icon {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border: none;
  border-radius: var(--radius-md);
  background: transparent;
  color: var(--color-text-3);
  cursor: pointer;
  transition: background var(--duration-fast) var(--ease-out), color var(--duration-fast) var(--ease-out);
}

.btn-icon:hover { background: var(--color-sidebar-hover); color: var(--color-text); }
</style>
