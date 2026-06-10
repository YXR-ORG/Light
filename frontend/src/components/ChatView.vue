<script setup lang="ts">
import { useChatStore } from '../stores/chat'
import MessageList from './MessageList.vue'
import TaskMessageItem, { type TaskStep } from './TaskMessageItem.vue'
import InputArea from './InputArea.vue'
import { ref, watch, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import { GetMessages } from '../../wailsjs/go/handler/ConversationHandler'
import type { storage } from '../../wailsjs/go/models'
import { isNearBottom, shouldAutoScroll } from '../utils/scroll'
import { shouldShowTaskHistory } from '../utils/taskHistory'

const store = useChatStore()

// ─── task 模式状态 ────────────────────────────────────────────────
const taskListRef = ref<HTMLElement | null>(null)
let taskUserScrolled = false
let taskScrollTimer: ReturnType<typeof setTimeout> | null = null

// 已完成的轮次（保留推理链，不清空）
interface TaskRound {
  userContent: string
  attachmentsMeta?: string
  steps: TaskStep[]
  assistantContent: string
  notice?: string    // 撞上限/死循环时的提示语
  isCutoff?: boolean  // 该轮次之后有上下文分割线
}
const completedRounds = ref<TaskRound[]>([])

// 历史 task 消息（从 DB 加载，仅用于跨会话恢复）
const taskHistoryMsgs = ref<storage.Message[]>([])

// 当前流式 task 步骤
const currentTaskSteps = ref<TaskStep[]>([])
// 当前流式 task 对应的用户消息内容
const currentTaskUserContent = ref('')
const currentTaskAttachmentsMeta = ref('')
// 流式内容累加
const streamingContent = ref('')
// 当前轮的提示语（撞上限/死循环）
const currentNotice = ref('')
// 是否正在流式输出 task
const taskStreaming = ref(false)

// 当前会话是否为 task 模式
const isTaskMode = computed(() => {
  const conv = store.conversations.find(c => c.id === store.currentConvId) as any
  return conv?.mode === 'task'
})

const showTaskHistory = computed(() => shouldShowTaskHistory(taskHistoryMsgs.value.length, completedRounds.value.length))

// task 模式清除上下文：taskCutoffActive=true 时，在已显示内容末尾画分割线，
// 下次发送只传 cutoff 之后的历史（后端 ignore_context 全量清空历史）。
// 分割线由模板根据 store.taskCutoffActive 直接渲染，无需在此标记轮次。

// 加载 task 历史消息（跨会话恢复用）
async function loadTaskHistory() {
  if (!store.currentConvId) return
  try {
    taskHistoryMsgs.value = await GetMessages(store.currentConvId)
  } catch {
    taskHistoryMsgs.value = []
  }
}

// 监听会话切换：清空当前轮次，重新加载历史
watch(() => store.currentConvId, async () => {
  completedRounds.value = []
  currentTaskSteps.value = []
  streamingContent.value = ''
  currentNotice.value = ''
  currentTaskUserContent.value = ''
  currentTaskAttachmentsMeta.value = ''
  taskStreaming.value = false
  taskUserScrolled = false
  if (isTaskMode.value) await loadTaskHistory()
  scrollTaskToBottom(true)
}, { immediate: true })

// 监听模式切换
watch(isTaskMode, async (v) => {
  if (v) await loadTaskHistory()
})

// ─── task:step 事件处理 ───────────────────────────────────────────
interface TaskStepEvent {
  conv_id: string
  type: string
  content?: string
  tool_name?: string
  tool_args?: string
  tool_result?: string
  confirm_id?: string
  cmd?: string
  error?: string
  user_content?: string
  attachments_meta?: string
}

function onTaskStep(evt: TaskStepEvent) {
  if (evt.type === 'user_msg') {
    currentTaskSteps.value = []
    streamingContent.value = ''
    currentNotice.value = ''
    currentTaskUserContent.value = evt.user_content || evt.content || ''
    currentTaskAttachmentsMeta.value = evt.attachments_meta || ''
    taskUserScrolled = false
    taskStreaming.value = true
    store.setStreaming(true)
    scrollTaskToBottom(true)
    return
  }

  if (evt.type === 'content') {
    streamingContent.value += evt.content || ''
    scrollTaskToBottom()
    return
  }

  // content_note：含 tool_call 轮次的过程旁白 → 归入折叠链，不进正文
  if (evt.type === 'content_note') {
    currentTaskSteps.value.push({ type: 'content_note', content: evt.content })
    scrollTaskToBottom()
    return
  }

  // content_rollback：本轮实时推送的 content 其实是“过程旁白”（该轮含 tool_call）。
  // 把这段内容从正文末尾撤回，改入折叠链作 content_note。
  if (evt.type === 'content_rollback') {
    const seg = evt.content || ''
    if (seg && streamingContent.value.endsWith(seg)) {
      streamingContent.value = streamingContent.value.slice(0, -seg.length)
    } else if (seg) {
      // 兜底：按长度从尾部移除（chunk 拼接与聚合文本理论一致）
      streamingContent.value = streamingContent.value.slice(0, Math.max(0, streamingContent.value.length - seg.length))
    }
    // 大段 tool-call 轮内容通常是模型误把“最终总结”伴随 update_plan 一起吐出。
    // 已从正文回滚即可，不再塞进推理链，避免最终界面看起来像总结输出了两次。
    if (seg.length <= 1200) {
      currentTaskSteps.value.push({ type: 'content_note', content: seg })
    }
    scrollTaskToBottom()
    return
  }

  // notice：撞上限/死循环的提示语
  if (evt.type === 'notice') {
    currentNotice.value = evt.content || ''
    scrollTaskToBottom()
    return
  }

  if (evt.type === 'done') {
    taskStreaming.value = false
    store.setStreaming(false)
    // 把本轮推理链存入 completedRounds，永久显示
    completedRounds.value.push({
      userContent: currentTaskUserContent.value,
      attachmentsMeta: currentTaskAttachmentsMeta.value,
      steps: [...currentTaskSteps.value],
      assistantContent: streamingContent.value,
      notice: currentNotice.value,
    })
    // 清空当前流式状态
    currentTaskSteps.value = []
    streamingContent.value = ''
    currentNotice.value = ''
    currentTaskUserContent.value = ''
    scrollTaskToBottom()
    return
  }

  if (evt.type === 'error') {
    currentTaskSteps.value.push({ type: 'error', error: evt.error || '未知错误' })
    taskStreaming.value = false
    store.setStreaming(false)
    scrollTaskToBottom()
    return
  }

  // thinking / tool_call / tool_result / bash_output 等
  const step: TaskStep = {
    type: evt.type as TaskStep['type'],
    content: evt.content,
    tool_name: evt.tool_name,
    tool_args: evt.tool_args,
    tool_result: evt.tool_result,
    confirm_id: evt.confirm_id,
    cmd: evt.cmd,
    error: evt.error,
  }
  currentTaskSteps.value.push(step)
  scrollTaskToBottom()
}

function isTaskAtBottom(): boolean {
  const el = taskListRef.value
  if (!el) return true
  return isNearBottom(el.scrollHeight, el.scrollTop, el.clientHeight)
}

function scrollTaskToBottom(force = false) {
  if (!shouldAutoScroll(force, taskUserScrolled)) return
  nextTick(() => {
    if (taskListRef.value) {
      taskListRef.value.scrollTop = taskListRef.value.scrollHeight
    }
  })
}

function onTaskScroll() {
  if (!taskStreaming.value) return
  if (isTaskAtBottom()) { taskUserScrolled = false; return }
  taskUserScrolled = true
  if (taskScrollTimer) clearTimeout(taskScrollTimer)
  taskScrollTimer = setTimeout(() => { taskScrollTimer = null }, 100)
}

onMounted(() => {
  EventsOn('task:step', onTaskStep)
})
onUnmounted(() => {
  EventsOff('task:step')
  if (taskScrollTimer) clearTimeout(taskScrollTimer)
})
</script>

<template>
  <div v-if="store.currentConvId" class="chat-view">
    <div class="chat-header">
      <div class="chat-header-info">
        <span class="chat-header-title">{{ store.conversations.find(c => c.id === store.currentConvId)?.title || '对话' }}</span>
        <span class="chat-header-model" v-if="store.conversations.find(c => c.id === store.currentConvId) as any">
          {{ store.providerMap[(store.conversations.find(c => c.id === store.currentConvId) as any)?.provider] || (store.conversations.find(c => c.id === store.currentConvId) as any)?.provider }} · {{ (store.conversations.find(c => c.id === store.currentConvId) as any)?.model }}
        </span>
        <!-- task 模式标签 -->
        <span v-if="isTaskMode" class="chat-header-badge">
          <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>
          任务模式
        </span>
      </div>
    </div>

    <!-- task 模式消息区 -->
    <div v-if="isTaskMode" ref="taskListRef" class="task-list" @scroll.passive="onTaskScroll">

      <!-- 跨会话恢复：DB 历史始终显示；当前会话新完成轮次追加在其后，避免结束后历史区消失 -->
      <template v-if="showTaskHistory">
        <template v-for="msg in taskHistoryMsgs" :key="msg.id">
          <TaskMessageItem
            :role="msg.role as 'user' | 'assistant'"
            :user-content="msg.role === 'user' ? msg.content : undefined"
            :steps="msg.role === 'assistant' ? [{ type: 'content', content: msg.content }] : []"
            :artifacts-json="(msg as any).artifacts"
            :attachments-meta="msg.role === 'user' ? (msg as any).attachments : undefined"
            :is-history="true"
          />
        </template>
      </template>

      <!-- 当前会话已完成的轮次（保留推理链） -->
      <template v-for="(round, i) in completedRounds" :key="i">
        <TaskMessageItem
          role="user"
          :user-content="round.userContent"
          :attachments-meta="round.attachmentsMeta"
          :steps="[]"
        />
        <TaskMessageItem
          role="assistant"
          :steps="round.steps"
          :streaming-content="round.assistantContent"
          :notice="round.notice"
          :is-history="false"
        />
      </template>

      <!-- 上下文分割线：清除上下文激活且有历史内容时显示 -->
      <div v-if="store.taskCutoffActive && (completedRounds.length > 0 || taskHistoryMsgs.length > 0)" class="task-ctx-divider">
        <span class="task-ctx-divider-line" />
        <span class="task-ctx-divider-label">上下文从此处清除</span>
        <span class="task-ctx-divider-line" />
      </div>

      <!-- 当前流式轮次 -->
      <template v-if="(taskStreaming || currentTaskSteps.length > 0 || streamingContent) && currentTaskUserContent">
        <TaskMessageItem
          role="user"
          :user-content="currentTaskUserContent"
          :attachments-meta="currentTaskAttachmentsMeta"
          :steps="[]"
        />
        <TaskMessageItem
          role="assistant"
          :steps="currentTaskSteps"
          :streaming-content="streamingContent"
          :notice="currentNotice"
          :streaming="taskStreaming"
        />
      </template>

      <!-- 空状态 -->
      <div v-if="taskHistoryMsgs.length === 0 && completedRounds.length === 0 && !taskStreaming" class="task-empty">
        <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>
        <p>任务模式：Agent 将自主规划并执行多步骤任务</p>
      </div>
    </div>

    <!-- 普通模式消息区 -->
    <MessageList v-else />

    <InputArea />
  </div>
  <div v-else class="chat-view empty">
    <div class="welcome">
      <div class="welcome-icon">
        <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>
      </div>
      <h1 class="welcome-title">Light</h1>
      <p class="welcome-desc">选择一个对话或新建一个开始聊天</p>
      <div class="welcome-shortcuts">
        <kbd>Enter</kbd><span>发送</span>
        <kbd>Shift + Enter</kbd><span>换行</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.chat-view {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.chat-header {
  padding: var(--space-3) var(--space-6);
  border-bottom: 1px solid var(--color-border);
  background: var(--color-paper);
}

.chat-header-info {
  display: flex;
  align-items: center;
  gap: var(--space-3);
}

.chat-header-title {
  font-size: var(--text-sm);
  font-weight: 600;
}

.chat-header-model {
  font-size: var(--text-xs);
  color: var(--color-text-3);
  padding: 2px var(--space-2);
  background: var(--color-paper-3);
  border-radius: var(--radius-full);
}

.chat-header-badge {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  font-size: var(--text-xs);
  color: oklch(0.45 0.18 280);
  background: oklch(0.94 0.04 280);
  border-radius: var(--radius-full);
  padding: 2px var(--space-2);
  font-weight: 500;
}

/* task 消息列表 */
.task-list {
  flex: 1;
  overflow-y: auto;
  padding: var(--space-2) 0;
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.task-empty {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--space-3);
  color: var(--color-text-3);
  font-size: var(--text-sm);
  text-align: center;
  padding: var(--space-10) 0;
}

.task-ctx-divider {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-2) 0;
  user-select: none;
}
.task-ctx-divider-line {
  flex: 1;
  height: 1px;
  background: linear-gradient(90deg, transparent, oklch(0.65 0.15 25 / 0.4), transparent);
}
.task-ctx-divider-label {
  font-size: 10px;
  font-weight: 500;
  color: oklch(0.65 0.15 25 / 0.7);
  white-space: nowrap;
  letter-spacing: 0.05em;
}

/* Empty state */
.chat-view.empty {
  align-items: center;
  justify-content: center;
}

.welcome {
  text-align: center;
  max-width: 320px;
}

.welcome-icon {
  width: 64px;
  height: 64px;
  margin: 0 auto var(--space-5);
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-xl);
  background: var(--color-paper-3);
  color: var(--color-text-3);
}

.welcome-title {
  font-size: var(--text-2xl);
  font-weight: 700;
  letter-spacing: -0.03em;
  margin-bottom: var(--space-2);
}

.welcome-desc {
  color: var(--color-text-3);
  font-size: var(--text-sm);
  margin-bottom: var(--space-6);
}

.welcome-shortcuts {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--space-2);
  font-size: var(--text-xs);
  color: var(--color-text-3);
}

.welcome-shortcuts kbd {
  padding: 2px 6px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  font-family: var(--font-mono);
  font-size: 11px;
  background: var(--color-paper-2);
}
</style>
