<script setup lang="ts">
import type { storage } from '../../wailsjs/go/models'

const props = defineProps<{
  conv: storage.Conversation
  active: boolean
}>()

const emit = defineEmits<{
  select: [id: string]
  delete: [id: string]
}>()

function onClick() {
  emit('select', props.conv.id)
}

function onDelete(e: MouseEvent) {
  e.stopPropagation()
  emit('delete', props.conv.id)
}
</script>

<template>
  <div class="conv-item" :class="{ active }" @click="onClick">
    <div class="conv-title">{{ conv.title }}</div>
    <div class="conv-meta">{{ conv.provider }} · {{ conv.model }}</div>
    <button class="delete-btn" @click="onDelete">×</button>
  </div>
</template>

<style scoped>
.conv-item {
  padding: 8px 12px; cursor: pointer; border-radius: 6px; position: relative;
  transition: background 0.15s;
}
.conv-item:hover { background: var(--hover-bg); }
.conv-item.active { background: var(--active-bg); }
.conv-title {
  font-size: 13px; font-weight: 500; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
}
.conv-meta { font-size: 11px; color: var(--text-secondary); margin-top: 2px; }
.delete-btn {
  position: absolute; right: 8px; top: 50%; transform: translateY(-50%);
  display: none; border: none; background: none; cursor: pointer;
  color: var(--text-secondary); font-size: 16px;
}
.conv-item:hover .delete-btn { display: block; }
</style>
