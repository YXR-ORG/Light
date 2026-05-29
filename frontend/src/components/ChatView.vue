<script setup lang="ts">
import { useChatStore } from '../stores/chat'
import MessageList from './MessageList.vue'
import InputArea from './InputArea.vue'

const store = useChatStore()
</script>

<template>
  <div class="chat-view" v-if="store.currentConvId">
    <MessageList />
    <InputArea />
  </div>
  <div class="chat-view empty" v-else>
    <div class="empty-state">
      <h2>Wails AI Chat</h2>
      <p>选择一个对话或新建一个开始聊天</p>
    </div>
  </div>
</template>

<style scoped>
.chat-view { flex: 1; display: flex; flex-direction: column; }
.chat-view.empty { align-items: center; justify-content: center; }
.empty-state { text-align: center; }
.empty-state h2 { font-size: 24px; margin-bottom: 8px; }
.empty-state p { color: var(--text-secondary); font-size: 14px; }
</style>
