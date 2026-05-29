<script setup lang="ts">
import { useChatStore } from '../stores/chat'
import MessageItem from './MessageItem.vue'
import { storage as models } from '../../wailsjs/go/models'

const store = useChatStore()

function makeStreamMsg(content: string) {
  const m = new models.Message({})
  m.id = 'streaming'
  m.role = 'assistant'
  m.content = content
  m.conversation_id = ''
  return m
}
</script>

<template>
  <div class="message-list" ref="listRef">
    <div v-if="!store.messages.length && !store.streamContent" class="msg-hint">
      <p>发送一条消息开始对话</p>
    </div>
    <template v-for="m in store.messages" :key="m.id">
      <MessageItem :msg="m" />
    </template>
    <div v-if="store.streamContent" class="streaming">
      <MessageItem :msg="makeStreamMsg(store.streamContent)" />
    </div>
    <div ref="sentinel" />
  </div>
</template>

<style scoped>
.message-list {
  flex: 1;
  overflow-y: auto;
  padding: var(--space-2) 0;
}

.msg-hint {
  text-align: center;
  padding: var(--space-12) var(--space-4);
  color: var(--color-text-3);
  font-size: var(--text-sm);
}

.streaming { opacity: 0.85; }
</style>
