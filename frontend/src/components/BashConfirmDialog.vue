<script setup lang="ts">
/**
 * BashConfirmDialog — 全局单例危险命令确认框
 *
 * 用法：在 App.vue 放一个 <BashConfirmDialog />，监听 task:bash_confirm 事件，
 * 用户选择后调用后端 TaskHandler.ConfirmBash。
 */
import { ref, onMounted, onUnmounted } from 'vue'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import { ConfirmBash } from '../../wailsjs/go/handler/TaskHandler'

interface BashConfirmPayload {
  conv_id: string
  confirm_id: string
  cmd: string
}

const visible = ref(false)
const convId = ref('')
const confirmId = ref('')
const cmd = ref('')
const loading = ref(false)

function onEvent(payload: BashConfirmPayload) {
  convId.value = payload.conv_id
  confirmId.value = payload.confirm_id
  cmd.value = payload.cmd
  visible.value = true
  loading.value = false
}

async function choose(approved: boolean) {
  if (loading.value) return
  loading.value = true
  try {
    await ConfirmBash(convId.value, confirmId.value, approved)
  } catch (e) {
    console.error('ConfirmBash error', e)
  } finally {
    visible.value = false
    loading.value = false
  }
}

onMounted(() => EventsOn('task:bash_confirm', onEvent))
onUnmounted(() => EventsOff('task:bash_confirm'))
</script>

<template>
  <Teleport to="body">
    <div v-if="visible" class="bash-overlay" @click.self="choose(false)">
      <div class="bash-dialog" role="alertdialog" aria-modal="true">

        <div class="bash-dialog__header">
          <span class="bash-dialog__icon">⚠️</span>
          <span class="bash-dialog__title">危险命令确认</span>
        </div>

        <p class="bash-dialog__desc">Agent 即将执行以下命令，请确认是否允许：</p>

        <pre class="bash-dialog__cmd">{{ cmd }}</pre>

        <div class="bash-dialog__actions">
          <button
            class="bash-dialog__btn bash-dialog__btn--deny"
            :disabled="loading"
            @click="choose(false)"
          >
            拒绝
          </button>
          <button
            class="bash-dialog__btn bash-dialog__btn--allow"
            :disabled="loading"
            @click="choose(true)"
          >
            {{ loading ? '处理中…' : '允许执行' }}
          </button>
        </div>

      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.bash-overlay {
  position: fixed;
  inset: 0;
  background: oklch(0 0 0 / 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 9999;
  backdrop-filter: blur(2px);
}

.bash-dialog {
  background: var(--color-surface, #fff);
  border: 1px solid oklch(0.85 0.05 50);
  border-radius: 12px;
  padding: 24px;
  width: min(480px, 92vw);
  box-shadow: 0 8px 32px oklch(0 0 0 / 0.2);
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.bash-dialog__header {
  display: flex;
  align-items: center;
  gap: 8px;
}

.bash-dialog__icon {
  font-size: 20px;
}

.bash-dialog__title {
  font-size: 16px;
  font-weight: 600;
  color: oklch(0.45 0.15 50);
}

.bash-dialog__desc {
  font-size: 14px;
  color: var(--color-text-secondary, oklch(0.4 0 0));
  margin: 0;
}

.bash-dialog__cmd {
  background: oklch(0.12 0 0);
  color: oklch(0.85 0.08 150);
  border-radius: 8px;
  padding: 12px 14px;
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  font-size: 13px;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 200px;
  overflow-y: auto;
  margin: 0;
}

.bash-dialog__actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 4px;
}

.bash-dialog__btn {
  padding: 8px 20px;
  border-radius: 8px;
  font-size: 14px;
  font-weight: 500;
  cursor: pointer;
  border: none;
  transition: opacity 0.15s;
}

.bash-dialog__btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.bash-dialog__btn--deny {
  background: var(--color-surface-2, oklch(0.93 0 0));
  color: var(--color-text, oklch(0.2 0 0));
}

.bash-dialog__btn--allow {
  background: oklch(0.55 0.18 50);
  color: #fff;
}

.bash-dialog__btn--allow:not(:disabled):hover {
  opacity: 0.85;
}
</style>
