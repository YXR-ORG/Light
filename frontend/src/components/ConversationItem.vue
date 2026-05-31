<script setup lang="ts">
import { computed } from 'vue'
import type { storage } from '../../wailsjs/go/models'

const props = defineProps<{
  conv: storage.Conversation
  active: boolean
  highlight?: string
}>()

const emit = defineEmits<{
  select: [id: string]
  delete: [id: string]
}>()

function onClick() { emit('select', props.conv.id) }
function onDelete(e: MouseEvent) { e.stopPropagation(); emit('delete', props.conv.id) }

const titleHtml = computed(() => {
  const title = props.conv.title
  const q = props.highlight?.trim()
  if (!q) return escapeHtml(title)
  const idx = title.toLowerCase().indexOf(q.toLowerCase())
  if (idx === -1) return escapeHtml(title)
  return (
    escapeHtml(title.slice(0, idx)) +
    `<mark>${escapeHtml(title.slice(idx, idx + q.length))}</mark>` +
    escapeHtml(title.slice(idx + q.length))
  )
})

function escapeHtml(s: string) {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}
</script>

<template>
  <div class="conv-item" :class="{ active }" @click="onClick">
    <div class="conv-icon">
      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>
    </div>
    <div class="conv-body">
      <div class="conv-title" v-html="titleHtml"></div>
      <div class="conv-meta">{{ conv.provider }} · {{ conv.model }}</div>
    </div>
    <button class="btn-delete" @click="onDelete" title="删除">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M18 6 6 18M6 6l12 12"/></svg>
    </button>
  </div>
</template>

<style scoped>
.conv-item {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-3) var(--space-3);
  border-radius: var(--radius-md);
  cursor: pointer;
  transition: background var(--duration-fast) var(--ease-out);
  position: relative;
}

.conv-item:hover { background: var(--color-sidebar-hover); }
.conv-item.active { background: var(--color-hover); }

.conv-icon {
  flex-shrink: 0;
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-md);
  background: var(--color-paper-3);
  color: var(--color-text-2);
}

.conv-item.active .conv-icon {
  background: var(--color-accent-soft);
  color: var(--color-accent);
}

.conv-body {
  flex: 1;
  min-width: 0;
}

.conv-title {
  font-size: var(--text-sm);
  font-weight: 500;
  line-height: var(--leading-tight);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.conv-meta {
  font-size: var(--text-xs);
  color: var(--color-text-3);
  margin-top: 2px;
}

.btn-delete {
  opacity: 0;
  position: absolute;
  right: var(--space-2);
  top: 50%;
  transform: translateY(-50%);
  width: 24px;
  height: 24px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-text-3);
  cursor: pointer;
  transition: opacity var(--duration-fast) var(--ease-out), background var(--duration-fast) var(--ease-out);
}

.conv-item:hover .btn-delete { opacity: 1; }
.btn-delete:hover { background: var(--color-paper-4); color: var(--color-danger); }

.conv-title mark {
  background: var(--color-accent-soft);
  color: var(--color-accent-2);
  border-radius: 2px;
  padding: 0 1px;
}
</style>
