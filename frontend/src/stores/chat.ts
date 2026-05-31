import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { storage } from '../../wailsjs/go/models'

// 打字机速度：每次 tick 输出的字符数
const CHARS_PER_TICK = 4
// tick 间隔 ms，16ms ≈ 60fps，调大则更慢
const TICK_INTERVAL_MS = 16

export const useChatStore = defineStore('chat', () => {
  const conversations = ref<storage.Conversation[]>([])
  const currentConvId = ref<string | null>(null)
  const messages = ref<storage.Message[]>([])
  const streaming = ref(false)
  const streamContent = ref('')
  const streamThinking = ref('')
  // ID of the last message BEFORE the context cutoff (null = no cutoff)
  const contextCutoffId = ref<string | null>(null)

  let pendingQueue = ''
  let timerId: ReturnType<typeof setInterval> | null = null
  let doneCallback: (() => void) | null = null

  function tick() {
    if (pendingQueue.length > 0) {
      const take = Math.min(CHARS_PER_TICK, pendingQueue.length)
      streamContent.value += pendingQueue.slice(0, take)
      pendingQueue = pendingQueue.slice(take)
    } else {
      if (doneCallback) {
        stopTimer()
        const cb = doneCallback
        doneCallback = null
        cb()
      } else {
        stopTimer()
      }
    }
  }

  function startTimer() {
    if (timerId === null) {
      timerId = setInterval(tick, TICK_INTERVAL_MS)
    }
  }

  function stopTimer() {
    if (timerId !== null) {
      clearInterval(timerId)
      timerId = null
    }
  }

  function setConversations(list: storage.Conversation[]) {
    conversations.value = list
  }

  function setCurrentConv(id: string | null) {
    currentConvId.value = id
    contextCutoffId.value = null  // reset cutoff when switching conversations
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
    pendingQueue += text
    startTimer()
  }

  function appendThinking(text: string) {
    streamThinking.value += text
  }

  function finishStream(cb: () => void) {
    if (pendingQueue.length === 0 && timerId === null) {
      cb()
    } else {
      doneCallback = cb
      startTimer()
    }
  }

  function resetStream() {
    stopTimer()
    pendingQueue = ''
    doneCallback = null
    streamContent.value = ''
    streamThinking.value = ''
  }

  function toggleContextCutoff() {
    if (contextCutoffId.value !== null) {
      contextCutoffId.value = null
    } else {
      const msgs = messages.value
      contextCutoffId.value = msgs.length > 0 ? msgs[msgs.length - 1].id : '__empty__'
    }
  }

  return {
    conversations, currentConvId, messages, streaming, streamContent, streamThinking,
    contextCutoffId,
    setConversations, setCurrentConv, setMessages, appendMessage,
    setStreaming, appendStream, appendThinking, finishStream, resetStream,
    toggleContextCutoff,
  }
})
