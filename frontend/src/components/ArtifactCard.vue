<script setup lang="ts">
import { computed } from 'vue'
import type { Artifact } from '../utils/artifacts'
import { OpenPath } from '../../wailsjs/go/handler/TaskHandler'
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'

const props = defineProps<{ artifact: Artifact }>()

const a = computed(() => props.artifact)

function baseName(p?: string) {
  if (!p) return ''
  return p.split('/').pop() || p
}

const displayTitle = computed(() => a.value.title || baseName(a.value.path) || a.value.url || '未命名')

// 图标按类型/动作
const icon = computed(() => {
  switch (a.value.type) {
    case 'file': return a.value.action === 'write' ? '📝' : '📄'
    case 'image': return '🖼️'
    case 'url': return '🔗'
    default: return '📎'
  }
})

// 类型标签
const tag = computed(() => {
  if (a.value.type === 'file') return a.value.action === 'write' ? '生成' : '读取'
  if (a.value.type === 'url') return '链接'
  if (a.value.type === 'image') return '图片'
  return a.value.type
})
const tagClass = computed(() => (a.value.type === 'file' && a.value.action === 'write') ? 'write' : a.value.type)

// 副标题（路径/大小 或 url）
const subtitle = computed(() => {
  if (a.value.type === 'file') {
    const parts: string[] = []
    if (a.value.bytes != null) parts.push(`${a.value.bytes} B`)
    if (a.value.path) parts.push(a.value.path)
    return parts.join(' · ')
  }
  return a.value.url || ''
})

const canReveal = computed(() => a.value.type === 'file' && !!a.value.abs_path)

// 点击整卡片：file → 打开文件；url/image → 浏览器打开
function onOpen() {
  if (a.value.type === 'file' && a.value.abs_path) {
    OpenPath(a.value.abs_path, false).catch((e) => console.warn('打开文件失败', e))
  } else if (a.value.url) {
    BrowserOpenURL(a.value.url)
  }
}

// 在文件夹中显示（仅 file）
function onReveal() {
  if (a.value.abs_path) {
    OpenPath(a.value.abs_path, true).catch((e) => console.warn('定位文件失败', e))
  }
}
</script>

<template>
  <div class="artifact-card" :title="a.type === 'file' ? '点击打开文件' : '点击打开'" @click="onOpen">
    <span class="artifact-card__icon">{{ icon }}</span>
    <div class="artifact-card__info">
      <span class="artifact-card__name">
        {{ displayTitle }}
        <span class="artifact-card__tag" :class="tagClass">{{ tag }}</span>
      </span>
      <span v-if="subtitle" class="artifact-card__meta">{{ subtitle }}</span>
    </div>
    <button v-if="canReveal" class="artifact-card__reveal" title="在文件夹中显示" @click.stop="onReveal">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M4 4h6l2 2h8a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2z"/></svg>
    </button>
  </div>
</template>

<style scoped>
.artifact-card {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  background: var(--color-paper-2);
  border: 1px solid var(--color-border);
  border-radius: 10px;
  margin-bottom: 6px;
  cursor: pointer;
  transition: all var(--duration-fast) var(--ease-out);
}
.artifact-card:hover {
  border-color: var(--color-accent);
  background: var(--color-accent-soft);
}
.artifact-card__icon { font-size: 18px; flex-shrink: 0; }
.artifact-card__info { display: flex; flex-direction: column; gap: 2px; min-width: 0; flex: 1; }
.artifact-card__name {
  font-size: 13px; font-weight: 600; color: var(--color-text);
  word-break: break-all; display: flex; align-items: center; gap: 6px;
}
.artifact-card__tag {
  font-size: 10px; font-weight: 500; padding: 0 6px; border-radius: 4px; flex-shrink: 0;
}
.artifact-card__tag.write { background: oklch(0.92 0.06 150); color: oklch(0.42 0.12 150); }
.artifact-card__tag.read { background: oklch(0.92 0.04 250); color: oklch(0.45 0.1 250); }
.artifact-card__tag.url, .artifact-card__tag.image { background: oklch(0.92 0.05 300); color: oklch(0.45 0.12 300); }
.artifact-card__meta { font-size: 11px; color: var(--color-text-3); word-break: break-all; }
.artifact-card__reveal {
  flex-shrink: 0; border: none; background: transparent; color: var(--color-text-3);
  cursor: pointer; padding: 4px; border-radius: var(--radius-sm);
  display: flex; align-items: center; transition: all var(--duration-fast);
}
.artifact-card__reveal:hover { color: var(--color-accent); background: var(--color-paper); }
</style>
