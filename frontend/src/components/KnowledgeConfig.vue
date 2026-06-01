<template>
  <div class="kb-panel">
    <!-- 列表视图 -->
    <template v-if="!activeKB">
      <div class="kb-header">
        <h3 class="kb-title">知识库</h3>
        <button class="btn-primary" @click="showCreate = true">新建知识库</button>
      </div>

      <!-- 新建表单 -->
      <div v-if="showCreate" class="kb-create-form">
        <input v-model="newName" class="kb-input" placeholder="知识库名称（必填）" maxlength="64" @keydown.enter="createKB" />
        <input v-model="newDesc" class="kb-input" placeholder="描述（可选）" maxlength="256" />
        <div class="kb-create-actions">
          <button class="btn-primary" :disabled="!newName.trim()" @click="createKB">创建</button>
          <button class="btn-ghost" @click="showCreate = false; newName = ''; newDesc = ''">取消</button>
        </div>
        <p v-if="createError" class="kb-error">{{ createError }}</p>
      </div>

      <div v-if="kbs.length === 0 && !showCreate" class="kb-empty">
        <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1" stroke-linecap="round" opacity="0.3"><path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20"/><path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z"/></svg>
        <p>还没有知识库，点击「新建知识库」开始</p>
      </div>

      <div class="kb-list">
        <div v-for="kb in kbs" :key="kb.id" class="kb-card" @click="openKB(kb)">
          <div class="kb-card-icon">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20"/><path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z"/></svg>
          </div>
          <div class="kb-card-info">
            <div class="kb-card-name">{{ kb.name }}</div>
            <div class="kb-card-meta">{{ kb.description || '暂无描述' }} · {{ kb.doc_count }} 个文档</div>
          </div>
          <button class="kb-card-delete" title="删除知识库" @click.stop="deleteKB(kb.id, kb.name)">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14H6L5 6"/><path d="M10 11v6M14 11v6"/><path d="M9 6V4h6v2"/></svg>
          </button>
        </div>
      </div>
    </template>

    <!-- 详情视图 -->
    <template v-else>
      <div class="kb-header">
        <button class="btn-back" @click="activeKB = null; refreshList()">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><polyline points="15 18 9 12 15 6"/></svg>
          返回
        </button>
        <h3 class="kb-title">{{ activeKB.name }}</h3>
        <button class="btn-primary" @click="uploadDocs">上传文件</button>
      </div>

      <div v-if="uploadError" class="kb-error" style="margin-bottom:8px">{{ uploadError }}</div>

      <div v-if="docs.length === 0" class="kb-empty">
        <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1" stroke-linecap="round" opacity="0.3"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>
        <p>还没有文档，点击「上传文件」添加</p>
      </div>

      <div class="doc-list">
        <div v-for="doc in docs" :key="doc.id" class="doc-row">
          <div class="doc-icon">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>
          </div>
          <div class="doc-info">
            <div class="doc-name">{{ doc.name }}</div>
            <div class="doc-meta">{{ formatSize(doc.size) }} · {{ doc.chunk_count }} 块</div>
          </div>
          <div class="doc-status" :class="'status-' + doc.status">
            <span v-if="doc.status === 'processing'" class="status-spinner"></span>
            {{ statusLabel(doc.status) }}
          </div>
          <button class="doc-delete" title="删除文档" @click="deleteDoc(doc.id, doc.name)">
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14H6L5 6"/><path d="M9 6V4h6v2"/></svg>
          </button>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import {
  ListKnowledgeBases, CreateKnowledgeBase, DeleteKnowledgeBase,
  ListDocuments, PickAndUploadDocuments, DeleteDocument, GetDocumentStatus
} from '../../wailsjs/go/handler/KnowledgeHandler'
import type { storage, kb } from '../../wailsjs/go/models'

type KB = storage.KnowledgeBase
type Doc = kb.KBDocument

const kbs = ref<KB[]>([])
const activeKB = ref<KB | null>(null)
const docs = ref<Doc[]>([])
const showCreate = ref(false)
const newName = ref('')
const newDesc = ref('')
const createError = ref('')
const uploadError = ref('')

let pollTimer: ReturnType<typeof setInterval> | null = null

onMounted(() => refreshList())
onUnmounted(() => stopPoll())

async function refreshList() {
  const list = await ListKnowledgeBases().catch(() => [])
  kbs.value = list ?? []
}

async function createKB() {
  createError.value = ''
  if (!newName.value.trim()) return
  const result = await CreateKnowledgeBase(newName.value.trim(), newDesc.value.trim()).catch((e: any) => {
    createError.value = String(e)
    return null
  })
  if (!result) return
  showCreate.value = false
  newName.value = ''
  newDesc.value = ''
  await refreshList()
}

async function deleteKB(id: string, name: string) {
  if (!confirm(`确定删除知识库「${name}」及其所有文档？此操作不可撤销。`)) return
  await DeleteKnowledgeBase(id).catch(() => null)
  await refreshList()
}

async function openKB(kb: KB) {
  activeKB.value = kb
  await loadDocs()
}

async function loadDocs() {
  if (!activeKB.value) return
  const list = await ListDocuments(activeKB.value.id).catch(() => [])
  docs.value = list ?? []
  const hasProcessing = docs.value.some(d => d.status === 'processing')
  if (hasProcessing) startPoll()
  else stopPoll()
}

async function uploadDocs() {
  if (!activeKB.value) return
  uploadError.value = ''
  const newDocs = await PickAndUploadDocuments(activeKB.value.id).catch((e: any) => {
    uploadError.value = String(e)
    return null
  })
  if (!newDocs) return
  if (newDocs.length > 0) {
    docs.value = [...docs.value, ...newDocs]
    startPoll()
  }
}

async function deleteDoc(docID: string, name: string) {
  if (!activeKB.value) return
  if (!confirm(`确定删除文档「${name}」？`)) return
  await DeleteDocument(activeKB.value.id, docID).catch(() => null)
  await loadDocs()
}

function startPoll() {
  if (pollTimer) return
  pollTimer = setInterval(async () => {
    if (!activeKB.value) { stopPoll(); return }
    const processing = docs.value.filter(d => d.status === 'processing')
    if (processing.length === 0) { stopPoll(); return }
    for (const doc of processing) {
      const status = await GetDocumentStatus(activeKB.value.id, doc.id).catch(() => 'error')
      const idx = docs.value.findIndex(d => d.id === doc.id)
      if (idx >= 0) {
        docs.value[idx] = { ...docs.value[idx], status }
        if (status === 'ready') {
          await loadDocs()
          return
        }
      }
    }
  }, 2000)
}

function stopPoll() {
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
}

function statusLabel(s: string) {
  return ({ pending: '等待中', processing: '处理中', ready: '就绪', error: '错误' } as Record<string,string>)[s] ?? s
}

function formatSize(bytes: number) {
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / 1024 / 1024).toFixed(1) + ' MB'
}
</script>

<style scoped>
.kb-panel { display: flex; flex-direction: column; gap: var(--space-4); height: 100%; }
.kb-header { display: flex; align-items: center; gap: var(--space-3); }
.kb-title { flex: 1; font-size: var(--text-base); font-weight: 600; color: var(--color-text); margin: 0; }
.kb-empty { display: flex; flex-direction: column; align-items: center; gap: var(--space-3); padding: var(--space-10) 0; color: var(--color-text-3); font-size: var(--text-sm); }
.kb-list { display: flex; flex-direction: column; gap: var(--space-2); }
.kb-card { display: flex; align-items: center; gap: var(--space-3); padding: var(--space-3) var(--space-4); border: 1px solid var(--color-border); border-radius: var(--radius-md); cursor: pointer; transition: background var(--duration-fast); }
.kb-card:hover { background: var(--color-paper-2); }
.kb-card-icon { color: var(--color-accent); flex-shrink: 0; }
.kb-card-info { flex: 1; min-width: 0; }
.kb-card-name { font-size: var(--text-sm); font-weight: 500; color: var(--color-text); }
.kb-card-meta { font-size: var(--text-xs); color: var(--color-text-3); margin-top: 2px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.kb-card-delete { flex-shrink: 0; padding: var(--space-1); border: none; background: transparent; color: var(--color-text-3); cursor: pointer; border-radius: var(--radius-sm); opacity: 0; transition: opacity var(--duration-fast); }
.kb-card:hover .kb-card-delete { opacity: 1; }
.kb-card-delete:hover { color: oklch(0.55 0.18 25); background: oklch(0.95 0.03 25); }
.kb-create-form { display: flex; flex-direction: column; gap: var(--space-2); padding: var(--space-4); background: var(--color-paper-2); border-radius: var(--radius-md); border: 1px solid var(--color-border); }
.kb-input { padding: var(--space-2) var(--space-3); border: 1px solid var(--color-border); border-radius: var(--radius-md); background: var(--color-paper); color: var(--color-text); font-size: var(--text-sm); font-family: inherit; outline: none; }
.kb-input:focus { border-color: var(--color-accent); }
.kb-create-actions { display: flex; gap: var(--space-2); }
.kb-error { font-size: var(--text-xs); color: oklch(0.55 0.18 25); }
.btn-back { display: flex; align-items: center; gap: var(--space-1); padding: var(--space-1) var(--space-2); border: none; background: transparent; color: var(--color-text-2); cursor: pointer; font-size: var(--text-sm); border-radius: var(--radius-sm); }
.btn-back:hover { background: var(--color-paper-3); color: var(--color-text); }
.doc-list { display: flex; flex-direction: column; gap: var(--space-1); }
.doc-row { display: flex; align-items: center; gap: var(--space-3); padding: var(--space-2) var(--space-3); border-radius: var(--radius-md); }
.doc-row:hover { background: var(--color-paper-2); }
.doc-icon { color: var(--color-text-3); flex-shrink: 0; }
.doc-info { flex: 1; min-width: 0; }
.doc-name { font-size: var(--text-sm); color: var(--color-text); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.doc-meta { font-size: var(--text-xs); color: var(--color-text-3); margin-top: 1px; }
.doc-status { font-size: var(--text-xs); flex-shrink: 0; display: flex; align-items: center; gap: 4px; }
.status-ready { color: var(--color-success); }
.status-processing { color: var(--color-warning); }
.status-error { color: oklch(0.55 0.18 25); }
.status-pending { color: var(--color-text-3); }
.status-spinner { width: 10px; height: 10px; border: 1.5px solid currentColor; border-top-color: transparent; border-radius: 50%; animation: spin 0.8s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
.doc-delete { flex-shrink: 0; padding: var(--space-1); border: none; background: transparent; color: var(--color-text-3); cursor: pointer; border-radius: var(--radius-sm); opacity: 0; transition: opacity var(--duration-fast); }
.doc-row:hover .doc-delete { opacity: 1; }
.doc-delete:hover { color: oklch(0.55 0.18 25); background: oklch(0.95 0.03 25); }
.btn-primary { padding: var(--space-2) var(--space-4); background: var(--color-accent); color: white; border: none; border-radius: var(--radius-md); font-size: var(--text-sm); font-family: inherit; cursor: pointer; transition: background var(--duration-fast); }
.btn-primary:hover:not(:disabled) { background: var(--color-accent-2); }
.btn-primary:disabled { opacity: 0.5; cursor: not-allowed; }
.btn-ghost { padding: var(--space-2) var(--space-4); background: transparent; color: var(--color-text-2); border: 1px solid var(--color-border); border-radius: var(--radius-md); font-size: var(--text-sm); font-family: inherit; cursor: pointer; }
.btn-ghost:hover { background: var(--color-paper-3); }
</style>
