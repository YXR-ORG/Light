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
  const streamStopped = ref(false)
  const streamStates = ref<Record<string, { content: string; thinking: string; streaming: boolean; stopped: boolean }>>({})
  // ID of the last message BEFORE the context cutoff (null = no cutoff)
  const contextCutoffId = ref<string | null>(null)
  const activeAgentId = ref<string | null>(null)
  // providers id→name 映射，由 InputArea 加载后写入，供所有组件使用
  const providerMap = ref<Record<string, string>>({})

  function setProviderMap(map: Record<string, string>) {
    providerMap.value = map
  }

  let pendingQueue = ''
  let timerId: ReturnType<typeof setInterval> | null = null
  let doneCallback: (() => void) | null = null

  function tick() {
    if (pendingQueue.length > 0) {
      const take = Math.min(CHARS_PER_TICK, pendingQueue.length)
      streamContent.value += pendingQueue.slice(0, take)
      pendingQueue = pendingQueue.slice(take)
      saveCurrentStreamState()
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
    saveCurrentStreamState()
    currentConvId.value = id
    contextCutoffId.value = null  // reset cutoff when switching conversations
    applyStreamState(id)
  }

  function setMessages(msgs: storage.Message[]) {
    messages.value = msgs
  }

  function appendMessage(msg: storage.Message) {
    messages.value.push(msg)
  }

  function setStreaming(v: boolean) {
    streaming.value = v
    saveCurrentStreamState()
  }

  function stopStreamView() {
    streamStopped.value = true
    streaming.value = false
    saveCurrentStreamState()
  }

  function appendStream(text: string) {
    pendingQueue += text
    startTimer()
  }

  function appendStreamForConv(convID: string, text: string) {
    if (convID === currentConvId.value) { appendStream(text); return }
    const state = ensureStreamState(convID)
    state.content += text
    state.streaming = true
  }

  function appendThinking(text: string) {
    streamThinking.value += text
    saveCurrentStreamState()
  }

  function appendThinkingForConv(convID: string, text: string) {
    if (convID === currentConvId.value) { appendThinking(text); return }
    const state = ensureStreamState(convID)
    state.thinking += text
    state.streaming = true
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
    streamStopped.value = false
    saveCurrentStreamState()
  }

  function resetStreamForConv(convID: string) {
    if (convID === currentConvId.value) { resetStream(); return }
    streamStates.value[convID] = { content: '', thinking: '', streaming: false, stopped: false }
  }

  function finishStreamForConv(convID: string, cb: () => void) {
    if (convID === currentConvId.value) { finishStream(cb); return }
    cb()
  }

  function setStreamingForConv(convID: string, v: boolean) {
    if (convID === currentConvId.value) { setStreaming(v); return }
    ensureStreamState(convID).streaming = v
  }

  function ensureStreamState(convID: string) {
    if (!streamStates.value[convID]) {
      streamStates.value[convID] = { content: '', thinking: '', streaming: false, stopped: false }
    }
    return streamStates.value[convID]
  }

  function saveCurrentStreamState() {
    if (!currentConvId.value) return
    streamStates.value[currentConvId.value] = {
      content: streamContent.value,
      thinking: streamThinking.value,
      streaming: streaming.value,
      stopped: streamStopped.value,
    }
  }

  function applyStreamState(convID: string | null) {
    stopTimer()
    pendingQueue = ''
    doneCallback = null
    const state = convID ? streamStates.value[convID] : null
    streamContent.value = state?.content || ''
    streamThinking.value = state?.thinking || ''
    streaming.value = state?.streaming || false
    streamStopped.value = state?.stopped || false
  }

  const taskCutoffActive = ref(false)

  function toggleTaskContextCutoff() {
    taskCutoffActive.value = !taskCutoffActive.value
  }

  function resetTaskCutoff() {
    taskCutoffActive.value = false
  }

  function toggleContextCutoff() {
    if (contextCutoffId.value !== null) {
      contextCutoffId.value = null
    } else {
      const msgs = messages.value
      contextCutoffId.value = msgs.length > 0 ? msgs[msgs.length - 1].id : '__empty__'
    }
  }

  function setActiveAgent(id: string | null) {
    activeAgentId.value = id
  }

  return {
    conversations, currentConvId, messages, streaming, streamContent, streamThinking, streamStopped,
    contextCutoffId, activeAgentId, providerMap, taskCutoffActive,
    setConversations, setCurrentConv, setMessages, appendMessage,
    setStreaming, setStreamingForConv, stopStreamView, appendStream, appendStreamForConv,
    appendThinking, appendThinkingForConv, finishStream, finishStreamForConv, resetStream, resetStreamForConv,
    toggleContextCutoff, toggleTaskContextCutoff, resetTaskCutoff, setActiveAgent, setProviderMap,
  }
})
