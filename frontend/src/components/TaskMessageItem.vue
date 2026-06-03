<script setup lang="ts">
import { ref, computed } from 'vue'
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
  streamingContent?: string
}>()

const userHtml = computed(() => {
  if (!props.userContent) return ''
  return marked(props.userContent) as string
})

const finalContent = computed(() =>
  props.streamingContent !== undefined
    ? props.streamingContent
    : props.steps.filter(s => s.type === 'content').map(s => s.content || '').join('')
)

// 流式过程中直接显示纯文本，完成后才渲染 markdown，避免不完整语法导致渲染跳变
const finalHtml = computed(() => {
  if (!finalContent.value) return ''
  if (props.streaming) {
    // 流式中：纯文本，换行转 <br>
    return finalContent.value
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/\n/g, '<br>')
  }
  return marked(finalContent.value) as string
})

// 推理链：非 content/done，合并连续同类 delta
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

// 推理链整体折叠状态（默认折叠）
const chainOpen = ref(false)

// 推理链步骤摘要（用于折叠时显示）
const chainSummary = computed(() => {
  const toolCalls = chainSteps.value.filter(s => s.type === 'tool_call')
  const hasThinking = chainSteps.value.some(s => s.type === 'thinking')
  const parts: string[] = []
  if (hasThinking) parts.push('思考')
  if (toolCalls.length > 0) {
    const names = [...new Set(toolCalls.map(s => s.tool_name).filter(Boolean))]
    parts.push(names.join('、'))
  }
  return parts.join(' · ') || '处理过程'
})

function formatArgs(args?: string) {
  if (!args) return ''
  try { return JSON.stringify(JSON.parse(args), null, 2) } catch { return args }
}

function truncate(s: string, max = 800) {
  return s.length <= max ? s : s.slice(0, max) + '\n…（已截断）'
}

// 解析 tool_result，检测是否为 write_file 的结构化结果
interface WriteFileResult {
  ok: boolean
  path: string
  abs_path: string
  bytes: number
  preview: string
  truncated: boolean
}

function parseWriteFileResult(result?: string): WriteFileResult | null {
  if (!result) return null
  try {
    const obj = JSON.parse(result)
    if (obj.ok && obj.path && obj.preview !== undefined) return obj as WriteFileResult
  } catch {}
  return null
}

// 根据文件扩展名判断是否为 markdown
function isMarkdown(path: string) {
  return /\.(md|markdown)$/i.test(path)
}

// 文件预览 html
function filePreviewHtml(r: WriteFileResult): string {
  if (isMarkdown(r.path)) {
    return marked(r.preview) as string
  }
  // 代码文件：用 highlight.js
  const ext = r.path.split('.').pop() || ''
  const lang = hljs.getLanguage(ext) ? ext : ''
  const highlighted = lang
    ? hljs.highlight(r.preview, { language: lang }).value
    : hljs.highlightAuto(r.preview).value
  return `<pre><code>${highlighted}</code></pre>`
}

function toolIcon(name?: string) {
  if (!name) return '⚙'
  if (name === 'bash_exec') return '💻'
  if (name === 'read_file') return '📖'
  if (name === 'write_file') return '✏️'
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
    <div v-if="isHistory" class="task-answer__bubble markdown-body" v-html="finalHtml" />

    <!-- 流式模式 -->
    <template v-else>

      <!-- 推理链：整体折叠 -->
      <div v-if="chainSteps.length" class="task-chain">
        <!-- 折叠头部 -->
        <button class="chain-header" @click="chainOpen = !chainOpen">
          <span class="chain-header__left">
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20z"/><path d="M12 8v4l3 3"/></svg>
            <span class="chain-header__label">{{ chainSummary }}</span>
          </span>
          <svg class="chain-header__chevron" :class="{ open: chainOpen }" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><polyline points="6 9 12 15 18 9"/></svg>
        </button>

        <!-- 展开内容：所有步骤平铺 -->
        <div v-if="chainOpen" class="chain-body">
          <template v-for="(step, i) in chainSteps" :key="i">

            <!-- thinking -->
            <div v-if="step.type === 'thinking'" class="chain-step chain-step--think">
              <span class="chain-step__icon">💭</span>
              <span class="chain-step__text">{{ step.content }}</span>
            </div>

            <!-- tool_call -->
            <div v-else-if="step.type === 'tool_call'" class="chain-step chain-step--tool">
              <span class="chain-step__icon">{{ toolIcon(step.tool_name) }}</span>
              <span class="chain-step__tool-name">{{ step.tool_name }}</span>
              <pre v-if="step.tool_args" class="chain-step__code">{{ formatArgs(step.tool_args) }}</pre>
            </div>

            <!-- tool_result write_file -->
            <div v-else-if="step.type === 'tool_result' && parseWriteFileResult(step.tool_result)" class="chain-step chain-step--file">
              <span class="chain-step__icon">✏️</span>
              <span class="chain-step__tool-name">{{ parseWriteFileResult(step.tool_result)!.path }}</span>
              <span class="chain-step__file-size">{{ parseWriteFileResult(step.tool_result)!.bytes }} B</span>
              <div class="chain-step__file-preview markdown-body"
                v-html="filePreviewHtml(parseWriteFileResult(step.tool_result)!)" />
            </div>

            <!-- tool_result 普通 -->
            <div v-else-if="step.type === 'tool_result'" class="chain-step chain-step--result">
              <span class="chain-step__icon chain-step__icon--result">↳</span>
              <pre class="chain-step__result">{{ truncate(step.tool_result || '') }}</pre>
            </div>

            <!-- bash_output -->
            <div v-else-if="step.type === 'bash_output'" class="chain-step chain-step--bash">
              <pre class="chain-step__bash">{{ step.content }}</pre>
            </div>

            <!-- error -->
            <div v-else-if="step.type === 'error'" class="chain-step chain-step--error">
              <span class="chain-step__icon">✗</span>
              <span>{{ step.error }}</span>
            </div>

          </template>
        </div>
      </div>

      <!-- 流式内容 / 最终回答 -->
      <div v-if="finalContent || streaming" class="task-answer">
        <div class="task-answer__bubble markdown-body" v-html="finalHtml" />
        <span v-if="streaming" class="task-cursor">▋</span>
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

/* ── 推理链整体折叠 ── */
.task-chain {
  width: 100%;
  max-width: 760px;
  border: 1px solid var(--color-border);
  border-radius: 8px;
  overflow: hidden;
  margin-bottom: 8px;
  font-size: 12.5px;
}

.chain-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  width: 100%;
  padding: 7px 12px;
  background: var(--color-paper-2);
  border: none;
  cursor: pointer;
  text-align: left;
  transition: background var(--duration-fast);
}
.chain-header:hover { background: var(--color-paper-3); }

.chain-header__left {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--color-text-2);
  font-size: 12px;
  font-weight: 500;
}

.chain-header__label { color: var(--color-text-2); }

.chain-header__chevron {
  opacity: 0.4;
  transition: transform 0.15s;
  flex-shrink: 0;
}
.chain-header__chevron.open { transform: rotate(180deg); }

.chain-body {
  border-top: 1px solid var(--color-border);
  display: flex;
  flex-direction: column;
  gap: 0;
}

/* 每个步骤 */
.chain-step {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 6px 12px;
  border-bottom: 1px solid oklch(0 0 0 / 0.04);
  font-family: 'JetBrains Mono', 'Fira Code', ui-monospace, monospace;
  font-size: 12px;
  line-height: 1.6;
}
.chain-step:last-child { border-bottom: none; }

.chain-step--think {
  background: oklch(0.975 0.008 280);
  font-family: inherit;
  font-size: 12px;
  color: var(--color-text-3);
}
.chain-step--tool { background: oklch(0.975 0.008 220); }
.chain-step--file { background: oklch(0.975 0.008 160); flex-direction: column; gap: 4px; }
.chain-step--result { background: oklch(0.975 0.008 160); }
.chain-step--bash { background: oklch(0.10 0 0); padding: 8px 12px; }
.chain-step--error { background: oklch(0.975 0.02 20); color: oklch(0.45 0.15 20); }

.chain-step__icon {
  flex-shrink: 0;
  font-size: 13px;
  line-height: 1.6;
}
.chain-step__icon--result { color: oklch(0.55 0.1 160); font-weight: 600; }

.chain-step__tool-name {
  font-weight: 600;
  color: oklch(0.38 0.1 220);
  flex-shrink: 0;
}

.chain-step__file-size {
  font-size: 10.5px;
  opacity: 0.45;
  flex-shrink: 0;
}

.chain-step__text {
  white-space: pre-wrap;
  word-break: break-word;
  flex: 1;
  color: var(--color-text-3);
  font-family: inherit;
  font-size: 12px;
}

.chain-step__code {
  margin: 4px 0 0 0;
  padding: 0;
  white-space: pre-wrap;
  word-break: break-all;
  color: oklch(0.35 0.08 220);
  max-height: 200px;
  overflow-y: auto;
  flex: 1;
  font-size: 11.5px;
}

.chain-step__result {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-all;
  color: oklch(0.35 0.08 160);
  max-height: 200px;
  overflow-y: auto;
  flex: 1;
  font-size: 11.5px;
}

.chain-step__bash {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-all;
  color: oklch(0.82 0.06 150);
  max-height: 280px;
  overflow-y: auto;
  font-size: 11.5px;
  width: 100%;
}

.chain-step__file-preview {
  width: 100%;
  max-height: 360px;
  overflow-y: auto;
  font-family: inherit;
  font-size: 13px;
  padding: 4px 0;
}

/* ── 最终回答 ── */
.task-answer { max-width: 760px; width: 100%; }
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
@keyframes blink { 50% { opacity: 0; } }
</style>
