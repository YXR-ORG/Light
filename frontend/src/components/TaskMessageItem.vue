<script setup lang="ts">
import { computed } from 'vue'
import { marked } from 'marked'
import hljs from 'highlight.js'

marked.setOptions({
  highlight(code: string, lang: string) {
    if (lang && hljs.getLanguage(lang)) {
      return hljs.highlight(code, { language: lang }).value
    }
    return hljs.highlightAuto(code).value
  },
  breaks: true,
  gfm: true,
} as any)

export interface TaskStep {
  type: 'thinking' | 'tool_call' | 'tool_result' | 'bash_confirm' | 'bash_output' | 'content' | 'done' | 'error'
  content?: string
  tool_name?: string
  tool_args?: string
  tool_result?: string
  confirm_id?: string
  cmd?: string
  error?: string
}

const props = defineProps<{
  role: 'user' | 'assistant'
  userContent?: string
  steps: TaskStep[]
  streaming?: boolean
  isHistory?: boolean
}>()

const userHtml = computed(() => {
  if (!props.userContent) return ''
  return marked(props.userContent) as string
})

const finalContent = computed(() =>
  props.steps.filter(s => s.type === 'content').map(s => s.content || '').join('')
)

const finalHtml = computed(() => {
  if (!finalContent.value) return ''
  return marked(finalContent.value) as string
})

// 推理链：非 content/done 的步骤，合并连续同类 delta
const chainSteps = computed(() => {
  const raw = props.steps.filter(s => s.type !== 'content' && s.type !== 'done')
  if (!raw.length) return []
  const merged: TaskStep[] = []
  for (const step of raw) {
    const last = merged[merged.length - 1]
    if (last && last.type === step.type && ['thinking', 'bash_output'].includes(step.type)) {
      last.content = (last.content || '') + (step.content || '')
    } else {
      merged.push({ ...step })
    }
  }
  return merged
})

function formatArgs(args?: string) {
  if (!args) return ''
  try { return JSON.stringify(JSON.parse(args), null, 2) } catch { return args }
}

function truncate(s: string, max = 800) {
  return s.length <= max ? s : s.slice(0, max) + '\n…（已截断）'
}

function toolIcon(name?: string) {
  if (!name) return '⚙'
  if (name === 'bash_exec') return '>'
  if (name === 'read_file') return '📖'
  if (name === 'write_file') return '✏'
  if (name === 'list_dir') return '📂'
  if (name === 'make_dir') return '📁'
  if (name?.startsWith('search_')) return '🔍'
  if (name === 'web_search') return '🌐'
  return '⚙'
}
</script>

<template>
  <!-- 用户消息 -->
  <div v-if="role === 'user'" class="task-msg task-msg--user">
    <div class="task-msg__bubble" v-html="userHtml" />
  </div>

  <!-- AI task 消息 -->
  <div v-else class="task-msg task-msg--assistant">

    <!-- 历史模式：直接渲染 markdown -->
    <div v-if="isHistory" class="task-msg__bubble markdown-body" v-html="finalHtml" />

    <!-- 流式/终端模式：平铺所有步骤 -->
    <template v-else>

      <!-- 推理链：终端平铺 -->
      <div v-if="chainSteps.length" class="task-terminal">
        <template v-for="(step, i) in chainSteps" :key="i">

          <!-- thinking -->
          <div v-if="step.type === 'thinking'" class="term-block term-block--think">
            <span class="term-prefix">💭</span>
            <span class="term-text">{{ step.content }}</span>
          </div>

          <!-- tool_call -->
          <div v-else-if="step.type === 'tool_call'" class="term-block term-block--tool">
            <span class="term-prefix">{{ toolIcon(step.tool_name) }} <span class="term-tool-name">{{ step.tool_name }}</span></span>
            <pre v-if="step.tool_args" class="term-code">{{ formatArgs(step.tool_args) }}</pre>
          </div>

          <!-- tool_result -->
          <div v-else-if="step.type === 'tool_result'" class="term-block term-block--result">
            <span class="term-prefix term-prefix--result">↳</span>
            <pre class="term-result">{{ truncate(step.tool_result || '') }}</pre>
          </div>

          <!-- bash_output -->
          <div v-else-if="step.type === 'bash_output'" class="term-block term-block--bash">
            <pre class="term-bash">{{ step.content }}</pre>
          </div>

          <!-- error -->
          <div v-else-if="step.type === 'error'" class="term-block term-block--error">
            <span class="term-prefix">✗</span>
            <span>{{ step.error }}</span>
          </div>

        </template>
      </div>

      <!-- 最终回答 -->
      <div v-if="finalContent || streaming" class="task-answer">
        <div class="task-answer__bubble markdown-body" v-html="finalHtml" />
        <span v-if="streaming && !finalContent" class="task-cursor">▋</span>
        <span v-if="streaming && finalContent" class="task-cursor task-cursor--inline">▋</span>
      </div>

    </template>
  </div>
</template>

<style scoped>
.task-msg {
  display: flex;
  flex-direction: column;
  margin: 6px 0;
}

/* 用户气泡 */
.task-msg--user { align-items: flex-end; }
.task-msg--user .task-msg__bubble {
  background: var(--color-accent);
  color: #fff;
  border-radius: 16px 16px 4px 16px;
  padding: 8px 14px;
  max-width: 72%;
  word-break: break-word;
  font-size: var(--text-sm);
}

/* AI 消息 */
.task-msg--assistant { align-items: flex-start; width: 100%; }

/* 历史模式：简洁 markdown bubble */
.task-msg--assistant > .task-msg__bubble {
  background: var(--color-paper-2);
  border-radius: 4px 16px 16px 16px;
  padding: 10px 14px;
  max-width: 760px;
  word-break: break-word;
}

/* ── 终端区域 ── */
.task-terminal {
  width: 100%;
  max-width: 760px;
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin-bottom: 6px;
  font-family: 'JetBrains Mono', 'Fira Code', ui-monospace, monospace;
  font-size: 12px;
}

.term-block {
  display: flex;
  flex-direction: column;
  padding: 4px 8px;
  border-radius: 4px;
  line-height: 1.6;
}

.term-block--think {
  color: var(--color-text-3);
  background: transparent;
  flex-direction: row;
  gap: 6px;
  align-items: flex-start;
}

.term-block--tool {
  background: oklch(0.96 0.01 220);
  border-left: 2px solid oklch(0.75 0.1 220);
}

.term-block--result {
  background: oklch(0.97 0.01 160);
  border-left: 2px solid oklch(0.75 0.08 160);
  flex-direction: row;
  gap: 8px;
  align-items: flex-start;
}

.term-block--bash {
  background: oklch(0.10 0 0);
  border-radius: 4px;
}

.term-block--error {
  background: oklch(0.97 0.03 20);
  border-left: 2px solid oklch(0.7 0.15 20);
  color: oklch(0.45 0.15 20);
  flex-direction: row;
  gap: 6px;
  align-items: center;
}

.term-prefix {
  font-size: 11px;
  opacity: 0.7;
  white-space: nowrap;
  flex-shrink: 0;
}

.term-prefix--result {
  color: oklch(0.55 0.1 160);
  font-weight: 600;
}

.term-tool-name {
  font-weight: 600;
  color: oklch(0.4 0.1 220);
}

.term-code {
  margin: 4px 0 0 0;
  padding: 0;
  white-space: pre-wrap;
  word-break: break-all;
  color: oklch(0.35 0.08 220);
  max-height: 200px;
  overflow-y: auto;
}

.term-result {
  margin: 0;
  padding: 0;
  white-space: pre-wrap;
  word-break: break-all;
  color: oklch(0.35 0.08 160);
  max-height: 200px;
  overflow-y: auto;
  flex: 1;
}

.term-bash {
  margin: 0;
  padding: 0;
  white-space: pre-wrap;
  word-break: break-all;
  color: oklch(0.85 0.05 150);
  max-height: 300px;
  overflow-y: auto;
}

.term-text {
  color: var(--color-text-3);
  white-space: pre-wrap;
  word-break: break-word;
  flex: 1;
  font-family: inherit;
}

/* ── 最终回答 ── */
.task-answer {
  max-width: 760px;
  width: 100%;
}

.task-answer__bubble {
  background: var(--color-paper-2);
  border-radius: 4px 16px 16px 16px;
  padding: 10px 14px;
  word-break: break-word;
}

/* 光标 */
.task-cursor {
  display: inline-block;
  animation: blink 1s step-end infinite;
  margin-left: 2px;
  font-family: monospace;
}
.task-cursor--inline { vertical-align: middle; }

@keyframes blink { 50% { opacity: 0; } }
</style>
