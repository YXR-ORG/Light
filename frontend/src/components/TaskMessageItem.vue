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
  isHistory?: boolean  // 历史消息模式：不显示推理链标签，直接渲染 markdown
}>()

// 用于渲染用户消息 markdown
const userHtml = computed(() => {
  if (!props.userContent) return ''
  return marked(props.userContent) as string
})

// 最终回答内容
const finalContent = computed(() => {
  return props.steps
    .filter(s => s.type === 'content')
    .map(s => s.content || '')
    .join('')
})

const finalHtml = computed(() => {
  if (!finalContent.value) return ''
  return marked(finalContent.value) as string
})

// 推理链步骤（非 content/done），合并连续同类型步骤
const chainSteps = computed(() => {
  const raw = props.steps.filter(s => s.type !== 'content' && s.type !== 'done')
  if (raw.length === 0) return []
  // 合并连续相同 type 的步骤（thinking、tool_result、bash_output 的流式 delta）
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

// 思考步骤折叠
const thinkingOpen = ref(false)
// 每个 tool 步骤折叠 map（index → open）
const toolOpen = ref<Record<number, boolean>>({})

function toggleTool(i: number) {
  toolOpen.value[i] = !toolOpen.value[i]
}

function formatArgs(args?: string) {
  if (!args) return ''
  try {
    return JSON.stringify(JSON.parse(args), null, 2)
  } catch {
    return args
  }
}

function truncate(s: string, max = 500) {
  if (s.length <= max) return s
  return s.slice(0, max) + '\n…（内容已截断，点击展开全部）'
}

// 工具图标
function toolIcon(name?: string) {
  if (!name) return '🔧'
  if (name === 'bash_exec') return '💻'
  if (name === 'read_file') return '📖'
  if (name === 'write_file') return '✏️'
  if (name === 'list_dir') return '📂'
  if (name === 'make_dir') return '📁'
  if (name === 'search_knowledge' || name.startsWith('search_')) return '🔍'
  if (name === 'web_search') return '🌐'
  return '🔧'
}
</script>

<template>
  <!-- 用户消息 -->
  <div v-if="role === 'user'" class="task-msg task-msg--user">
    <div class="task-msg__bubble" v-html="userHtml" />
  </div>

  <!-- AI task 消息 -->
  <div v-else class="task-msg task-msg--assistant">

    <!-- 历史模式：直接渲染 markdown，无标签无推理链 -->
    <div v-if="isHistory" class="task-msg__bubble markdown-body" v-html="finalHtml" />

    <!-- 流式模式：推理链 + 回答 -->
    <template v-else>
      <!-- 推理链 -->
      <div v-if="chainSteps.length" class="task-chain">

        <!-- 思考步骤（折叠） -->
        <template v-for="(step, i) in chainSteps" :key="i">

          <!-- thinking -->
          <div v-if="step.type === 'thinking'" class="chain-card chain-card--thinking">
            <button class="chain-card__header" @click="thinkingOpen = !thinkingOpen">
              <span>🤔 思考</span>
              <span class="chain-card__toggle">{{ thinkingOpen ? '▲' : '▼' }}</span>
            </button>
            <div v-if="thinkingOpen" class="chain-card__body chain-card__body--pre">{{ step.content }}</div>
          </div>

          <!-- tool_call -->
          <div v-else-if="step.type === 'tool_call'" class="chain-card chain-card--tool">
            <button class="chain-card__header" @click="toggleTool(i)">
              <span>{{ toolIcon(step.tool_name) }} {{ step.tool_name }}</span>
              <span class="chain-card__toggle">{{ toolOpen[i] ? '▲' : '▼' }}</span>
            </button>
            <div v-if="toolOpen[i]" class="chain-card__body">
              <pre class="chain-card__code">{{ formatArgs(step.tool_args) }}</pre>
            </div>
          </div>

          <!-- tool_result -->
          <div v-else-if="step.type === 'tool_result'" class="chain-card chain-card--result">
            <div class="chain-card__result-label">↳ 结果</div>
            <pre class="chain-card__result-body">{{ truncate(step.tool_result || '') }}</pre>
          </div>

          <!-- bash_output -->
          <div v-else-if="step.type === 'bash_output'" class="chain-card chain-card--bash-output">
            <pre class="chain-card__result-body">{{ step.content }}</pre>
          </div>

          <!-- error -->
          <div v-else-if="step.type === 'error'" class="chain-card chain-card--error">
            <span>❌ {{ step.error }}</span>
          </div>

        </template>
      </div>

      <!-- 最终回答 -->
      <div v-if="finalContent || streaming" class="task-msg__answer">
        <div class="task-msg__answer-label">💬 回答</div>
        <div class="task-msg__bubble markdown-body" v-html="finalHtml" />
        <span v-if="streaming" class="task-cursor">▋</span>
      </div>
    </template>

  </div>
</template>

<style scoped>
.task-msg {
  display: flex;
  flex-direction: column;
  margin: 8px 0;
}

.task-msg--user {
  align-items: flex-end;
}

.task-msg--user .task-msg__bubble {
  background: var(--color-user-bg, oklch(0.6 0.15 250));
  color: #fff;
  border-radius: 16px 16px 4px 16px;
  padding: 10px 14px;
  max-width: 72%;
  word-break: break-word;
}

.task-msg--assistant {
  align-items: flex-start;
}

.task-chain {
  width: 100%;
  max-width: 760px;
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-bottom: 8px;
}

.chain-card {
  border-radius: 8px;
  border: 1px solid var(--color-border, oklch(0.88 0 0));
  overflow: hidden;
  font-size: 13px;
}

.chain-card--thinking {
  border-color: oklch(0.82 0.05 280);
  background: oklch(0.97 0.01 280);
}

.chain-card--tool {
  border-color: oklch(0.85 0.06 220);
  background: oklch(0.97 0.01 220);
}

.chain-card--result {
  border-color: oklch(0.88 0.04 160);
  background: oklch(0.97 0.01 160);
  padding: 6px 10px;
}

.chain-card--bash-output {
  background: oklch(0.12 0 0);
  border-color: oklch(0.25 0 0);
  padding: 6px 10px;
}

.chain-card--bash-output pre {
  color: oklch(0.85 0.05 150);
}

.chain-card--error {
  background: oklch(0.97 0.03 20);
  border-color: oklch(0.8 0.1 20);
  padding: 8px 12px;
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
  font-size: 13px;
  font-weight: 500;
  text-align: left;
  color: inherit;
}

.chain-card__header:hover {
  opacity: 0.8;
}

.chain-card__toggle {
  font-size: 10px;
  opacity: 0.5;
}

.chain-card__body {
  padding: 6px 10px;
  border-top: 1px solid oklch(0.9 0 0 / 0.5);
}

.chain-card__body--pre,
.chain-card__code,
.chain-card__result-body {
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
  margin: 0;
  max-height: 300px;
  overflow-y: auto;
}

.chain-card__result-label {
  font-size: 11px;
  opacity: 0.6;
  margin-bottom: 4px;
}

.task-msg__answer {
  max-width: 760px;
  width: 100%;
}

.task-msg__answer-label {
  font-size: 12px;
  opacity: 0.5;
  margin-bottom: 4px;
}

.task-msg__bubble {
  background: var(--color-assistant-bg, oklch(0.97 0 0));
  border-radius: 4px 16px 16px 16px;
  padding: 10px 14px;
  word-break: break-word;
}

.task-cursor {
  display: inline-block;
  animation: blink 1s step-end infinite;
  margin-left: 2px;
}

@keyframes blink {
  50% { opacity: 0; }
}
</style>
