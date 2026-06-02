<script setup lang="ts">
import { computed, ref, nextTick } from 'vue'
import type { storage } from '../../wailsjs/go/models'

const props = defineProps<{
  conv: storage.Conversation
  active: boolean
  highlight?: string
}>()

const emit = defineEmits<{
  select: [id: string]
  delete: [id: string]
  rename: [id: string, title: string]
  toggleFavorite: [id: string]
}>()

// ── 右键菜单 ─────────────────────────────────────────────────────────
const ctxMenu = ref<{ x: number; y: number } | null>(null)
const ctxRef = ref<HTMLElement | null>(null)

function onContextMenu(e: MouseEvent) {
  e.preventDefault()
  ctxMenu.value = { x: e.clientX, y: e.clientY }
  nextTick(() => {
    if (ctxRef.value) {
      const el = ctxRef.value
      const vh = window.innerHeight
      if (ctxMenu.value && ctxMenu.value.y + el.offsetHeight > vh) {
        ctxMenu.value = { x: ctxMenu.value.x, y: vh - el.offsetHeight - 8 }
      }
    }
    document.addEventListener('mousedown', closeCtx, { once: true })
  })
}

function closeCtx() {
  ctxMenu.value = null
}

// ── 重命名（内联编辑）────────────────────────────────────────────────
const renaming = ref(false)
const renameValue = ref('')
const renameInput = ref<HTMLInputElement | null>(null)

function startRename() {
  closeCtx()
  renaming.value = true
  renameValue.value = props.conv.title
  nextTick(() => {
    renameInput.value?.focus()
    renameInput.value?.select()
  })
}

function commitRename() {
  const v = renameValue.value.trim()
  if (v && v !== props.conv.title) emit('rename', props.conv.id, v)
  renaming.value = false
}

function cancelRename() { renaming.value = false }

// ── 其他操作 ─────────────────────────────────────────────────────────
function onClick() { if (!renaming.value && !pendingDelete.value) emit('select', props.conv.id) }

// 删除：hover 按钮两步确认
const pendingDelete = ref(false)

function requestDelete(e: MouseEvent) {
  e.stopPropagation()
  pendingDelete.value = true
  nextTick(() => {
    document.addEventListener('mousedown', cancelDelete, { once: true })
  })
}

function cancelDelete() {
  pendingDelete.value = false
}

function confirmDeleteNow(e: MouseEvent) {
  e.stopPropagation()
  pendingDelete.value = false
  emit('delete', props.conv.id)
}

// ── 高亮搜索词 ───────────────────────────────────────────────────────
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
  <div
    class="conv-item"
    :class="{ active }"
    @click="onClick"
    @contextmenu.prevent="onContextMenu"
  >
    <!-- 已收藏：星标常驻在图标右上角 -->
    <div class="conv-icon">
      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>
      <span v-if="conv.starred" class="star-badge">
        <svg width="8" height="8" viewBox="0 0 24 24" fill="currentColor" stroke="none"><polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"/></svg>
      </span>
    </div>

    <div class="conv-body">
      <input
        v-if="renaming"
        ref="renameInput"
        class="rename-input"
        v-model="renameValue"
        @blur="commitRename"
        @keydown.enter.prevent="commitRename"
        @keydown.escape.prevent="cancelRename"
        @click.stop
        maxlength="120"
      />
      <div v-else class="conv-title" v-html="titleHtml" />
      <div class="conv-meta">{{ conv.provider }} · {{ conv.model }}</div>
    </div>

    <!-- hover 操作区：删除（两步确认） -->
    <div class="conv-actions" @click.stop @mousedown.stop>
      <template v-if="!pendingDelete">
        <button class="btn-delete" @click="requestDelete" title="删除">
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><path d="M18 6 6 18M6 6l12 12"/></svg>
        </button>
      </template>
      <template v-else>
        <span class="confirm-label">删除？</span>
        <button class="confirm-yes" @click="confirmDeleteNow" title="确认删除">
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><polyline points="20 6 9 17 4 12"/></svg>
        </button>
        <button class="confirm-no" @click.stop="cancelDelete" title="取消">
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><path d="M18 6 6 18M6 6l12 12"/></svg>
        </button>
      </template>
    </div>
  </div>

  <!-- 右键菜单 -->
  <Teleport to="body">
    <div
      v-if="ctxMenu"
      ref="ctxRef"
      class="ctx-menu"
      :style="{ left: ctxMenu.x + 'px', top: ctxMenu.y + 'px' }"
      @mousedown.stop
    >
      <button class="ctx-item" @click="startRename">
        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>
        重命名
      </button>
      <button class="ctx-item" @click="emit('toggleFavorite', conv.id); closeCtx()">
        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"/></svg>
        {{ conv.starred ? '取消收藏' : '收藏' }}
      </button>
    </div>
  </Teleport>
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

/* 图标 + 收藏星标角标 */
.conv-icon {
  flex-shrink: 0;
  width: 32px; height: 32px;
  display: flex; align-items: center; justify-content: center;
  border-radius: var(--radius-md);
  background: var(--color-paper-3);
  color: var(--color-text-2);
  position: relative;
}
.conv-item.active .conv-icon {
  background: var(--color-accent-soft);
  color: var(--color-accent);
}
.star-badge {
  position: absolute;
  bottom: -2px; right: -2px;
  width: 13px; height: 13px;
  background: var(--color-sidebar);
  border-radius: 50%;
  display: flex; align-items: center; justify-content: center;
  color: oklch(0.78 0.18 75);
}
.conv-item.active .star-badge { background: var(--color-hover); }

.conv-body { flex: 1; min-width: 0; }

.conv-title {
  font-size: var(--text-sm); font-weight: 500;
  line-height: var(--leading-tight);
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
}
.conv-meta { font-size: var(--text-xs); color: var(--color-text-3); margin-top: 2px; }

.rename-input {
  width: 100%; font-size: var(--text-sm); font-weight: 500;
  line-height: var(--leading-tight);
  background: var(--color-paper); border: 1px solid var(--color-accent);
  border-radius: var(--radius-sm); padding: 1px var(--space-1);
  color: var(--color-text); font-family: var(--font-body); outline: none;
}

/* hover 操作区 */
.conv-actions {
  opacity: 0;
  flex-shrink: 0;
  display: flex; align-items: center; gap: 2px;
  transition: opacity var(--duration-fast);
}
.conv-item:hover .conv-actions { opacity: 1; }

/* 删除按钮（初始态） */
.btn-delete {
  display: flex; align-items: center; justify-content: center;
  width: 22px; height: 22px;
  border: none; background: transparent;
  color: var(--color-text-3);
  border-radius: var(--radius-sm);
  cursor: pointer; padding: 0;
  transition: background var(--duration-fast), color var(--duration-fast);
}
.btn-delete:hover { background: var(--color-paper-4); color: var(--color-danger); }

/* 二次确认态 */
.confirm-label {
  font-size: 11px; color: var(--color-danger);
  font-weight: 500; white-space: nowrap;
  padding: 0 2px;
}
.confirm-yes, .confirm-no {
  display: flex; align-items: center; justify-content: center;
  width: 20px; height: 20px;
  border: none; border-radius: var(--radius-sm);
  cursor: pointer; padding: 0;
  transition: background var(--duration-fast), color var(--duration-fast);
}
.confirm-yes {
  background: var(--color-danger); color: #fff;
}
.confirm-yes:hover { opacity: 0.85; }
.confirm-no {
  background: var(--color-paper-3); color: var(--color-text-3);
}
.confirm-no:hover { background: var(--color-paper-4); color: var(--color-text); }

.conv-title :deep(mark) {
  background: var(--color-accent-soft);
  color: var(--color-accent-2);
  border-radius: 2px; padding: 0 1px;
}

/* ── 右键菜单 ── */
.ctx-menu {
  position: fixed; z-index: 9999;
  min-width: 152px;
  background: var(--color-paper);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-lg);
  padding: var(--space-1);
  display: flex; flex-direction: column; gap: 1px;
}
.ctx-item {
  display: flex; align-items: center; gap: var(--space-2);
  padding: var(--space-2) var(--space-3);
  border: none; background: transparent;
  color: var(--color-text-2); font-size: var(--text-sm);
  font-family: var(--font-body); border-radius: var(--radius-md);
  cursor: pointer; width: 100%; text-align: left;
  transition: background var(--duration-fast), color var(--duration-fast);
}
.ctx-item:hover { background: var(--color-paper-3); color: var(--color-text); }
.ctx-item.danger { color: var(--color-danger); }
.ctx-item.danger:hover { background: oklch(0.96 0.02 25); }
</style>
