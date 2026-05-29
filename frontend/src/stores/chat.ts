import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { Conversation, Message } from '../types'

export const useChatStore = defineStore('chat', () => {
  const conversations = ref<Conversation[]>([])
  const currentConvId = ref<string | null>(null)
  const messages = ref<Message[]>([])
  const streaming = ref(false)
  const streamContent = ref('')

  function setConversations(list: Conversation[]) {
    conversations.value = list
  }

  function setCurrentConv(id: string | null) {
    currentConvId.value = id
  }

  function setMessages(msgs: Message[]) {
    messages.value = msgs
  }

  function appendMessage(msg: Message) {
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
