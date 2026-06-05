// 通用产物（Artifact）机制 —— 前端解析层
//
// 工具在返回给 LLM 的文本里夹带标记：<!--ARTIFACT:base64(json)-->
// 前端从 tool_result 中解析这些标记，自动在“产物区”渲染对应卡片。
// 新工具只要在后端 EmbedArtifact，前端零改动即可自动展示。

export interface PlanStep {
  content: string
  status?: string   // pending | in_progress | done
}

export interface Artifact {
  type: string          // file | image | url | plan | ...（可扩展）
  action?: string       // write | read（file 专用）
  title?: string        // 展示标题
  path?: string         // 相对路径（file）
  abs_path?: string     // 绝对路径（file，用于打开/定位）
  url?: string          // 链接（url/image）
  bytes?: number        // 字节大小
  mime?: string         // MIME 类型
  plan_id?: string      // plan 去重标识
  steps?: PlanStep[]    // plan 步骤
}

export interface TaskArtifacts {
  plans: Artifact[]
  files: Artifact[]
  others: Artifact[]
}

const ARTIFACT_RE = /<!--ARTIFACT:([A-Za-z0-9+/=]*)-->/g

// base64 → UTF-8 字符串（兼容中文等多字节内容）
function decodeBase64Utf8(b64: string): string {
  const bin = atob(b64)
  const bytes = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
  return new TextDecoder('utf-8').decode(bytes)
}

// 从一段工具结果文本中解析出所有产物
export function parseArtifacts(text?: string): Artifact[] {
  if (!text) return []
  const out: Artifact[] = []
  // 用 matchAll 避免 lastIndex 状态问题
  for (const m of text.matchAll(ARTIFACT_RE)) {
    try {
      const json = decodeBase64Utf8(m[1])
      const a = JSON.parse(json) as Artifact
      if (a && a.type) out.push(a)
    } catch {
      // 忽略解析失败的标记
    }
  }
  return out
}

// 移除文本中的所有产物标记（用于纯文本展示，避免显示难看的注释）
export function stripArtifacts(text?: string): string {
  if (!text) return ''
  return text.replace(ARTIFACT_RE, '').trim()
}

// 从多个 tool_result 收集产物，按去重键去重。
//   - plan：按 plan_id 去重，总是保留最新一次（后者覆盖）
//   - file：按 abs_path，write 优先覆盖 read
//   - url/其他：按 url / title
export function collectArtifacts(results: (string | undefined)[]): Artifact[] {
  const map = new Map<string, Artifact>()
  for (const r of results) {
    for (const a of parseArtifacts(r)) {
      if (a.type === 'plan') {
        // plan 始终保留最新（按 plan_id，无则用固定键）
        map.set('plan:' + (a.plan_id || 'default'), a)
        continue
      }
      const key = a.abs_path || a.url || a.title || JSON.stringify(a)
      const existing = map.get(key)
      // write 优先覆盖 read
      if (!existing || (a.action === 'write' && existing.action !== 'write')) {
        map.set(key, a)
      }
    }
  }
  return Array.from(map.values())
}

function artifactType(a: Artifact): string {
  return String(a.type || '').trim().toLowerCase()
}

export function splitTaskArtifacts(artifacts: Artifact[]): TaskArtifacts {
  const plans: Artifact[] = []
  const files: Artifact[] = []
  const others: Artifact[] = []

  for (const a of artifacts) {
    const t = artifactType(a)
    if (t === 'plan') plans.push(a)
    else if (t === 'file') files.push(a)
    else others.push(a)
  }

  return { plans, files, others }
}
