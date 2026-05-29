<script setup lang="ts">
import { useChatStore } from '../stores/chat'
import MessageList from './MessageList.vue'
import InputArea from './InputArea.vue'

const store = useChatStore()
</script>

<template>
  <div v-if="store.currentConvId" class="chat-view">
    <div class="chat-header">
      <div class="chat-header-info">
        <span class="chat-header-title">{{ store.conversations.find(c => c.id === store.currentConvId)?.title || '对话' }}</span>
        <span class="chat-header-model" v-if="store.conversations.find(c => c.id === store.currentConvId) as any">
          {{ (store.conversations.find(c => c.id === store.currentConvId) as any)?.provider }} · {{ (store.conversations.find(c => c.id === store.currentConvId) as any)?.model }}
        </span>
      </div>
    </div>
    <MessageList />
    <InputArea />
  </div>
  <div v-else class="chat-view empty">
    <div class="welcome">
      <div class="welcome-icon">
        <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>
      </div>
      <h1 class="welcome-title">AI Chat</h1>
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
