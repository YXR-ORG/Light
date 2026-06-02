<script setup lang="ts">
import { useChatStore } from '../stores/chat'
import MessageItem from './MessageItem.vue'
import { storage as models } from '../../wailsjs/go/models'
import { ref, watch, nextTick, computed } from 'vue'
import { StreamChat } from '../../wailsjs/go/handler/ChatHandler'
import type { storage } from '../../wailsjs/go/models'

const store = useChatStore()
const listRef = ref<HTMLElement | null>(null)
let userScrolled = false
let scrollTimer: ReturnType<typeof setTimeout> | null = null

function makeStreamMsg(content: string) {
  const m = new models.Message({})
  m.id = 'streaming'
  m.role = 'assistant'
  m.content = content
  m.conversation_id = ''
  return m
}

// 最后一条 assistant 消息的 id（streaming 时不显示按钮）
const lastAssistantId = computed(() => {
  if (store.streaming) return null
  for (let i = store.messages.length - 1; i >= 0; i--) {
    if (store.messages[i].role === 'assistant') return store.messages[i].id
  }
  return null
})

// 重新生成：找最后一条 user 消息重发
async function handleRegenerate() {
  if (store.streaming) return
  const conv = store.conversations.find(c => c.id === store.currentConvId)
  if (!conv || !store.currentConvId) return

  // 找最后一条 user 消息
  let lastUserMsg: storage.Message | null = null
  for (let i = store.messages.length - 1; i >= 0; i--) {
    if (store.messages[i].role === 'user') { lastUserMsg = store.messages[i]; break }
  }
  if (!lastUserMsg) return

  store.resetStream()
  store.setStreaming(true)

  try {
    await StreamChat({
      conversation_id: store.currentConvId,
      content: lastUserMsg.content,
      provider: conv.provider,
      model: conv.model,
      agent_id: store.activeAgentId ?? '',
      mcp_server_ids: [],
      skill_ids: [],
      web_search: false,
      mode: (conv as any).mode || 'normal',
      knowledge_base_id: (conv as any).knowledge_base_id || '',
      ignore_context: false,
      context_cutoff_id: store.contextCutoffId ?? '',
      attachments: [],
    } as any)
  } catch (e: any) {
    store.setStreaming(false)
    store.appendStream(`\n\n⚠️ 重新生成失败：${e}`)
  }
}

function isAtBottom(): boolean {
  const el = listRef.value
  if (!el) return true
  return el.scrollHeight - el.scrollTop - el.clientHeight < 60
}

function scrollToBottom(force = false) {
  if (!force && userScrolled) return
  nextTick(() => {
    const el = listRef.value
    if (el) el.scrollTop = el.scrollHeight
  })
}

function onScroll() {
  if (!store.streaming) return
  if (isAtBottom()) { userScrolled = false; return }
  userScrolled = true
  if (scrollTimer) clearTimeout(scrollTimer)
  scrollTimer = setTimeout(() => { scrollTimer = null }, 100)
}

watch(() => store.messages.length, () => { userScrolled = false; scrollToBottom(true) })
watch(() => store.streamContent, () => scrollToBottom(false))
watch(() => store.streaming, (v) => { if (!v) userScrolled = false })
watch(() => store.currentConvId, () => { userScrolled = false; scrollToBottom(true) })
</script>

<template>
  <div class="message-list" ref="listRef" @scroll.passive="onScroll">
    <div v-if="!store.messages.length && !store.streamContent" class="msg-hint">
      <p>发送一条消息开始对话</p>
    </div>
    <template v-for="(m, idx) in store.messages" :key="m.id">
      <MessageItem
        :msg="m"
        :is-last="m.id === lastAssistantId"
        @regenerate="handleRegenerate"
      />
      <div v-if="store.contextCutoffId && m.id === store.contextCutoffId" class="ctx-divider">
        <span class="ctx-divider-line" />
        <span class="ctx-divider-label">上下文从此处清除</span>
        <span class="ctx-divider-line" />
      </div>
    </template>
    <div v-if="store.streamContent || store.streaming" class="streaming">
      <MessageItem
        v-if="store.streamContent || store.streamThinking"
        :msg="makeStreamMsg(store.streamContent)"
        :streaming="true"
        :thinking="store.streamThinking"
      />
      <div v-else-if="store.streaming" class="thinking">
        <div class="thinking-dots">
          <span /><span /><span />
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.message-list {
  flex: 1;
  overflow-y: scroll; /* 强制常显滚动条轨道 */
  padding: var(--space-2) 0;
}

.message-list::-webkit-scrollbar {
  width: 12px;
}
.message-list::-webkit-scrollbar-track {
  background: transparent;
  margin: 8px 0;
}
.message-list::-webkit-scrollbar-thumb {
  background: var(--color-border);
  border-radius: 99px;
  min-height: 40px;
  border: 3px solid transparent;
  background-clip: padding-box;
}
.message-list::-webkit-scrollbar-thumb:hover {
  background-color: var(--color-text-3);
  border-width: 1px;
  background-clip: padding-box;
}

.msg-hint {
  text-align: center;
  padding: var(--space-12) var(--space-4);
  color: var(--color-text-3);
  font-size: var(--text-sm);
}

.streaming { opacity: 1; }

.ctx-divider {
  display: flex; align-items: center; gap: var(--space-3);
  padding: var(--space-2) var(--space-6);
  user-select: none;
}
.ctx-divider-line {
  flex: 1; height: 1px;
  background: linear-gradient(90deg, transparent, oklch(0.65 0.15 25 / 0.4), transparent);
}
.ctx-divider-label {
  font-size: 10px; font-weight: 500;
  color: oklch(0.65 0.15 25 / 0.7);
  white-space: nowrap; letter-spacing: 0.05em;
}

.thinking {
  display: flex;
  gap: var(--space-4);
  padding: var(--space-4) var(--space-6);
}

.thinking-dots {
  display: flex;
  align-items: center;
  gap: 4px;
  padding-left: calc(28px + var(--space-4));
}

.thinking-dots span {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--color-text-3);
  animation: bounce 1.2s infinite ease-in-out;
}

.thinking-dots span:nth-child(1) { animation-delay: 0s; }
.thinking-dots span:nth-child(2) { animation-delay: 0.2s; }
.thinking-dots span:nth-child(3) { animation-delay: 0.4s; }

@keyframes bounce {
  0%, 60%, 100% { transform: translateY(0); opacity: 0.4; }
  30% { transform: translateY(-6px); opacity: 1; }
}
</style>
