<script setup lang="ts">
import { ref } from 'vue'
import { useChatStore } from '../stores/chat'
import { SendMessage } from '../../wailsjs/go/handler/ChatHandler'
import { GetMessages } from '../../wailsjs/go/handler/ConversationHandler'
import type { storage } from '../../wailsjs/go/models'

const store = useChatStore()
const input = ref('')

async function send() {
  const text = input.value.trim()
  if (!text || !store.currentConvId) return
  input.value = ''
  store.setStreaming(true)

  try {
    const conv = store.conversations.find(c => c.id === store.currentConvId)
    await SendMessage(null, {
      conversation_id: store.currentConvId,
      content: text,
      provider: conv?.provider || 'openai',
      model: conv?.model || 'gpt-4o',
    })
    const msgs = await GetMessages(store.currentConvId)
    store.setMessages(msgs)
  } catch (e: any) {
    store.appendStream(`\n\n**Error:** ${e.message || e}`)
  } finally {
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
    <textarea
      v-model="input"
      placeholder="Type a message..."
      :disabled="store.streaming"
      @keydown="onKeydown"
      rows="3"
    />
    <button class="send-btn" :disabled="!input.trim() || store.streaming" @click="send">
      Send
    </button>
  </div>
</template>

<style scoped>
.input-area {
  padding: 16px 24px; border-top: 1px solid var(--border-color);
  display: flex; gap: 12px; align-items: flex-end;
}
textarea {
  flex: 1; resize: none; padding: 10px 14px; border: 1px solid var(--border-color);
  border-radius: 8px; font-size: 14px; font-family: inherit; line-height: 1.5;
  outline: none;
}
textarea:focus { border-color: var(--accent); }
.send-btn {
  padding: 10px 20px; background: var(--accent); color: #fff; border: none;
  border-radius: 8px; cursor: pointer; font-size: 14px;
}
.send-btn:disabled { opacity: 0.5; cursor: not-allowed; }
</style>
