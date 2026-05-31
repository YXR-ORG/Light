<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useChatStore } from '../stores/chat'
import { SetSystemPrompt } from '../../wailsjs/go/handler/ConversationHandler'
import { ListAgents } from '../../wailsjs/go/handler/AgentHandler'
import type { storage } from '../../wailsjs/go/models'

const store = useChatStore()
const agents = ref<storage.Agent[]>([])

onMounted(async () => {
  agents.value = await ListAgents().catch(() => [])
})

const currentConv = computed(() =>
  store.conversations.find(c => c.id === store.currentConvId)
)

const activeAgentId = computed(() => {
  const prompt = (currentConv.value as any)?.system_prompt ?? ''
  const match = agents.value.find(a => a.system_prompt === prompt)
  return match?.id ?? 'builtin-default'
})

async function selectAgent(agent: storage.Agent) {
  if (!store.currentConvId) return
  try {
    await SetSystemPrompt(store.currentConvId, agent.system_prompt)
    const conv = store.conversations.find(c => c.id === store.currentConvId)
    if (conv) (conv as any).system_prompt = agent.system_prompt
  } catch (e) {
    console.error('SetSystemPrompt failed', e)
  }
}
</script>

<template>
  <div class="skills-panel">
    <div class="skills-header">
      <span class="skills-title">选择智能体</span>
      <span class="skills-hint">为当前对话设置 AI 角色</span>
    </div>
    <div class="skills-list">
      <button
        v-for="agent in agents"
        :key="agent.id"
        class="skill-item"
        :class="{ active: activeAgentId === agent.id }"
        @click="selectAgent(agent)"
      >
        <span class="skill-icon">{{ agent.icon }}</span>
        <div class="skill-info">
          <span class="skill-name">{{ agent.name }}</span>
          <span class="skill-desc">{{ agent.description }}</span>
        </div>
        <span v-if="activeAgentId === agent.id" class="skill-check">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="20 6 9 17 4 12"/>
          </svg>
        </span>
      </button>
    </div>
  </div>
</template>

<style scoped>
.skills-panel {
  background: var(--color-paper);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-lg);
  overflow: hidden;
  width: 100%;
}
.skills-header {
  display: flex; align-items: baseline; gap: var(--space-2);
  padding: var(--space-3) var(--space-4);
  border-bottom: 1px solid var(--color-border);
  background: var(--color-paper-2);
}
.skills-title { font-size: var(--text-sm); font-weight: 600; color: var(--color-text); }
.skills-hint { font-size: var(--text-xs); color: var(--color-text-3); }
.skills-list { padding: var(--space-2); display: flex; flex-direction: column; gap: var(--space-1); }
.skill-item {
  display: flex; align-items: center; gap: var(--space-3);
  padding: var(--space-2) var(--space-3);
  border: none; border-radius: var(--radius-md);
  background: transparent; cursor: pointer; text-align: left;
  transition: background var(--duration-fast) var(--ease-out); width: 100%;
}
.skill-item:hover { background: var(--color-paper-3); }
.skill-item.active { background: var(--color-accent-soft); }
.skill-icon { font-size: 18px; line-height: 1; flex-shrink: 0; width: 28px; text-align: center; }
.skill-info { flex: 1; display: flex; flex-direction: column; gap: 2px; min-width: 0; }
.skill-name { font-size: var(--text-sm); font-weight: 500; color: var(--color-text); line-height: var(--leading-tight); }
.skill-desc { font-size: var(--text-xs); color: var(--color-text-3); line-height: var(--leading-tight); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.skill-check { color: var(--color-accent); flex-shrink: 0; display: flex; align-items: center; }
</style>
