<script setup lang="ts">
import type { storage } from '../../wailsjs/go/models'
import { computed } from 'vue'

const props = defineProps<{ msg: storage.Message }>()

const isUser = computed(() => props.msg.role === 'user')

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
</script>

<template>
  <div class="msg-row" :class="{ user: isUser, assistant: !isUser }">
    <div class="msg-avatar">{{ isUser ? 'U' : 'AI' }}</div>
    <div class="msg-content">
      <div class="msg-label">{{ isUser ? '你' : 'AI 助手' }}</div>
      <div class="msg-text" v-text="content || (msg.role === 'assistant' ? '...' : '')" />
    </div>
  </div>
</template>

<style scoped>
.msg-row {
  display: flex;
  gap: var(--space-4);
  padding: var(--space-4) var(--space-6);
  transition: background var(--duration-fast) var(--ease-out);
}

.msg-row.assistant {
  background: var(--color-paper-2);
}

.msg-avatar {
  flex-shrink: 0;
  width: 28px;
  height: 28px;
  border-radius: var(--radius-full);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: -0.01em;
  margin-top: 2px;
}

.user .msg-avatar {
  background: var(--color-accent);
  color: #fff;
}

.assistant .msg-avatar {
  background: var(--color-paper-4);
  color: var(--color-text-2);
}

.msg-content {
  flex: 1;
  min-width: 0;
}

.msg-label {
  font-size: var(--text-xs);
  font-weight: 600;
  color: var(--color-text-2);
  margin-bottom: var(--space-1);
}

.msg-text {
  font-size: var(--text-sm);
  line-height: var(--leading-relaxed);
  white-space: pre-wrap;
  word-break: break-word;
  color: var(--color-text);
}

.user .msg-text {
  color: var(--color-text);
}
</style>
