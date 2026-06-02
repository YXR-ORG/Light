<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ListFavorites } from '../../wailsjs/go/handler/ConversationHandler'
import type { storage } from '../../wailsjs/go/models'

const emit = defineEmits<{
  close: []
  select: [id: string]
}>()

const favorites = ref<storage.Conversation[]>([])
const loading = ref(true)

onMounted(async () => {
  try {
    favorites.value = await ListFavorites()
  } finally {
    loading.value = false
  }
})

function onOverlayClick(e: MouseEvent) {
  if (e.target === e.currentTarget) emit('close')
}
</script>

<template>
  <Teleport to="body">
    <div class="fav-overlay" @mousedown="onOverlayClick">
      <div class="fav-dialog">
        <div class="fav-header">
          <div class="fav-title">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"/></svg>
            我的收藏
          </div>
          <button class="fav-close" @click="emit('close')">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M18 6 6 18M6 6l12 12"/></svg>
          </button>
        </div>

        <div class="fav-body">
          <div v-if="loading" class="fav-hint">加载中…</div>
          <div v-else-if="!favorites.length" class="fav-hint">
            <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round" style="opacity:.3"><polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"/></svg>
            <p>还没有收藏的对话</p>
            <p class="fav-hint-sub">在左侧对话列表 hover 后点击 ☆ 即可收藏</p>
          </div>
          <div v-else class="fav-list">
            <button
              v-for="c in favorites"
              :key="c.id"
              class="fav-item"
              @click="emit('select', c.id)"
            >
              <div class="fav-item-icon">
                <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>
              </div>
              <div class="fav-item-body">
                <div class="fav-item-title">{{ c.title }}</div>
                <div class="fav-item-meta">{{ c.provider }} · {{ c.model }}</div>
              </div>
              <svg class="fav-star" width="12" height="12" viewBox="0 0 24 24" fill="currentColor" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"/></svg>
            </button>
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.fav-overlay {
  position: fixed; inset: 0;
  z-index: 1000;
  background: oklch(0 0 0 / 0.35);
  display: flex; align-items: center; justify-content: center;
  backdrop-filter: blur(2px);
}

.fav-dialog {
  width: 440px;
  max-height: 70vh;
  background: var(--color-paper);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-xl);
  box-shadow: var(--shadow-lg);
  display: flex; flex-direction: column;
  overflow: hidden;
}

.fav-header {
  display: flex; align-items: center; justify-content: space-between;
  padding: var(--space-4) var(--space-5);
  border-bottom: 1px solid var(--color-border);
  flex-shrink: 0;
}

.fav-title {
  display: flex; align-items: center; gap: var(--space-2);
  font-size: var(--text-base); font-weight: 600;
  color: var(--color-text);
}

.fav-close {
  display: flex; align-items: center; justify-content: center;
  width: 28px; height: 28px;
  border: none; background: transparent;
  color: var(--color-text-3); border-radius: var(--radius-md);
  cursor: pointer;
  transition: background var(--duration-fast), color var(--duration-fast);
}
.fav-close:hover { background: var(--color-paper-3); color: var(--color-text); }

.fav-body {
  flex: 1; overflow-y: auto;
  padding: var(--space-2);
}

.fav-hint {
  display: flex; flex-direction: column; align-items: center;
  gap: var(--space-2);
  padding: var(--space-10) var(--space-4);
  color: var(--color-text-3);
  font-size: var(--text-sm);
  text-align: center;
}
.fav-hint p { margin: 0; }
.fav-hint-sub { font-size: var(--text-xs); opacity: .7; }

.fav-list { display: flex; flex-direction: column; gap: 2px; }

.fav-item {
  display: flex; align-items: center; gap: var(--space-3);
  padding: var(--space-3) var(--space-3);
  border: none; background: transparent;
  border-radius: var(--radius-md);
  cursor: pointer; width: 100%; text-align: left;
  font-family: var(--font-body);
  transition: background var(--duration-fast);
}
.fav-item:hover { background: var(--color-paper-3); }

.fav-item-icon {
  flex-shrink: 0;
  width: 30px; height: 30px;
  display: flex; align-items: center; justify-content: center;
  border-radius: var(--radius-md);
  background: var(--color-paper-3);
  color: var(--color-text-2);
}

.fav-item-body { flex: 1; min-width: 0; }
.fav-item-title {
  font-size: var(--text-sm); font-weight: 500;
  color: var(--color-text);
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
}
.fav-item-meta { font-size: var(--text-xs); color: var(--color-text-3); margin-top: 2px; }

.fav-star { flex-shrink: 0; color: oklch(0.78 0.18 75); }
</style>
