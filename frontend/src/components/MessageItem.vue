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
        `🔧 **Tool Call:** ${c.function?.name}\n\`\`\`json\n${JSON.stringify(JSON.parse(c.function?.arguments || '{}'), null, 2)}\n\`\`\``
      ).join('\n\n')
      return props.msg.content + '\n\n' + toolParts
    } catch {
      return props.msg.content
    }
  }
  return props.msg.content
})
</script>

<template>
  <div class="message" :class="{ user: isUser, assistant: !isUser }">
    <div class="avatar">{{ isUser ? 'U' : 'AI' }}</div>
    <div class="bubble">
      <div class="role-label">{{ isUser ? 'You' : 'Assistant' }}</div>
      <div class="content" v-text="content || (msg.role === 'assistant' ? '...' : '')" />
    </div>
  </div>
</template>

<style scoped>
.message { display: flex; gap: 12px; padding: 16px 24px; }
.avatar {
  width: 32px; height: 32px; border-radius: 50%; display: flex; align-items: center;
  justify-content: center; font-size: 12px; font-weight: 600; flex-shrink: 0;
}
.user .avatar { background: var(--accent); color: #fff; }
.assistant .avatar { background: #e5e7eb; color: #374151; }
.bubble { flex: 1; min-width: 0; }
.role-label { font-size: 12px; font-weight: 600; margin-bottom: 4px; color: var(--text-secondary); }
.content { line-height: 1.6; white-space: pre-wrap; word-break: break-word; }
</style>
