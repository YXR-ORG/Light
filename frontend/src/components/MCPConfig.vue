<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { List, Save, Delete, Toggle, TestConnection } from '../../wailsjs/go/handler/MCPHandler'
import type { storage } from '../../wailsjs/go/models'

type MCPServer = storage.MCPServer

const servers = ref<MCPServer[]>([])
const showForm = ref(false)
const saving = ref(false)
const testing = ref(false)
const testResult = ref<string | null>(null)
const testError = ref<string | null>(null)

const form = ref<MCPServer>({
  id: '', name: '', type: 'sse', url: '', command: '',
  args: '', env: '', enabled: true, created_at: '', updated_at: '',
})

onMounted(loadServers)

async function loadServers() {
  servers.value = await List().catch(() => [])
}

function openForm(server?: MCPServer) {
  form.value = server
    ? { ...server }
    : { id: '', name: '', type: 'sse', url: '', command: '', args: '', env: '', enabled: true, created_at: '', updated_at: '' }
  testResult.value = null
  testError.value = null
  showForm.value = true
}

function cancelForm() {
  showForm.value = false
  testResult.value = null
  testError.value = null
}

async function saveServer() {
  if (!form.value.name.trim()) return
  saving.value = true
  try {
    await Save(form.value)
    await loadServers()
    showForm.value = false
  } catch (e: any) {
    console.error('保存失败', e)
  } finally {
    saving.value = false
  }
}

async function deleteServer(id: string) {
  await Delete(id).catch(console.error)
  await loadServers()
}

async function toggleServer(server: MCPServer) {
  await Toggle(server.id, !server.enabled).catch(console.error)
  await loadServers()
}

async function testConnection() {
  testing.value = true
  testResult.value = null
  testError.value = null
  try {
    const tools = await TestConnection(form.value)
    testResult.value = tools.length > 0 ? tools.join(', ') : '连接成功（无工具）'
  } catch (e: any) {
    testError.value = String(e)
  } finally {
    testing.value = false
  }
}
</script>

<template>
  <div class="mcp-config">
    <!-- 列表视图 -->
    <div v-if="!showForm">
      <div v-if="servers.length === 0" class="empty-state">
        <div class="empty-icon">⚡</div>
        <p>暂无 MCP 服务器</p>
        <p class="hint">添加 SSE 或 Stdio 类型的 MCP 服务器以启用工具调用</p>
      </div>
      <div v-else class="server-list">
        <div v-for="s in servers" :key="s.id" class="server-item">
          <div class="server-info">
            <span class="server-name">{{ s.name }}</span>
            <span class="server-type">{{ s.type.toUpperCase() }}</span>
            <span class="server-addr">{{ s.type === 'sse' ? s.url : s.command }}</span>
          </div>
          <div class="server-actions">
            <button class="btn-icon" title="编辑" @click="openForm(s)">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
                <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/>
                <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/>
              </svg>
            </button>
            <button class="btn-icon btn-danger" title="删除" @click="deleteServer(s.id)">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
                <polyline points="3 6 5 6 21 6"/>
                <path d="M19 6l-1 14H6L5 6"/>
                <path d="M10 11v6M14 11v6M9 6V4h6v2"/>
              </svg>
            </button>
            <label class="toggle" :title="s.enabled ? '禁用' : '启用'">
              <input type="checkbox" :checked="s.enabled" @change="toggleServer(s)" />
              <span class="slider" />
            </label>
          </div>
        </div>
      </div>
      <button class="btn-add" @click="openForm()">+ 添加 MCP 服务器</button>
    </div>

    <!-- 添加/编辑表单 -->
    <div v-else class="server-form">
      <div class="field">
        <label>名称</label>
        <input v-model="form.name" placeholder="我的 MCP 服务器" />
      </div>
      <div class="field">
        <label>类型</label>
        <div class="type-grid">
          <button class="type-btn" :class="{ active: form.type === 'sse' }" @click="form.type = 'sse'">
            SSE
            <span class="type-desc">HTTP 服务器</span>
          </button>
          <button class="type-btn" :class="{ active: form.type === 'stdio' }" @click="form.type = 'stdio'">
            Stdio
            <span class="type-desc">本地进程</span>
          </button>
        </div>
      </div>
      <div v-if="form.type === 'sse'" class="field">
        <label>URL</label>
        <input v-model="form.url" placeholder="http://localhost:3000/sse" />
      </div>
      <template v-else>
        <div class="field">
          <label>命令</label>
          <input v-model="form.command" placeholder="npx" />
        </div>
        <div class="field">
          <label>参数 <span class="optional">空格分隔或 JSON 数组</span></label>
          <input v-model="form.args" placeholder="-y @modelcontextprotocol/server-filesystem /" />
        </div>
      </template>
      <div class="field">
        <label>环境变量 <span class="optional">JSON 对象，可选</span></label>
        <input v-model="form.env" placeholder='{"API_KEY": "xxx"}' />
      </div>

      <div v-if="testResult" class="test-result success">✓ 工具列表: {{ testResult }}</div>
      <div v-if="testError" class="test-result error">✗ 连接失败: {{ testError }}</div>

      <div class="form-actions">
        <button class="btn btn-test" @click="testConnection" :disabled="testing">
          {{ testing ? '测试中...' : '测试连接' }}
        </button>
        <div class="form-actions-right">
          <button class="btn btn-cancel" @click="cancelForm">取消</button>
          <button class="btn btn-primary" @click="saveServer" :disabled="saving">
            {{ saving ? '保存中...' : '保存' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.mcp-config { display: flex; flex-direction: column; gap: var(--space-4); }

.empty-state {
  text-align: center;
  padding: var(--space-8) 0;
  color: var(--color-text-3);
  font-size: var(--text-sm);
}
.empty-icon { font-size: 32px; margin-bottom: var(--space-3); }
.empty-state p { margin: 0; }
.empty-state .hint { margin-top: var(--space-2); font-size: var(--text-xs); }

.server-list { display: flex; flex-direction: column; gap: var(--space-2); margin-bottom: var(--space-3); }

.server-item {
  display: flex; align-items: center; justify-content: space-between;
  padding: var(--space-3) var(--space-4);
  border: 1px solid var(--color-border); border-radius: var(--radius-md);
  background: var(--color-paper-2);
}

.server-info { display: flex; align-items: center; gap: var(--space-2); min-width: 0; flex: 1; overflow: hidden; }
.server-name { font-size: var(--text-sm); font-weight: 500; color: var(--color-text); white-space: nowrap; }
.server-type {
  font-size: var(--text-xs); padding: 2px 6px; flex-shrink: 0;
  background: var(--color-accent-soft); color: var(--color-accent);
  border-radius: var(--radius-sm); font-weight: 600;
}
.server-addr {
  font-size: var(--text-xs); color: var(--color-text-3);
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
}

.server-actions { display: flex; align-items: center; gap: var(--space-2); flex-shrink: 0; margin-left: var(--space-2); }

.btn-icon {
  width: 28px; height: 28px; display: flex; align-items: center; justify-content: center;
  border: none; border-radius: var(--radius-md); background: transparent;
  color: var(--color-text-3); cursor: pointer;
  transition: background var(--duration-fast) var(--ease-out), color var(--duration-fast) var(--ease-out);
}
.btn-icon:hover { background: var(--color-paper-3); color: var(--color-text); }
.btn-icon.btn-danger:hover { background: oklch(0.95 0.05 25); color: var(--color-danger); }

.toggle { position: relative; display: inline-block; width: 36px; height: 20px; cursor: pointer; flex-shrink: 0; }
.toggle input { opacity: 0; width: 0; height: 0; position: absolute; }
.slider {
  position: absolute; inset: 0; background: var(--color-border-2);
  border-radius: var(--radius-full);
  transition: background var(--duration-fast) var(--ease-out);
}
.slider::before {
  content: ''; position: absolute;
  width: 14px; height: 14px; left: 3px; top: 3px;
  background: white; border-radius: 50%;
  transition: transform var(--duration-fast) var(--ease-out);
}
.toggle input:checked + .slider { background: var(--color-accent); }
.toggle input:checked + .slider::before { transform: translateX(16px); }

.btn-add {
  width: 100%; padding: var(--space-2) var(--space-3);
  border: 1px dashed var(--color-border-2); border-radius: var(--radius-md);
  background: transparent; color: var(--color-text-3); font-size: var(--text-sm);
  font-family: inherit; cursor: pointer;
  transition: border-color var(--duration-fast) var(--ease-out), color var(--duration-fast) var(--ease-out);
}
.btn-add:hover { border-color: var(--color-accent); color: var(--color-accent); }

.server-form { display: flex; flex-direction: column; gap: var(--space-3); }

.field label {
  display: block; font-size: var(--text-xs); font-weight: 500;
  color: var(--color-text-2); margin-bottom: var(--space-1);
}
.optional { color: var(--color-text-3); font-weight: 400; }

.field input {
  width: 100%; padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border); border-radius: var(--radius-md);
  font-size: var(--text-sm); font-family: inherit; color: var(--color-text);
  background: var(--color-paper); outline: none;
  transition: border-color var(--duration-fast) var(--ease-out);
  box-sizing: border-box;
}
.field input:focus {
  border-color: var(--color-accent);
  box-shadow: 0 0 0 3px var(--color-accent-soft);
}

.type-grid {
  display: grid; grid-template-columns: 1fr 1fr; gap: var(--space-2);
}
.type-btn {
  display: flex; flex-direction: column; align-items: center; justify-content: center;
  gap: 2px;
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border); border-radius: var(--radius-md);
  background: var(--color-paper); color: var(--color-text-2);
  font-size: var(--text-sm); font-family: inherit; font-weight: 500;
  cursor: pointer;
  transition: border-color var(--duration-fast) var(--ease-out),
              background var(--duration-fast) var(--ease-out),
              color var(--duration-fast) var(--ease-out);
}
.type-btn:hover { border-color: var(--color-accent); color: var(--color-text); }
.type-btn.active {
  border-color: var(--color-accent); background: var(--color-accent-soft);
  color: var(--color-accent);
}
.type-desc {
  font-size: var(--text-xs); font-weight: 400;
  color: var(--color-text-3);
}
.type-btn.active .type-desc { color: var(--color-accent); opacity: 0.8; }

.test-result {
  padding: var(--space-2) var(--space-3); border-radius: var(--radius-md);
  font-size: var(--text-xs); word-break: break-all;
}
.test-result.success { background: oklch(0.95 0.05 145); color: var(--color-success); }
.test-result.error { background: oklch(0.95 0.05 25); color: var(--color-danger); }

.form-actions {
  display: flex; align-items: center; justify-content: space-between;
  padding-top: var(--space-2);
}
.form-actions-right { display: flex; gap: var(--space-2); }

.btn {
  padding: var(--space-2) var(--space-4); border-radius: var(--radius-md);
  font-size: var(--text-sm); font-family: inherit; cursor: pointer; border: none;
  transition: background var(--duration-fast) var(--ease-out), opacity var(--duration-fast) var(--ease-out);
}
.btn-test { background: var(--color-paper-3); color: var(--color-text-2); }
.btn-test:hover:not(:disabled) { background: var(--color-paper-4); }
.btn-cancel { background: var(--color-paper-3); color: var(--color-text-2); }
.btn-cancel:hover { background: var(--color-paper-4); }
.btn-primary { background: var(--color-accent); color: #fff; }
.btn-primary:hover:not(:disabled) { background: var(--color-accent-2); }
.btn:disabled { opacity: 0.5; cursor: not-allowed; }
</style>
