<script setup lang="ts">
import { computed } from 'vue'
import { buildPreviewSrcdoc } from '../utils/htmlPreview'

const props = defineProps<{ html: string }>()
const emit = defineEmits<{ close: [] }>()
const srcdoc = computed(() => buildPreviewSrcdoc(props.html))
</script>

<template>
  <Teleport to="body">
    <div class="html-preview-mask" @click.self="emit('close')">
      <div class="html-preview-dialog">
        <div class="html-preview-header">
          <span>HTML 预览</span>
          <button class="html-preview-close" @click="emit('close')" title="关闭">×</button>
        </div>
        <iframe class="html-preview-frame" sandbox="allow-scripts" :srcdoc="srcdoc" />
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.html-preview-mask {
  position: fixed;
  inset: 0;
  z-index: 10000;
  background: oklch(0 0 0 / 0.42);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--space-6);
}
.html-preview-dialog {
  width: min(960px, 92vw);
  height: min(720px, 82vh);
  background: var(--color-paper);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-lg);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.html-preview-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--space-3) var(--space-4);
  border-bottom: 1px solid var(--color-border);
  font-size: var(--text-sm);
  font-weight: 600;
}
.html-preview-close {
  border: none;
  background: transparent;
  color: var(--color-text-3);
  cursor: pointer;
  font-size: 22px;
  line-height: 1;
}
.html-preview-close:hover { color: var(--color-text); }
.html-preview-frame {
  flex: 1;
  width: 100%;
  border: none;
  background: white;
}
</style>
