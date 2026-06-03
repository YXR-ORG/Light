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

const finalHtml = computed(() => {
  if (!finalContent.value) return ''
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

// 折叠状态：默认折叠
const thinkingOpen = ref(false)
const toolOpen = ref<Record<number, boolean>>({})

function toggleThinking() { thinkingOpen.value = !thinkingOpen.value }
function toggleTool(i: number) { toolOpen.value[i] = !toolOpen.value[i] }

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

      <!-- 推理链 -->
      <div v-if="chainSteps.length" class="task-chain">
        <template v-for="(step, i) in chainSteps" :key="i">

          <!-- thinking：默认折叠 -->
          <div v-if="step.type === 'thinking'" class="chain-card chain-card--thinking">
            <button class="chain-card__header" @click="toggleThinking">
              <span class="chain-card__title">
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="12" cy="12" r="10"/><path d="M12 16v-4M12 8h.01"/></svg>
                思考过程
              </span>
              <svg class="chain-card__chevron" :class="{ open: thinkingOpen }" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><polyline points="6 9 12 15 18 9"/></svg>
            </button>
            <div v-if="thinkingOpen" class="chain-card__body chain-card__body--pre">{{ step.content }}</div>
          </div>

          <!-- tool_call：默认折叠 -->
          <div v-else-if="step.type === 'tool_call'" class="chain-card chain-card--tool">
            <button class="chain-card__header" @click="toggleTool(i)">
              <span class="chain-card__title">
                <span class="chain-card__icon">{{ toolIcon(step.tool_name) }}</span>
                <span class="chain-card__tool-name">{{ step.tool_name }}</span>
              </span>
              <svg class="chain-card__chevron" :class="{ open: toolOpen[i] }" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><polyline points="6 9 12 15 18 9"/></svg>
            </button>
            <div v-if="toolOpen[i]" class="chain-card__body">
              <pre class="chain-card__code">{{ formatArgs(step.tool_args) }}</pre>
            </div>
          </div>

          <!-- tool_result：默认折叠，与上一个 tool_call 关联 -->
          <div v-else-if="step.type === 'tool_result'" class="chain-card chain-card--result">
            <template v-if="parseWriteFileResult(step.tool_result)">
              <!-- write_file 结构化结果：文件预览卡片 -->
              <button class="chain-card__header chain-card__header--result" @click="toggleTool(i)">
                <span class="chain-card__title chain-card__title--result">
                  <span>✏️</span>
                  <span class="chain-card__tool-name">{{ parseWriteFileResult(step.tool_result)!.path }}</span>
                  <span class="chain-card__file-size">{{ parseWriteFileResult(step.tool_result)!.bytes }} B</span>
                </span>
                <svg class="chain-card__chevron" :class="{ open: toolOpen[i] }" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><polyline points="6 9 12 15 18 9"/></svg>
              </button>
              <div v-if="toolOpen[i]" class="chain-card__body chain-card__file-preview">
                <div class="chain-card__file-preview-inner"
                  v-html="filePreviewHtml(parseWriteFileResult(step.tool_result)!)" />
                <div v-if="parseWriteFileResult(step.tool_result)!.truncated" class="chain-card__truncated">
                  …内容已截断（仅显示前 2000 字符）
                </div>
              </div>
            </template>
            <template v-else>
              <!-- 普通 tool_result -->
              <button class="chain-card__header chain-card__header--result" @click="toggleTool(i)">
                <span class="chain-card__title chain-card__title--result">
                  <span>↳ 结果</span>
                  <span class="chain-card__tool-label">{{ step.tool_name }}</span>
                </span>
                <svg class="chain-card__chevron" :class="{ open: toolOpen[i] }" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><polyline points="6 9 12 15 18 9"/></svg>
              </button>
              <div v-if="toolOpen[i]" class="chain-card__body">
                <pre class="chain-card__result-body">{{ truncate(step.tool_result || '') }}</pre>
              </div>
            </template>
          </div>

          <!-- bash_output -->
          <div v-else-if="step.type === 'bash_output'" class="chain-card chain-card--bash">
            <pre class="chain-card__bash">{{ step.content }}</pre>
          </div>

          <!-- error -->
          <div v-else-if="step.type === 'error'" class="chain-card chain-card--error">
            <span class="chain-card__error-icon">✗</span>
            <span>{{ step.error }}</span>
          </div>

        </template>
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

/* ── 推理链 ── */
.task-chain {
  width: 100%;
  max-width: 760px;
  display: flex;
  flex-direction: column;
  gap: 3px;
  margin-bottom: 8px;
}

.chain-card {
  border-radius: 6px;
  border: 1px solid var(--color-border);
  overflow: hidden;
  font-size: 12.5px;
}

.chain-card--thinking {
  border-color: oklch(0.84 0.04 280);
  background: oklch(0.975 0.008 280);
}
.chain-card--tool {
  border-color: oklch(0.84 0.06 220);
  background: oklch(0.975 0.008 220);
}
.chain-card--result {
  border-color: oklch(0.84 0.05 160);
  background: oklch(0.975 0.008 160);
}
.chain-card--bash {
  background: oklch(0.10 0 0);
  border-color: oklch(0.22 0 0);
  padding: 8px 10px;
}
.chain-card--error {
  border-color: oklch(0.78 0.12 20);
  background: oklch(0.975 0.02 20);
  padding: 7px 10px;
  display: flex;
  align-items: center;
  gap: 6px;
  color: oklch(0.45 0.15 20);
}

.chain-card__header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  width: 100%;
  padding: 6px 10px;
  background: none;
  border: none;
  cursor: pointer;
  text-align: left;
  color: inherit;
  transition: background var(--duration-fast);
}
.chain-card__header:hover { background: oklch(0 0 0 / 0.03); }
.chain-card__header--result { padding: 5px 10px; }

.chain-card__title {
  display: flex;
  align-items: center;
  gap: 5px;
  font-size: 12px;
  font-weight: 500;
  color: var(--color-text-2);
}
.chain-card__title--result { color: oklch(0.45 0.08 160); }

.chain-card__icon { font-size: 13px; }
.chain-card__tool-name { font-family: var(--font-mono); font-size: 11.5px; color: oklch(0.38 0.1 220); }
.chain-card__tool-label { font-size: 10.5px; opacity: 0.5; font-family: var(--font-mono); }
.chain-card__error-icon { font-size: 13px; }

.chain-card__chevron {
  opacity: 0.4;
  transition: transform 0.15s;
  flex-shrink: 0;
}
.chain-card__chevron.open { transform: rotate(180deg); }

.chain-card__body {
  padding: 6px 10px 8px;
  border-top: 1px solid oklch(0 0 0 / 0.06);
}
.chain-card__body--pre,
.chain-card__code,
.chain-card__result-body {
  font-family: 'JetBrains Mono', 'Fira Code', ui-monospace, monospace;
  font-size: 11.5px;
  white-space: pre-wrap;
  word-break: break-all;
  margin: 0;
  max-height: 280px;
  overflow-y: auto;
  color: var(--color-text-2);
  line-height: 1.6;
}
.chain-card__bash {
  font-family: 'JetBrains Mono', 'Fira Code', ui-monospace, monospace;
  font-size: 11.5px;
  white-space: pre-wrap;
  word-break: break-all;
  margin: 0;
  max-height: 280px;
  overflow-y: auto;
  color: oklch(0.82 0.06 150);
  line-height: 1.6;
}

/* ── 最终回答 ── */
.task-answer { max-width: 760px; width: 100%; }
.task-answer__bubble {
  background: var(--color-paper-2);
  border-radius: 4px 16px 16px 16px;
  padding: 10px 14px;
  word-break: break-word;
}

.chain-card__file-size {
  font-size: 10.5px;
  opacity: 0.45;
  font-family: var(--font-mono);
}

.chain-card__file-preview {
  padding: 0;
}

.chain-card__file-preview-inner {
  padding: 10px 12px;
  max-height: 400px;
  overflow-y: auto;
  font-size: 13px;
  line-height: 1.6;
}

.chain-card__file-preview-inner pre {
  margin: 0;
  font-size: 12px;
  font-family: 'JetBrains Mono', 'Fira Code', ui-monospace, monospace;
  white-space: pre-wrap;
  word-break: break-all;
}

.chain-card__truncated {
  padding: 4px 12px 6px;
  font-size: 11px;
  color: var(--color-text-3);
  border-top: 1px solid oklch(0 0 0 / 0.06);
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
