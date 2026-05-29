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
  <div class="message-list">
    <template v-for="m in store.messages" :key="m.id">
      <MessageItem :msg="m" />
    </template>
    <div v-if="store.streamContent" class="streaming">
      <MessageItem :msg="makeStreamMsg(store.streamContent)" />
    </div>
  </div>
</template>

<style scoped>
.message-list { flex: 1; overflow-y: auto; padding: 8px 0; }
.streaming { opacity: 0.8; }
</style>
