<script setup lang="ts">
import { useChatStore } from '../stores/chat'
import MessageItem from './MessageItem.vue'
import { storage as models, type storage } from '../../wailsjs/go/models'
import { ref, watch, nextTick, computed } from 'vue'
import { StreamChat } from '../../wailsjs/go/handler/ChatHandler'

const store = useChatStore()
const listRef = ref<HTMLElement | null>(null)
let userScrolled = false
let scrollTimer: ReturnType<typeof setTimeout> | null = null

// 正在重新生成的 groupID（决定在哪里内联显示流式内容）
const regenGroupId = ref<string | null>(null)

function makeStreamMsg(content: string) {
  const m = new models.Message({})
  m.id = 'streaming'
  m.role = 'assistant'
  m.content = content
  m.conversation_id = ''
  return m
}

// ── generation group 版本切换 ──────────────────────────────────────
const activeGenIndex = ref<Record<string, number>>({})

interface DisplayMsg {
  msg: storage.Message
  groupVersions: storage.Message[]
  groupID: string
}

const displayMessages = computed((): DisplayMsg[] => {
  const msgs = store.messages
  if (!msgs.length) return []

  // 按 group 归组（保留插入顺序）
  const groupMap = new Map<string, storage.Message[]>()
  const groupOrder: string[] = []

  for (const m of msgs) {
    const gid = m.generation_group_id || m.id
    if (!groupMap.has(gid)) {
      groupMap.set(gid, [])
      groupOrder.push(gid)
    }
    groupMap.get(gid)!.push(m)
  }

  const result: DisplayMsg[] = []

  for (const gid of groupOrder) {
    const group = groupMap.get(gid)!
    if (group[0].role !== 'assistant') {
      for (const m of group) {
        result.push({ msg: m, groupVersions: [], groupID: gid })
      }
      continue
    }

    const sorted = [...group].sort((a, b) => (a.gen_index ?? 0) - (b.gen_index ?? 0))
    let idx = activeGenIndex.value[gid] ?? (sorted.length - 1)
    if (idx >= sorted.length) idx = sorted.length - 1

    result.push({
      msg: sorted[idx],
      groupVersions: sorted.length > 1 ? sorted : [],
      groupID: gid,
    })
  }

  return result
})

function switchVersion(groupID: string, delta: number, versions: storage.Message[]) {
  const cur = activeGenIndex.value[groupID] ?? (versions.length - 1)
  const next = Math.max(0, Math.min(versions.length - 1, cur + delta))
  activeGenIndex.value = { ...activeGenIndex.value, [groupID]: next }
}

function currentVersionLabel(groupID: string, versions: storage.Message[]) {
  const idx = activeGenIndex.value[groupID] ?? (versions.length - 1)
  return `${idx + 1} / ${versions.length}`
}

// ── 重新生成 ─────────────────────────────────────────────────────────
async function handleRegenerate(groupID: string, displayIndex: number) {
  if (store.streaming) return
  const conv = store.conversations.find(c => c.id === store.currentConvId)
  if (!conv || !store.currentConvId) return

  // 找该 group 前面最近的 user 消息（不是全局最后一条）
  let lastUserMsg: storage.Message | null = null
  for (let i = displayIndex - 1; i >= 0; i--) {
    if (displayMessages.value[i].msg.role === 'user') {
      lastUserMsg = displayMessages.value[i].msg
      break
    }
  }
  if (!lastUserMsg) return

  // 标记当前 regenGroupId，流式内容将内联渲染在该位置
  regenGroupId.value = groupID
  // 切换到最新版本（视觉上"切换"到正在生成的位置）
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
      mode: 'chat',
      knowledge_base_id: '',
      ignore_context: false,
      context_cutoff_id: store.contextCutoffId ?? '',
      attachments: [],
      regenerate_group_id: groupID,
    } as any)
  } catch (e: any) {
    store.setStreaming(false)
    store.appendStream(`\n\n⚠️ 重新生成失败：${e}`)
  }
}

// 流结束后清除 regenGroupId，并自动把版本切到最新
watch(() => store.streaming, (v) => {
  if (!v && regenGroupId.value) {
    // 等 messages 刷新后切到最新版
    nextTick(() => {
      if (regenGroupId.value) {
        const gid = regenGroupId.value
        const msgs = store.messages
        const versionsOfGroup = msgs.filter(m => (m.generation_group_id || m.id) === gid)
        const maxIdx = versionsOfGroup.length - 1
        activeGenIndex.value = { ...activeGenIndex.value, [gid]: maxIdx }
        regenGroupId.value = null
      }
    })
  }
})

// ── 滚动控制 ─────────────────────────────────────────────────────────
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
watch(() => store.streamContent, () => {
  // 只有底部追加模式（普通对话）才自动滚动
  if (!regenGroupId.value) scrollToBottom(false)
})
watch(() => store.currentConvId, () => {
  userScrolled = false
  activeGenIndex.value = {}
  regenGroupId.value = null
  scrollToBottom(true)
})
</script>

<template>
  <div class="message-list" ref="listRef" @scroll.passive="onScroll">
    <div v-if="!store.messages.length && !store.streamContent && !store.streaming" class="msg-hint">
      <p>发送一条消息开始对话</p>
    </div>

    <template v-for="(item, idx) in displayMessages" :key="item.msg.id + '-' + item.groupID">

      <!-- 正在重新生成该 group：内联显示流式内容，替换原消息 -->
      <template v-if="item.msg.role === 'assistant' && regenGroupId === item.groupID && store.streaming">
        <MessageItem
          v-if="store.streamContent || store.streamThinking"
          :msg="makeStreamMsg(store.streamContent)"
          :streaming="true"
          :thinking="store.streamThinking"
        />
        <div v-else class="thinking">
          <div class="thinking-dots"><span /><span /><span /></div>
        </div>
      </template>

      <!-- 正常渲染 -->
      <template v-else>
        <MessageItem
          :msg="item.msg"
          :is-last="false"
          :show-actions="item.msg.role === 'assistant' && !store.streaming"
          @regenerate="handleRegenerate(item.groupID, idx)"
        />

        <!-- 版本切换条 -->
        <div v-if="item.groupVersions.length > 1" class="gen-switcher">
          <button
            class="gen-btn"
            :disabled="(activeGenIndex[item.groupID] ?? item.groupVersions.length - 1) === 0"
            @click="switchVersion(item.groupID, -1, item.groupVersions)"
          >
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><polyline points="15 18 9 12 15 6"/></svg>
          </button>
          <span class="gen-label">{{ currentVersionLabel(item.groupID, item.groupVersions) }}</span>
          <button
            class="gen-btn"
            :disabled="(activeGenIndex[item.groupID] ?? item.groupVersions.length - 1) === item.groupVersions.length - 1"
            @click="switchVersion(item.groupID, 1, item.groupVersions)"
          >
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><polyline points="9 18 15 12 9 6"/></svg>
          </button>
        </div>
      </template>

      <!-- context cutoff divider -->
      <div v-if="store.contextCutoffId && item.msg.id === store.contextCutoffId" class="ctx-divider">
        <span class="ctx-divider-line" />
        <span class="ctx-divider-label">上下文从此处清除</span>
        <span class="ctx-divider-line" />
      </div>
    </template>

    <!-- 普通对话的流式消息（非重新生成时在底部追加） -->
    <div v-if="!regenGroupId && (store.streamContent || store.streaming)" class="streaming">
      <MessageItem
        v-if="store.streamContent || store.streamThinking"
        :msg="makeStreamMsg(store.streamContent)"
        :streaming="true"
        :thinking="store.streamThinking"
      />
      <div v-else-if="store.streaming" class="thinking">
        <div class="thinking-dots"><span /><span /><span /></div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.message-list {
  flex: 1;
  overflow-y: scroll;
  padding: var(--space-2) 0;
}
.message-list::-webkit-scrollbar { width: 12px; }
.message-list::-webkit-scrollbar-track { background: transparent; margin: 8px 0; }
.message-list::-webkit-scrollbar-thumb { background: var(--color-border); border-radius: 99px; min-height: 40px; border: 3px solid transparent; background-clip: padding-box; }
.message-list::-webkit-scrollbar-thumb:hover { background-color: var(--color-text-3); border-width: 1px; background-clip: padding-box; }

.msg-hint { text-align: center; padding: var(--space-12) var(--space-4); color: var(--color-text-3); font-size: var(--text-sm); }
.streaming { opacity: 1; }

/* 版本切换条 */
.gen-switcher {
  display: flex; align-items: center; gap: var(--space-2);
  padding: 2px var(--space-6) var(--space-2);
  background: var(--color-paper-2);
}
.gen-btn {
  display: flex; align-items: center; justify-content: center;
  width: 20px; height: 20px;
  border: none; background: transparent; color: var(--color-text-3);
  cursor: pointer; border-radius: var(--radius-sm);
  transition: background var(--duration-fast), color var(--duration-fast);
}
.gen-btn:hover:not(:disabled) { background: var(--color-paper-3); color: var(--color-text); }
.gen-btn:disabled { opacity: 0.3; cursor: default; }
.gen-label { font-size: 11px; color: var(--color-text-3); min-width: 36px; text-align: center; }

.ctx-divider { display: flex; align-items: center; gap: var(--space-3); padding: var(--space-2) var(--space-6); user-select: none; }
.ctx-divider-line { flex: 1; height: 1px; background: linear-gradient(90deg, transparent, oklch(0.65 0.15 25 / 0.4), transparent); }
.ctx-divider-label { font-size: 10px; font-weight: 500; color: oklch(0.65 0.15 25 / 0.7); white-space: nowrap; letter-spacing: 0.05em; }

.thinking { display: flex; gap: var(--space-4); padding: var(--space-4) var(--space-6); }
.thinking-dots { display: flex; align-items: center; gap: 4px; padding-left: calc(28px + var(--space-4)); }
.thinking-dots span { width: 6px; height: 6px; border-radius: 50%; background: var(--color-text-3); animation: bounce 1.2s infinite ease-in-out; }
.thinking-dots span:nth-child(1) { animation-delay: 0s; }
.thinking-dots span:nth-child(2) { animation-delay: 0.2s; }
.thinking-dots span:nth-child(3) { animation-delay: 0.4s; }
@keyframes bounce { 0%, 60%, 100% { transform: translateY(0); opacity: 0.4; } 30% { transform: translateY(-6px); opacity: 1; } }
</style>
