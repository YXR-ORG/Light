<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useChatStore } from '../stores/chat'
import { useSettingsStore } from '../stores/settings'
import { StreamChat } from '../../wailsjs/go/handler/ChatHandler'
import { GetMessages } from '../../wailsjs/go/handler/ConversationHandler'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import type { StreamChunk } from '../types'

const store = useChatStore()
const settingsStore = useSettingsStore()
const input = ref('')
const unsubs: (() => void)[] = []

onMounted(() => {
  const unsub = EventsOn('chat:chunk', (chunk: StreamChunk) => {
    if (chunk.done) {
      store.setStreaming(false)
      store.resetStream()
      if (store.currentConvId) {
        GetMessages(store.currentConvId).then(msgs => store.setMessages(msgs))
      }
      return
    }
    store.appendStream(chunk.content)
  })
  unsubs.push(unsub)
})

onUnmounted(() => {
  unsubs.forEach(fn => fn())
})

async function send() {
  const text = input.value.trim()
  if (!text || !store.currentConvId) return
  input.value = ''
  store.resetStream()
  store.setStreaming(true)

  try {
    const conv = store.conversations.find(c => c.id === store.currentConvId)
    await StreamChat(null, {
      conversation_id: store.currentConvId,
      content: text,
      provider: conv?.provider || 'openai',
      model: conv?.model || 'gpt-4o',
    })
  } catch (e: any) {
    const msg = e.message || String(e)
    if (msg.includes('API key') || msg.includes('api_key') || msg.includes('not configured')) {
      store.appendStream('\n\n**⚠️ 请先在设置中配置 API Key**\n\n点击侧边栏底部的 ⚙️ 按钮打开设置')
    } else {
      store.appendStream(`\n\n**错误:** ${msg}`)
    }
    store.setStreaming(false)
  }
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    send()
  }
}

function openSettings() {
  settingsStore.setOpen(true)
}
</script>

<template>
  <div class="input-area">
    <div class="input-row">
      <textarea
        v-model="input"
        placeholder="输入消息..."
        :disabled="store.streaming"
        @keydown="onKeydown"
        rows="3"
      />
      <div class="actions">
        <button class="settings-btn" @click="openSettings" title="设置">⚙️</button>
        <button class="send-btn" :disabled="!input.trim() || store.streaming" @click="send">
          发送
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.input-area {
  padding: 0 24px 16px;
}
.input-row {
  display: flex; gap: 12px; align-items: flex-end;
}
textarea {
  flex: 1; resize: none; padding: 10px 14px; border: 1px solid var(--border-color);
  border-radius: 8px; font-size: 14px; font-family: inherit; line-height: 1.5;
  outline: none;
}
textarea:focus { border-color: var(--accent); }
.actions {
  display: flex; flex-direction: column; gap: 8px;
}
.settings-btn {
  width: 40px; height: 40px; border: 1px solid var(--border-color);
  border-radius: 8px; background: #fff; cursor: pointer; font-size: 18px;
  display: flex; align-items: center; justify-content: center;
}
.send-btn {
  padding: 10px 20px; background: var(--accent); color: #fff; border: none;
  border-radius: 8px; cursor: pointer; font-size: 14px;
}
.send-btn:disabled { opacity: 0.5; cursor: not-allowed; }
</style>
