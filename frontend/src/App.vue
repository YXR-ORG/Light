<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import { useSettingsStore } from './stores/settings'
import { useChatStore } from './stores/chat'
import SettingsDialog from './components/SettingsDialog.vue'
import Sidebar from './components/Sidebar.vue'
import ChatView from './components/ChatView.vue'
import { EventsOn } from '../wailsjs/runtime/runtime'
import { GetMessages } from '../wailsjs/go/handler/ConversationHandler'
import type { StreamChunk } from './types'

const settingsStore = useSettingsStore()
const chatStore = useChatStore()
const unsubs: (() => void)[] = []

onMounted(() => {
  unsubs.push(EventsOn('chat:chunk', (chunk: StreamChunk) => {
    if (chunk.done) {
      chatStore.setStreaming(false)
      if (chunk.error) {
        chatStore.appendStream(`\n\n⚠️ ${chunk.error}`)
      }
      chatStore.finishStream(() => {
        if (chatStore.currentConvId) {
          GetMessages(chatStore.currentConvId).then(msgs => {
            chatStore.setMessages(msgs)
            chatStore.resetStream()
          })
        } else {
          chatStore.resetStream()
        }
      })
      return
    }
    if (chunk.thinking) {
      chatStore.appendThinking(chunk.thinking)
    }
    if (chunk.content) {
      chatStore.appendStream(chunk.content)
    }
  }))
})

onUnmounted(() => {
  unsubs.forEach(fn => fn())
})
</script>

<template>
  <div class="app-shell">
    <Sidebar />
    <main class="main-area">
      <ChatView />
    </main>
  </div>
  <SettingsDialog />
</template>

<style>
@import './assets/tokens.css';

.app-shell {
  display: flex;
  height: 100vh;
  background: var(--color-paper);
}

.main-area {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
}
</style>
