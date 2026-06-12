<script setup lang="ts">
import { ref, computed } from 'vue'
import { marked } from 'marked'
import hljs from 'highlight.js'
import { collectArtifacts, splitTaskArtifacts, stripArtifacts, type Artifact } from '../utils/artifacts'
import { decorateHtmlPreviewBlocks, getHtmlPreviewSourceFromButton } from '../utils/htmlPreview'
import { shouldShowTaskActions, taskCopyText } from '../utils/taskActions'
import { sanitizeRenderedMarkdown } from '../utils/markdownSafe'
import ArtifactCard from './ArtifactCard.vue'
import HtmlPreviewDialog from './HtmlPreviewDialog.vue'

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
  type: 'thinking' | 'tool_call' | 'tool_result' | 'bash_confirm' | 'bash_output' | 'content' | 'content_note' | 'done' | 'error'
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
  notice?: string
  artifactsJson?: string  // 历史消息的产物 JSON（[]Artifact），优先于从 steps 收集
  attachmentsMeta?: string // 附件元数据 JSON（[]{ name, mime_type }）
  canRegenerate?: boolean
}>()

const emit = defineEmits<{
  regenerate: []
}>()

const userHtml = computed(() => {
  if (!props.userContent) return ''
  return sanitizeRenderedMarkdown(marked(props.userContent) as string)
})

const finalContent = computed(() =>
  props.streamingContent !== undefined
    ? props.streamingContent
    : props.steps.filter(s => s.type === 'content').map(s => s.content || '').join('')
)

// 始终渲染 markdown（与 chat 模式一致），流式中也实时渲染。
// marked 对不完整语法容错良好，下一帧即修正。
const finalHtml = computed(() => {
  if (!finalContent.value) return ''
  return decorateHtmlPreviewBlocks(sanitizeRenderedMarkdown(marked(finalContent.value) as string))
})

const showTaskLoading = computed(() => props.streaming && !finalContent.value && !props.notice)

// 推理链：非 content/done，合并连续同类 delta
const chainSteps = computed(() => {
  const raw = props.steps.filter(s => s.type !== 'content' && s.type !== 'done')
  if (!raw.length) return []
  const merged: TaskStep[] = []
  for (const step of raw) {
    const last = merged[merged.length - 1]
    if (last && last.type === step.type && ['thinking', 'bash_output', 'content_note'].includes(step.type)) {
      last.content = (last.content || '') + (step.content || '')
    } else {
      merged.push({ ...step })
    }
  }
  return merged
})

// 推理链整体折叠状态（默认折叠）
const chainOpen = ref(false)
const copied = ref(false)
const previewHtml = ref('')

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
// 本次任务涉及的所有产物（文件/链接/图片…）。
// 历史消息：优先用持久化的 artifactsJson；实时流式：从所有 tool_result 自动收集。
const artifacts = computed<Artifact[]>(() => {
  if (props.artifactsJson) {
    try {
      const arr = JSON.parse(props.artifactsJson)
      if (Array.isArray(arr)) return arr as Artifact[]
    } catch {}
  }
  return collectArtifacts(props.steps.filter(s => s.type === 'tool_result').map(s => s.tool_result))
})

const taskArtifacts = computed(() => splitTaskArtifacts(artifacts.value))

// plan 产物放回复区顶部（执行导航）；“本次涉及的文件”只展示真正的 file 产物。
const planArtifacts = computed(() => taskArtifacts.value.plans)
const fileArtifacts = computed(() => taskArtifacts.value.files)

interface AttachmentMeta { name: string; mime_type: string }
const attachmentMetas = computed<AttachmentMeta[]>(() => {
  if (!props.attachmentsMeta) return []
  try { return JSON.parse(props.attachmentsMeta) as AttachmentMeta[] } catch { return [] }
})

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

const showActions = computed(() => shouldShowTaskActions(props.role, props.streaming, !!finalContent.value))

async function copyTaskContent() {
  await navigator.clipboard.writeText(taskCopyText(finalContent.value, props.steps)).catch(() => {})
  copied.value = true
  setTimeout(() => { copied.value = false }, 2000)
}

function onMarkdownClick(e: MouseEvent) {
  const target = e.target as HTMLElement
  const button = target.closest('.html-preview-run') as HTMLElement | null
  if (!button) return
  previewHtml.value = getHtmlPreviewSourceFromButton(button)
}
</script>

<template>
  <div class="task-msg-row" :class="{ user: role === 'user', assistant: role === 'assistant' }">
    <div class="task-msg-avatar">{{ role === 'user' ? 'U' : 'AI' }}</div>
    <div class="task-msg-content">
      <div class="task-msg-label">{{ role === 'user' ? '你' : 'AI 助手' }}</div>

      <template v-if="role === 'user'">
        <div v-if="attachmentMetas.length > 0" class="task-msg__attachments">
          <span v-for="(a, i) in attachmentMetas" :key="i" class="task-attachment-chip">
            <svg v-if="a.mime_type.startsWith('image/')" width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="8.5" cy="8.5" r="1.5"/><polyline points="21 15 16 10 5 21"/></svg>
            <svg v-else width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>
            <span>{{ a.name }}</span>
          </span>
        </div>
        <div class="task-user-text" v-html="userHtml" />
      </template>

      <template v-else>
        <template v-if="isHistory">
          <ArtifactCard v-for="(p, i) in planArtifacts" :key="'plan-' + i" :artifact="p" />
          <div class="task-answer__bubble markdown-body" v-html="finalHtml" @click="onMarkdownClick" />
          <div v-if="showActions" class="task-msg-actions">
            <button class="task-msg-action" :class="{ copied }" @click="copyTaskContent" :title="copied ? '已复制' : '复制'">
              <svg v-if="copied" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><polyline points="20 6 9 17 4 12"/></svg>
              <svg v-else width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
            </button>
            <button v-if="canRegenerate !== false" class="task-msg-action" @click="emit('regenerate')" title="重新生成">
              <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><polyline points="1 4 1 10 7 10"/><path d="M3.51 15a9 9 0 1 0 .49-4.5"/></svg>
            </button>
          </div>
          <div v-if="fileArtifacts.length > 0" class="task-files">
            <div class="task-files__title">
              <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>
              本次涉及的文件（{{ fileArtifacts.length }}）
            </div>
            <ArtifactCard v-for="(art, i) in fileArtifacts" :key="i" :artifact="art" />
          </div>
        </template>

        <template v-else>
          <ArtifactCard v-for="(p, i) in planArtifacts" :key="'plan-' + i" :artifact="p" />

          <div v-if="chainSteps.length" class="task-chain">
            <button class="chain-header" @click="chainOpen = !chainOpen">
              <span class="chain-header__left">
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20z"/><path d="M12 8v4l3 3"/></svg>
                <span class="chain-header__label">{{ chainSummary }}</span>
              </span>
              <svg class="chain-header__chevron" :class="{ open: chainOpen }" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><polyline points="6 9 12 15 18 9"/></svg>
            </button>

            <div v-if="chainOpen" class="chain-body">
              <template v-for="(step, i) in chainSteps" :key="i">
                <div v-if="step.type === 'thinking'" class="chain-step chain-step--think">
                  <span class="chain-step__icon">💭</span>
                  <span class="chain-step__text">{{ step.content }}</span>
                </div>
                <div v-else-if="step.type === 'content_note'" class="chain-step chain-step--note">
                  <span class="chain-step__icon">📝</span>
                  <span class="chain-step__text">{{ step.content }}</span>
                </div>
                <div v-else-if="step.type === 'tool_call'" class="chain-step chain-step--tool">
                  <span class="chain-step__icon">{{ toolIcon(step.tool_name) }}</span>
                  <span class="chain-step__tool-name">{{ step.tool_name }}</span>
                  <pre v-if="step.tool_args" class="chain-step__code">{{ formatArgs(step.tool_args) }}</pre>
                </div>
                <div v-else-if="step.type === 'tool_result'" class="chain-step chain-step--result">
                  <span class="chain-step__icon chain-step__icon--result">↳</span>
                  <pre class="chain-step__result">{{ truncate(stripArtifacts(step.tool_result)) }}</pre>
                </div>
                <div v-else-if="step.type === 'bash_output'" class="chain-step chain-step--bash">
                  <pre class="chain-step__bash">{{ step.content }}</pre>
                </div>
                <div v-else-if="step.type === 'error'" class="chain-step chain-step--error">
                  <span class="chain-step__icon">✗</span>
                  <span>{{ step.error }}</span>
                </div>
              </template>
            </div>
          </div>

          <div v-if="finalContent || streaming || notice" class="task-answer">
            <div v-if="notice" class="task-notice">{{ notice }}</div>
            <div v-if="showTaskLoading" class="task-loading">
              <span class="task-loading__dot" />
              <span class="task-loading__dot" />
              <span class="task-loading__dot" />
              <span class="task-loading__text">正在处理任务…</span>
            </div>
            <template v-else>
              <div v-if="finalContent" class="task-answer__bubble markdown-body" v-html="finalHtml" @click="onMarkdownClick" />
              <span v-if="streaming" class="task-cursor">▋</span>
            </template>
          </div>

          <div v-if="showActions" class="task-msg-actions">
            <button class="task-msg-action" :class="{ copied }" @click="copyTaskContent" :title="copied ? '已复制' : '复制'">
              <svg v-if="copied" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><polyline points="20 6 9 17 4 12"/></svg>
              <svg v-else width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
            </button>
            <button v-if="canRegenerate !== false" class="task-msg-action" @click="emit('regenerate')" title="重新生成">
              <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><polyline points="1 4 1 10 7 10"/><path d="M3.51 15a9 9 0 1 0 .49-4.5"/></svg>
            </button>
          </div>

          <div v-if="fileArtifacts.length > 0" class="task-files">
            <div class="task-files__title">
              <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>
              本次涉及的文件（{{ fileArtifacts.length }}）
            </div>
            <ArtifactCard v-for="(art, i) in fileArtifacts" :key="i" :artifact="art" />
          </div>
        </template>
      </template>
    </div>
  </div>
  <HtmlPreviewDialog v-if="previewHtml" :html="previewHtml" @close="previewHtml = ''" />
</template>

<style scoped>
.task-msg-row {
  display: flex;
  gap: var(--space-4);
  padding: var(--space-4) var(--space-6);
}
.task-msg-row.assistant { background: var(--color-paper-2); }

.task-msg-avatar {
  flex-shrink: 0;
  width: 28px; height: 28px;
  border-radius: var(--radius-full);
  display: flex; align-items: center; justify-content: center;
  font-size: 11px; font-weight: 700;
  margin-top: 2px;
}
.user .task-msg-avatar { background: var(--color-accent); color: #fff; }
.assistant .task-msg-avatar { background: var(--color-paper-4); color: var(--color-text-2); }

.task-msg-content { flex: 1; min-width: 0; }
.task-msg-label {
  font-size: var(--text-xs); font-weight: 600;
  color: var(--color-text-2); margin-bottom: var(--space-1);
}
.task-msg-actions { display: inline-flex; gap: var(--space-1); margin-top: var(--space-2); opacity: 0; transition: opacity var(--duration-fast); }
.task-msg-row:hover .task-msg-actions, .task-msg-content:hover .task-msg-actions { opacity: 1; }
.task-msg-action { display: flex; align-items: center; justify-content: center; width: 22px; height: 22px; border: none; border-radius: var(--radius-sm); background: transparent; color: var(--color-text-3); cursor: pointer; }
.task-msg-action:hover, .task-msg-action.copied { background: var(--color-paper-3); color: var(--color-accent); }

.task-msg__attachments {
  display: flex; flex-wrap: wrap; gap: var(--space-1); margin-bottom: var(--space-2);
}
.task-attachment-chip {
  display: flex; align-items: center; gap: 4px;
  padding: 2px var(--space-2);
  background: var(--color-paper-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-full);
  font-size: var(--text-xs); color: var(--color-text-3);
  max-width: 200px; overflow: hidden;
}
.task-attachment-chip span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.task-user-text {
  word-break: break-word;
  font-size: var(--text-sm);
  line-height: var(--leading-relaxed);
  color: var(--color-text);
}

/* ── Markdown 渲染样式（与 chat 模式一致） ── */
.markdown-body {
  font-size: var(--text-sm);
  line-height: var(--leading-relaxed);
  word-break: break-word;
  color: var(--color-text);
}
.markdown-body :deep(p) { margin: 0 0 var(--space-2); }
.markdown-body :deep(p:last-child) { margin-bottom: 0; }
.markdown-body :deep(h1) { font-size: var(--text-xl); font-weight: 700; margin: var(--space-4) 0 var(--space-2); line-height: 1.3; }
.markdown-body :deep(h2) { font-size: var(--text-lg); font-weight: 600; margin: var(--space-3) 0 var(--space-2); }
.markdown-body :deep(h3) { font-size: var(--text-base); font-weight: 600; margin: var(--space-2) 0 var(--space-1); }
.markdown-body :deep(ul), .markdown-body :deep(ol) { padding-left: var(--space-5); margin: var(--space-1) 0 var(--space-2); }
.markdown-body :deep(li) { margin: 2px 0; }
.markdown-body :deep(li > p) { margin: 0; }
.markdown-body :deep(code) {
  background: var(--color-paper-3);
  padding: 1px 5px;
  border-radius: var(--radius-sm);
  font-family: var(--font-mono);
  font-size: 0.88em;
  color: oklch(0.45 0.15 25);
}
.markdown-body :deep(pre) {
  background: oklch(0.14 0 0);
  border-radius: var(--radius-md);
  overflow-x: auto;
  margin: var(--space-2) 0;
}
.markdown-body :deep(.html-preview-block) { position: relative; }
.markdown-body :deep(.html-preview-run) {
  position: absolute;
  right: 8px;
  bottom: 8px;
  border: 1px solid oklch(1 0 0 / 0.18);
  border-radius: var(--radius-sm);
  background: oklch(0.2 0 0 / 0.9);
  color: oklch(0.92 0 0);
  font-size: 11px;
  padding: 3px 7px;
  cursor: pointer;
}
.markdown-body :deep(.html-preview-run:hover) { background: var(--color-accent); color: #fff; }
.markdown-body :deep(pre code) {
  background: none;
  padding: var(--space-3) var(--space-4);
  display: block;
  font-family: var(--font-mono);
  font-size: 12px;
  line-height: 1.6;
  color: oklch(0.88 0 0);
}
.markdown-body :deep(.hljs-keyword) { color: oklch(0.75 0.15 290); }
.markdown-body :deep(.hljs-string) { color: oklch(0.75 0.15 145); }
.markdown-body :deep(.hljs-comment) { color: oklch(0.55 0 0); font-style: italic; }
.markdown-body :deep(.hljs-number) { color: oklch(0.75 0.15 55); }
.markdown-body :deep(.hljs-function) { color: oklch(0.75 0.12 220); }
.markdown-body :deep(.hljs-title) { color: oklch(0.75 0.12 220); }
.markdown-body :deep(.hljs-built_in) { color: oklch(0.7 0.1 200); }
.markdown-body :deep(.hljs-type) { color: oklch(0.75 0.12 55); }
.markdown-body :deep(.hljs-attr) { color: oklch(0.75 0.1 200); }
.markdown-body :deep(.hljs-variable) { color: oklch(0.88 0 0); }
.markdown-body :deep(blockquote) {
  border-left: 3px solid var(--color-accent);
  padding: var(--space-1) var(--space-3);
  color: var(--color-text-2);
  margin: var(--space-2) 0;
  background: var(--color-accent-soft);
  border-radius: 0 var(--radius-sm) var(--radius-sm) 0;
}
.markdown-body :deep(img), .markdown-body :deep(svg), .markdown-body :deep(video), .markdown-body :deep(canvas), .markdown-body :deep(iframe) {
  max-width: 100%;
  max-height: 360px;
  height: auto;
  box-sizing: border-box;
  object-fit: contain;
}
.markdown-body :deep(img), .markdown-body :deep(video), .markdown-body :deep(canvas), .markdown-body :deep(iframe) {
  display: block;
  margin: var(--space-2) 0;
  border-radius: var(--radius-md);
}
.markdown-body :deep(iframe) {
  width: 100%;
  min-height: 240px;
  border: 1px solid var(--color-border);
}
.markdown-body :deep(table) { border-collapse: collapse; margin: var(--space-2) 0; width: 100%; max-width: 100%; display: block; overflow-x: auto; }
.markdown-body :deep(th), .markdown-body :deep(td) {
  border: 1px solid var(--color-border);
  padding: var(--space-1) var(--space-2);
  font-size: var(--text-sm);
  text-align: left;
}
.markdown-body :deep(th) { background: var(--color-paper-3); font-weight: 600; }
.markdown-body :deep(tr:nth-child(even)) { background: var(--color-paper-2); }
.markdown-body :deep(a) { color: var(--color-accent); text-decoration: none; border-bottom: 1px solid var(--color-accent-soft); }
.markdown-body :deep(a:hover) { border-bottom-color: var(--color-accent); }
.markdown-body :deep(hr) { border: none; border-top: 1px solid var(--color-border); margin: var(--space-3) 0; }
.markdown-body :deep(strong) { font-weight: 600; }
.markdown-body :deep(em) { font-style: italic; color: var(--color-text-2); }

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
.chain-step--note {
  background: oklch(0.975 0.006 250);
  font-family: inherit;
  font-size: 12px;
  color: var(--color-text-3);
}
.chain-step--tool { background: oklch(0.975 0.008 220); }
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

/* ── 本次涉及的产物区 ── */
.task-files {
  max-width: 760px;
  width: 100%;
  margin-top: 10px;
}
.task-files__title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  font-weight: 600;
  color: var(--color-text-2);
  margin-bottom: 6px;
}

/* ── 最终回答 ── */
.task-answer { max-width: 760px; width: 100%; }
.task-notice {
  background: oklch(0.96 0.04 75);
  color: oklch(0.45 0.12 55);
  border: 1px solid oklch(0.85 0.08 75);
  border-radius: 8px;
  padding: 7px 12px;
  font-size: 12.5px;
  line-height: 1.5;
  margin-bottom: 8px;
}
.task-answer__bubble {
  background: var(--color-paper-2);
  border-radius: 4px 16px 16px 16px;
  padding: 10px 14px;
  word-break: break-word;
}

.task-loading {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 8px 12px;
  border-radius: 999px;
  background: var(--color-paper-2);
  border: 1px solid var(--color-border);
  color: var(--color-text-3);
  font-size: 12.5px;
}
.task-loading__dot {
  width: 5px;
  height: 5px;
  border-radius: 999px;
  background: currentColor;
  opacity: 0.35;
  animation: task-dot 1.2s ease-in-out infinite;
}
.task-loading__dot:nth-child(2) { animation-delay: 0.15s; }
.task-loading__dot:nth-child(3) { animation-delay: 0.3s; }
.task-loading__text { margin-left: 2px; }

/* 光标 */
.task-cursor {
  display: inline-block;
  animation: blink 1s step-end infinite;
  margin-left: 2px;
  font-family: monospace;
}
@keyframes blink { 50% { opacity: 0; } }
@keyframes task-dot {
  0%, 80%, 100% { opacity: 0.25; transform: translateY(0); }
  40% { opacity: 0.9; transform: translateY(-2px); }
}
</style>
