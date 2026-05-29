<script setup lang="ts">
import { onMounted } from 'vue'
import { useChatStore } from '../stores/chat'
import { useSettingsStore } from '../stores/settings'
import ConversationItem from './ConversationItem.vue'
import {
  List, Create, Delete,
} from '../../wailsjs/go/handler/ConversationHandler'

const store = useChatStore()
const settingsStore = useSettingsStore()

onMounted(loadConversations)

async function loadConversations() {
  try {
    store.setConversations(await List())
  } catch { /* ignore */ }
}

async function selectConv(id: string) {
  store.setCurrentConv(id)
  const msgs = await GetMessages(id)
  store.setMessages(msgs)
}

async function deleteConv(id: string) {
  try { await Delete(id) } catch { /* ignore */ }
  if (store.currentConvId === id) {
    store.setCurrentConv(null)
    store.setMessages([])
  }
  loadConversations()
}

async function newChat() {
  const conv = await Create('openai', 'gpt-4o')
  store.setConversations([conv, ...store.conversations])
  store.setCurrentConv(conv.id)
  store.setMessages([])
}

import { GetMessages } from '../../wailsjs/go/handler/ConversationHandler'
</script>

<template>
  <aside class="sidebar">
    <div class="sidebar-header">
      <span class="sidebar-title">对话</span>
      <button class="btn-new" @click="newChat" title="新建对话">
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none"><path d="M8 3v10M3 8h10" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>
      </button>
    </div>
    <div class="conv-list">
      <ConversationItem
        v-for="c in store.conversations" :key="c.id"
        :conv="c" :active="c.id === store.currentConvId"
        @select="selectConv" @delete="deleteConv"
      />
      <div v-if="!store.conversations.length" class="empty-list">暂无对话</div>
    </div>
    <div class="sidebar-footer">
      <button class="btn-settings" @click="settingsStore.setOpen(true)">
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none"><path d="M8 10a2 2 0 1 0 0-4 2 2 0 0 0 0 4Z" stroke="currentColor" stroke-width="1.3"/><path d="M13.5 8a5.5 5.5 0 0 1-.1 1l1.5 1.2-1.3 2.3-1.8-.4a5.5 5.5 0 0 1-1.8 1L9.5 15h-3l-.5-1.9a5.5 5.5 0 0 1-1.8-1l-1.8.4L1 10.2 2.6 9A5.5 5.5 0 0 1 2.5 8c0-.34.03-.67.1-1L1 5.8l1.3-2.3 1.8.4a5.5 5.5 0 0 1 1.8-1L6.5 1h3l.5 1.9a5.5 5.5 0 0 1 1.8 1l1.8-.4L15 5.8 13.4 7c.07.33.1.66.1 1Z" stroke="currentColor" stroke-width="1.3"/></svg>
        设置
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
  border-bottom: 1px solid var(--color-border);
}

.sidebar-title {
  font-size: var(--text-lg);
  font-weight: 600;
  letter-spacing: -0.01em;
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

.conv-list {
  flex: 1;
  overflow-y: auto;
  padding: var(--space-2);
}

.empty-list {
  text-align: center;
  padding: var(--space-8) var(--space-4);
  color: var(--color-text-3);
  font-size: var(--text-sm);
}

.sidebar-footer {
  padding: var(--space-3) var(--space-4);
  border-top: 1px solid var(--color-border);
}

.btn-settings {
  width: 100%;
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
</style>
