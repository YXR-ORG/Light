import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { storage } from '../../wailsjs/go/models'

export const useChatStore = defineStore('chat', () => {
  const conversations = ref<storage.Conversation[]>([])
  const currentConvId = ref<string | null>(null)
  const messages = ref<storage.Message[]>([])
  const streaming = ref(false)
  const streamContent = ref('')

  function setConversations(list: storage.Conversation[]) {
    conversations.value = list
  }

  function setCurrentConv(id: string | null) {
    currentConvId.value = id
  }

  function setMessages(msgs: storage.Message[]) {
    messages.value = msgs
  }

  function appendMessage(msg: storage.Message) {
    messages.value.push(msg)
  }

  function setStreaming(v: boolean) {
    streaming.value = v
  }

  function appendStream(text: string) {
    streamContent.value += text
  }

  function resetStream() {
    streamContent.value = ''
  }

  return {
    conversations, currentConvId, messages, streaming, streamContent,
    setConversations, setCurrentConv, setMessages, appendMessage,
    setStreaming, appendStream, resetStream,
  }
})
