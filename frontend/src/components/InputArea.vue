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
  unsubs.push(EventsOn('chat:chunk', (chunk: StreamChunk) => {
    if (chunk.done) {
      store.setStreaming(false)
      store.resetStream()
      if (store.currentConvId) {
        GetMessages(store.currentConvId).then(msgs => store.setMessages(msgs))
      }
      return
    }
    store.appendStream(chunk.content)
  }))
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
    if (msg.includes('API key') || msg.includes('api_key') || msg.includes('配置')) {
      store.appendStream('\n\n⚠️ 请先在设置中配置 API Key')
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
</script>

<template>
  <div class="input-area">
    <div class="input-inner">
      <textarea
        ref="ta"
        v-model="input"
        placeholder="输入消息..."
        :disabled="store.streaming"
        @keydown="onKeydown"
        rows="1"
      />
      <div class="input-actions">
        <button class="btn-icon" @click="settingsStore.setOpen(true)" title="设置">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>
        </button>
        <button class="btn-send" :disabled="!input.trim() || store.streaming" @click="send">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor"><path d="M2.01 21 23 12 2.01 3 2 10l15 2-15 2z"/></svg>
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.input-area {
  padding: 0 var(--space-6) var(--space-5);
}

.input-inner {
  display: flex;
  align-items: flex-end;
  gap: var(--space-2);
  padding: var(--space-2);
  background: var(--color-paper);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-xl);
  transition: border-color var(--duration-normal) var(--ease-out), box-shadow var(--duration-normal) var(--ease-out);
}

.input-inner:focus-within {
  border-color: var(--color-accent);
  box-shadow: 0 0 0 3px var(--color-accent-soft);
}

textarea {
  flex: 1;
  border: none;
  resize: none;
  padding: var(--space-2) var(--space-3);
  font-family: var(--font-body);
  font-size: var(--text-sm);
  line-height: var(--leading-normal);
  color: var(--color-text);
  background: transparent;
  outline: none;
  max-height: 200px;
}

textarea::placeholder {
  color: var(--color-text-3);
}

.input-actions {
  display: flex;
  gap: var(--space-1);
}

.btn-icon, .btn-send {
  width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: background var(--duration-fast) var(--ease-out), color var(--duration-fast) var(--ease-out);
}

.btn-icon {
  background: transparent;
  color: var(--color-text-3);
}

.btn-icon:hover {
  background: var(--color-paper-3);
  color: var(--color-text-2);
}

.btn-send {
  background: var(--color-accent);
  color: #fff;
}

.btn-send:hover:not(:disabled) {
  background: var(--color-accent-2);
}

.btn-send:disabled {
  opacity: 0.35;
  cursor: not-allowed;
}
</style>
