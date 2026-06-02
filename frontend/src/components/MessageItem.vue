<script setup lang="ts">
import type { storage } from '../../wailsjs/go/models'
import { computed, ref, watch } from 'vue'
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

const props = defineProps<{
  msg: storage.Message
  streaming?: boolean
  thinking?: string
  isLast?: boolean      // 是否是最后一条（保留用于重新生成按钮判断）
  showActions?: boolean // 是否显示操作按钮（所有历史 assistant 消息都显示）
}>()

const emit = defineEmits<{
  regenerate: []
}>()

const isUser = computed(() => props.msg.role === 'user')
const thinkingOpen = ref(false)
const copied = ref(false)

watch(() => props.streaming, (v) => { if (v) thinkingOpen.value = true })

const content = computed(() => {
  if (props.msg.role === 'assistant' && props.msg.tool_calls) {
    try {
      const calls = JSON.parse(props.msg.tool_calls)
      const toolParts = calls.map((c: any) =>
        `🔧 **工具调用:** ${c.function?.name}\n\`\`\`json\n${JSON.stringify(JSON.parse(c.function?.arguments || '{}'), null, 2)}\n\`\`\``
      ).join('\n\n')
      return props.msg.content + '\n\n' + toolParts
    } catch { return props.msg.content }
  }
  return props.msg.content
})

const renderedContent = computed(() => {
  if (isUser.value) return content.value
  return marked.parse(content.value || '') as string
})

const thinkingText = computed(() => props.thinking || props.msg.thinking || '')
const hasThinking = computed(() => !!thinkingText.value)

const attachmentMetas = computed(() => {
  if (!props.msg.attachments) return []
  try { return JSON.parse(props.msg.attachments) as { name: string; mime_type: string }[] }
  catch { return [] }
})

// 复制纯文本内容（去掉 tool_calls 部分）
async function copyContent() {
  const text = props.msg.content || ''
  await navigator.clipboard.writeText(text).catch(() => {})
  copied.value = true
  setTimeout(() => { copied.value = false }, 2000)
}
</script>

<template>
  <div class="msg-row" :class="{ user: isUser, assistant: !isUser }">
    <div class="msg-avatar">{{ isUser ? 'U' : 'AI' }}</div>
    <div class="msg-content">
      <div class="msg-label">{{ isUser ? '你' : 'AI 助手' }}</div>

      <!-- 思考块 -->
      <div v-if="!isUser && hasThinking" class="thinking-block">
        <button class="thinking-toggle" @click="thinkingOpen = !thinkingOpen">
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"
            :style="{ transform: thinkingOpen ? 'rotate(90deg)' : 'rotate(0deg)', transition: 'transform 0.2s' }">
            <polyline points="9 18 15 12 9 6"/>
          </svg>
          <span>思考过程</span>
          <span v-if="streaming" class="thinking-badge">思考中...</span>
        </button>
        <div v-if="thinkingOpen" class="thinking-body">
          <div class="thinking-text">{{ thinkingText }}</div>
        </div>
      </div>

      <!-- 附件标签 -->
      <div v-if="attachmentMetas.length > 0" class="msg-attachments">
        <div v-for="(a, i) in attachmentMetas" :key="i" class="msg-attachment-chip">
          <svg v-if="a.mime_type.startsWith('image/')" width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="8.5" cy="8.5" r="1.5"/><polyline points="21 15 16 10 5 21"/></svg>
          <svg v-else width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>
          <span>{{ a.name }}</span>
        </div>
      </div>

      <!-- 消息正文 -->
      <div class="msg-text" v-if="isUser" v-text="content || ''" />
      <div class="msg-text markdown-body" v-else v-html="renderedContent || ''" />
      <span v-if="props.streaming && !isUser" class="cursor" />

      <!-- 操作按钮：所有历史 assistant 消息 hover 时显示 -->
      <div v-if="!isUser && showActions && !streaming" class="msg-actions">
        <button class="msg-action-btn" :class="{ copied }" @click="copyContent" :title="copied ? '已复制' : '复制'">
          <svg v-if="copied" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>
          <svg v-else width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
        </button>
        <!-- 所有 assistant 消息都显示重新生成 -->
        <button class="msg-action-btn" @click="emit('regenerate')" title="重新生成">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><polyline points="1 4 1 10 7 10"/><path d="M3.51 15a9 9 0 1 0 .49-4.5"/></svg>
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.msg-attachments {
  display: flex; flex-wrap: wrap; gap: var(--space-1); margin-bottom: var(--space-2);
}
.msg-attachment-chip {
  display: flex; align-items: center; gap: 4px;
  padding: 2px var(--space-2);
  background: var(--color-paper-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-full);
  font-size: var(--text-xs); color: var(--color-text-3);
  max-width: 200px; overflow: hidden;
}
.msg-attachment-chip span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

.msg-row {
  display: flex;
  gap: var(--space-4);
  padding: var(--space-4) var(--space-6);
}

.msg-row.assistant {
  background: var(--color-paper-2);
}

.msg-avatar {
  flex-shrink: 0;
  width: 28px; height: 28px;
  border-radius: var(--radius-full);
  display: flex; align-items: center; justify-content: center;
  font-size: 11px; font-weight: 700;
  margin-top: 2px;
}

.user .msg-avatar { background: var(--color-accent); color: #fff; }
.assistant .msg-avatar { background: var(--color-paper-4); color: var(--color-text-2); }

.msg-content { flex: 1; min-width: 0; }

.msg-label {
  font-size: var(--text-xs); font-weight: 600;
  color: var(--color-text-2); margin-bottom: var(--space-1);
}

/* 思考块 */
.thinking-block {
  margin-bottom: var(--space-3);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  overflow: hidden;
}

.thinking-toggle {
  display: flex; align-items: center; gap: var(--space-1);
  width: 100%; padding: var(--space-2) var(--space-3);
  background: var(--color-paper-2); border: none; cursor: pointer;
  font-size: var(--text-xs); color: var(--color-text-3);
  font-family: inherit; text-align: left;
  transition: background var(--duration-fast) var(--ease-out);
}
.thinking-toggle:hover { background: var(--color-paper-3); color: var(--color-text-2); }

.thinking-badge {
  margin-left: auto;
  font-size: 10px; color: var(--color-accent);
  animation: pulse 1.5s ease-in-out infinite;
}
@keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.4; } }

.thinking-body {
  padding: var(--space-3);
  background: var(--color-paper-2);
  border-top: 1px solid var(--color-border);
}

.thinking-text {
  font-size: var(--text-xs);
  line-height: 1.7;
  color: var(--color-text-3);
  white-space: pre-wrap;
  word-break: break-word;
  font-family: var(--font-mono);
}

/* 用户消息 */
.user .msg-text {
  font-size: var(--text-sm);
  line-height: var(--leading-relaxed);
  white-space: pre-wrap;
  word-break: break-word;
  color: var(--color-text);
}

/* AI 消息：markdown 渲染，不用 pre-wrap（由 markdown 元素自己控制） */
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

/* 行内代码 */
.markdown-body :deep(code) {
  background: var(--color-paper-3);
  padding: 1px 5px;
  border-radius: var(--radius-sm);
  font-family: var(--font-mono);
  font-size: 0.88em;
  color: oklch(0.45 0.15 25);
}

/* 代码块 */
.markdown-body :deep(pre) {
  background: oklch(0.14 0 0);
  border-radius: var(--radius-md);
  overflow-x: auto;
  margin: var(--space-2) 0;
  position: relative;
}

.markdown-body :deep(pre code) {
  background: none;
  padding: var(--space-3) var(--space-4);
  display: block;
  font-family: var(--font-mono);
  font-size: 12px;
  line-height: 1.6;
  color: oklch(0.88 0 0);
}

/* highlight.js 主题色（暗色代码块） */
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

.markdown-body :deep(table) { border-collapse: collapse; margin: var(--space-2) 0; width: 100%; }
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

.cursor {
  display: inline-block;
  width: 2px; height: 1em;
  background: var(--color-accent);
  margin-left: 2px;
  vertical-align: text-bottom;
  animation: blink 0.8s step-end infinite;
}

@keyframes blink {
  0%, 100% { opacity: 1; }
  50% { opacity: 0; }
}

/* 消息操作按钮 */
.msg-actions {
  display: flex;
  gap: var(--space-1);
  margin-top: var(--space-2);
  opacity: 0;
  transition: opacity var(--duration-fast);
}
.msg-row:hover .msg-actions { opacity: 1; }

.msg-action-btn {
  display: flex; align-items: center; justify-content: center;
  width: 26px; height: 26px;
  border: none; background: transparent;
  color: var(--color-text-3);
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: background var(--duration-fast), color var(--duration-fast);
}
.msg-action-btn:hover { background: var(--color-paper-3); color: var(--color-text); }
.msg-action-btn.copied { color: var(--color-success); }
</style>
