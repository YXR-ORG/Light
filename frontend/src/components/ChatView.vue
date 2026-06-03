<script setup lang="ts">
import { useChatStore } from '../stores/chat'
import MessageList from './MessageList.vue'
import TaskMessageItem, { type TaskStep } from './TaskMessageItem.vue'
import InputArea from './InputArea.vue'
import { ref, watch, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import { GetMessages } from '../../wailsjs/go/handler/ConversationHandler'
import type { storage } from '../../wailsjs/go/models'

const store = useChatStore()

// ─── task 模式状态 ────────────────────────────────────────────────
const taskListRef = ref<HTMLElement | null>(null)

// 历史 task 消息（从 DB 加载）
const taskHistoryMsgs = ref<storage.Message[]>([])

// 当前流式 task 步骤
const currentTaskSteps = ref<TaskStep[]>([])
// 当前流式 task 对应的用户消息内容
const currentTaskUserContent = ref('')
// 是否正在流式输出 task
const taskStreaming = ref(false)

// 当前会话是否为 task 模式
const isTaskMode = computed(() => {
  const conv = store.conversations.find(c => c.id === store.currentConvId) as any
  return conv?.mode === 'task'
})

// 加载 task 历史消息
async function loadTaskHistory() {
  if (!store.currentConvId) return
  try {
    taskHistoryMsgs.value = await GetMessages(store.currentConvId)
  } catch {
    taskHistoryMsgs.value = []
  }
}

// 监听会话切换
watch(() => store.currentConvId, async () => {
  currentTaskSteps.value = []
  currentTaskUserContent.value = ''
  taskStreaming.value = false
  if (isTaskMode.value) await loadTaskHistory()
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
}

// content 步骤用独立的 ref 累加，避免每个 token 都触发 chainSteps computed 重算
const streamingContent = ref('')

function onTaskStep(evt: TaskStepEvent) {
  if (evt.type === 'user_msg') {
    currentTaskSteps.value = []
    streamingContent.value = ''
    currentTaskUserContent.value = evt.user_content || evt.content || ''
    taskStreaming.value = true
    store.setStreaming(true)
    scrollTaskToBottom()
    return
  }

  if (evt.type === 'content') {
    streamingContent.value += evt.content || ''
    scrollTaskToBottom()
    return
  }

  if (evt.type === 'done') {
    taskStreaming.value = false
    store.setStreaming(false)
    loadTaskHistory().then(() => {
      currentTaskSteps.value = []
      streamingContent.value = ''
      currentTaskUserContent.value = ''
      scrollTaskToBottom()
    })
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

function scrollTaskToBottom() {
  nextTick(() => {
    if (taskListRef.value) {
      taskListRef.value.scrollTop = taskListRef.value.scrollHeight
    }
  })
}

onMounted(() => {
  EventsOn('task:step', onTaskStep)
})
onUnmounted(() => {
  EventsOff('task:step')
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
    <div v-if="isTaskMode" ref="taskListRef" class="task-list">
      <!-- 历史消息 -->
      <template v-for="msg in taskHistoryMsgs" :key="msg.id">
        <TaskMessageItem
          :role="msg.role as 'user' | 'assistant'"
          :user-content="msg.role === 'user' ? msg.content : undefined"
          :steps="msg.role === 'assistant' ? [{ type: 'content', content: msg.content }] : []"
          :is-history="true"
        />
      </template>

      <!-- 当前流式轮次（仅在流式进行中或步骤未被历史替换前显示） -->
      <template v-if="(taskStreaming || currentTaskSteps.length > 0 || streamingContent) && currentTaskUserContent">
        <!-- 用户消息 -->
        <TaskMessageItem
          role="user"
          :user-content="currentTaskUserContent"
          :steps="[]"
        />
        <!-- AI 步骤 -->
        <TaskMessageItem
          role="assistant"
          :steps="currentTaskSteps"
          :streaming-content="streamingContent"
          :streaming="taskStreaming"
        />
      </template>

      <!-- 空状态 -->
      <div v-if="taskHistoryMsgs.length === 0 && !taskStreaming" class="task-empty">
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
  padding: var(--space-5) var(--space-6);
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
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
