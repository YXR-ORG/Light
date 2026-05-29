<script setup lang="ts">
import { onMounted } from 'vue'
import { useChatStore } from '../stores/chat'
import { useSettingsStore } from '../stores/settings'
import ConversationItem from './ConversationItem.vue'

import {
  List, Create, Delete, GetMessages,
} from '../../wailsjs/go/handler/ConversationHandler'

const store = useChatStore()
const settingsStore = useSettingsStore()

onMounted(() => {
  loadConversations()
})

async function loadConversations() {
  try {
    const list = await List()
    store.setConversations(list)
  } catch (e) {
    console.error('加载对话失败', e)
  }
}

async function selectConv(id: string) {
  store.setCurrentConv(id)
  try {
    const msgs = await GetMessages(id)
    store.setMessages(msgs)
  } catch (e) {
    console.error('加载消息失败', e)
  }
}

async function deleteConv(id: string) {
  try {
    await Delete(id)
  } catch (e) {
    console.error('删除失败', e)
  }
  if (store.currentConvId === id) {
    store.setCurrentConv(null)
    store.setMessages([])
  }
  loadConversations()
}

async function newChat() {
  try {
    const conv = await Create('openai', 'gpt-4o')
    store.setConversations([conv, ...store.conversations])
    store.setCurrentConv(conv.id)
    store.setMessages([])
  } catch (e) {
    console.error('新建对话失败', e)
  }
}

function openSettings() {
  settingsStore.setOpen(true)
}
</script>

<template>
  <aside class="sidebar">
    <div class="sidebar-header">
      <h2>对话</h2>
      <button class="new-btn" @click="newChat">+ 新建</button>
    </div>
    <div class="conv-list">
      <ConversationItem
        v-for="c in store.conversations"
        :key="c.id"
        :conv="c"
        :active="c.id === store.currentConvId"
        @select="selectConv"
        @delete="deleteConv"
      />
    </div>
    <div class="sidebar-footer">
      <button class="settings-btn" @click="openSettings">⚙️ 设置</button>
    </div>
  </aside>
</template>

<style scoped>
.sidebar {
  width: 260px; height: 100vh; display: flex; flex-direction: column;
  border-right: 1px solid var(--border-color); background: var(--sidebar-bg);
}
.sidebar-header {
  display: flex; align-items: center; justify-content: space-between;
  padding: 12px 16px; border-bottom: 1px solid var(--border-color);
}
.sidebar-header h2 { margin: 0; font-size: 16px; }
.new-btn {
  padding: 4px 12px; border-radius: 6px; border: 1px solid var(--border-color);
  background: var(--accent); color: #fff; cursor: pointer; font-size: 13px;
}
.conv-list { flex: 1; overflow-y: auto; padding: 8px; }
.sidebar-footer {
  padding: 12px 16px; border-top: 1px solid var(--border-color);
}
.settings-btn {
  width: 100%; padding: 8px; border-radius: 6px; border: 1px solid var(--border-color);
  background: #fff; cursor: pointer; font-size: 13px; text-align: center;
}
.settings-btn:hover { background: var(--hover-bg); }
</style>
