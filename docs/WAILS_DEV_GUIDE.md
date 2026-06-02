# Wails 桌面客户端开发参考手册

> 基于 Light（AI 对话客户端）开发经验总结。面向下一个 Wails 项目（如笔记软件、工具箱等），纯技术视角，不涉及业务逻辑。

---

## 一、技术栈选型

| 层 | 选型 | 版本 | 说明 |
|----|------|------|------|
| 桌面框架 | Wails v2 | v2.12.0 | Go + WebView，跨平台 |
| AI 框架 | eino | v0.9.2 | CloudWeGo 出品，统一 LLM 接口 |
| AI 扩展 | eino-ext | 各组件独立版本 | 优先用 eino-ext，不要自己封装 |
| 前端框架 | Vue 3 + TypeScript | Vite 构建 | Composition API + `<script setup>` |
| 状态管理 | Pinia | - | 替代 Vuex，更简洁 |
| 数据库 | SQLite + GORM | gorm v1.31 | 本地持久化 |
| SQLite 驱动 | mattn/go-sqlite3 | v1.14.22 | CGO，必须 `-tags fts5` |

---

## 二、项目结构约定

```
├── main.go                    # 入口：Wails.Run + Bind handlers
├── app.go                     # App 结构体：startup 生命周期
├── wails.json                 # Wails 配置
├── Makefile                   # 构建脚本
├── go.mod
├── internal/
│   ├── handler/               # 所有暴露给前端的 Handler（每个域一个文件）
│   ├── storage/               # 数据库：models.go + 各域 CRUD
│   └── <业务包>/              # 纯 Go 业务逻辑，不依赖 Wails
├── frontend/
│   ├── src/
│   │   ├── components/        # Vue 组件
│   │   ├── stores/            # Pinia stores
│   │   └── assets/            # CSS tokens 等
│   └── wailsjs/               # 自动生成，不要手动修改
│       ├── go/                # Go handler 的 TS bindings
│       └── runtime/           # Wails runtime (EventsOn/Emit 等)
└── build/
    ├── appicon.png            # 应用图标
    ├── darwin/                # macOS Info.plist
    ├── windows/               # Windows 资源
    └── models/                # 大文件（如 ONNX 模型），不进 git
```

---

## 三、Handler 设计

### 3.1 基本规则

Handler 是 Go 和前端之间的桥梁，遵循以下规则：

```go
// ✅ 正确：普通方法，无 context.Context 参数
func (h *NoteHandler) GetNote(id string) (*storage.Note, error) { ... }

// ❌ 错误：Wails 不支持 context.Context 作为参数（会 panic）
func (h *NoteHandler) GetNote(ctx context.Context, id string) (*storage.Note, error) { ... }
```

需要 context 的 handler（如流式输出、文件对话框），在 `startup` 时通过 `SetContext` 注入：

```go
// app.go
func (a *App) startup(ctx context.Context) {
    a.ctx = ctx
    a.chatHandler.SetContext(ctx)  // 注入给需要的 handler
}

// handler
type ChatHandler struct { ctx context.Context }
func (h *ChatHandler) SetContext(ctx context.Context) { h.ctx = ctx }
```

### 3.2 注册方式

所有 handler 在 `main.go` 的 `Bind` 数组里统一注册：

```go
wails.Run(&options.App{
    Bind: []interface{}{
        app,
        app.noteHandler,
        app.settingsHandler,
        // ...每增加一个 handler 就加一行
    },
})
```

### 3.3 生成 Bindings

每次修改 Go handler（增删方法、修改签名）后必须重新生成：

```bash
wails generate module
```

生成的文件在 `frontend/wailsjs/go/`，**不要手动修改**。前端直接 import 调用：

```ts
import { GetNote, SaveNote } from '../../wailsjs/go/handler/NoteHandler'
```

---

## 四、流式输出

Wails handler 是请求-响应模式，不支持直接返回 stream。流式输出必须通过**事件系统**实现。

### 4.1 后端（Go）

```go
// 定义 chunk 结构
type StreamChunk struct {
    Content  string `json:"content"`
    Thinking string `json:"thinking"`  // 可选：思考链
    Done     bool   `json:"done"`
    Error    string `json:"error"`
}

func (h *Handler) StreamSomething(req Request) error {
    go func() {
        // 流式循环
        for chunk := range stream {
            runtime.EventsEmit(h.ctx, "stream:chunk", StreamChunk{
                Content: chunk.Text,
            })
        }
        // 结束信号，必须发送
        runtime.EventsEmit(h.ctx, "stream:chunk", StreamChunk{Done: true})
    }()
    return nil  // 立即返回，异步执行
}
```

### 4.2 前端（Vue）

`EventsOn` 返回取消订阅函数，务必在 `onUnmounted` 调用：

```ts
import { EventsOn } from '../../wailsjs/runtime/runtime'

const unsubs: (() => void)[] = []

onMounted(() => {
    unsubs.push(EventsOn('stream:chunk', (chunk) => {
        if (chunk.done) {
            // 流结束，刷新数据
            if (chunk.error) showError(chunk.error)
            reloadData()
            return
        }
        appendContent(chunk.content)
    }))
})

onUnmounted(() => unsubs.forEach(fn => fn()))
```

### 4.3 打字机效果

直接把流式 chunk 拼接到响应式变量上，Vue 自动触发 DOM 更新即可实现打字机效果。如需限速（避免刷新过快），用 `setInterval` 按固定字符数出队：

```ts
const CHARS_PER_TICK = 4
const TICK_MS = 16  // ~60fps

let queue = ''
const displayed = ref('')

function appendToQueue(text: string) {
    queue += text
    startTimer()
}

function tick() {
    if (queue.length > 0) {
        displayed.value += queue.slice(0, CHARS_PER_TICK)
        queue = queue.slice(CHARS_PER_TICK)
    }
}
```

---

## 五、组件间数据同步

### 5.1 用 Pinia store 作为共享状态

组件间需要共享的数据（不只是父子关系），放到 Pinia store，不要用事件总线或 prop 穿透：

```ts
// stores/app.ts
export const useAppStore = defineStore('app', () => {
    const providerMap = ref<Record<string, string>>({})  // id → name
    const items = ref<Item[]>([])

    function setProviderMap(map: Record<string, string>) {
        providerMap.value = map
    }
    return { providerMap, items, setProviderMap }
})
```

### 5.2 跨组件通知刷新

**场景**：设置弹窗修改了某项数据，需要通知其他组件（如 Sidebar）刷新。

用前端的 Wails `EventsEmit` 发事件（纯前端事件，不走 Go）：

```ts
// SettingsDialog.vue — 保存后发事件
import { EventsEmit } from '../../wailsjs/runtime/runtime'

async function saveItem() {
    await SaveItem(item)
    EventsEmit('items:updated')  // 通知其他组件
}

// Sidebar.vue — 监听并刷新
onMounted(() => {
    unsubItems = EventsOn('items:updated', loadItems)
})
onUnmounted(() => { if (unsubItems) unsubItems() })
```

---

## 六、文件操作

### 6.1 原生文件选择对话框

**不要用 `<input type="file">`**，Wails WebView 权限有限，且用户体验差。用 Wails 提供的原生对话框：

```go
import "github.com/wailsapp/wails/v2/pkg/runtime"

// 单文件
func (h *Handler) PickFile() (string, error) {
    return runtime.OpenFileDialog(h.ctx, runtime.OpenDialogOptions{
        Title: "选择文件",
        Filters: []runtime.FileFilter{
            {DisplayName: "图片", Pattern: "*.png;*.jpg;*.jpeg"},
            {DisplayName: "所有文件", Pattern: "*"},
        },
    })
}

// 多文件
func (h *Handler) PickFiles() ([]string, error) {
    return runtime.OpenMultipleFilesDialog(h.ctx, runtime.OpenDialogOptions{...})
}

// 保存对话框
func (h *Handler) PickSavePath() (string, error) {
    return runtime.SaveFileDialog(h.ctx, runtime.SaveDialogOptions{...})
}
```

### 6.2 附件设计（以图片/文件上传为例）

由**后端**完成文件读取和 Base64 编码，前端只传文件路径或直接用后端返回的数据：

```go
type Attachment struct {
    Name     string `json:"name"`
    MimeType string `json:"mime_type"`
    Data     string `json:"data"`      // base64（图片等二进制）
    Text     string `json:"text"`      // 纯文本内容（文档等）
}

func (h *Handler) PickAttachments() ([]Attachment, error) {
    paths, _ := runtime.OpenMultipleFilesDialog(h.ctx, ...)
    var result []Attachment
    for _, p := range paths {
        raw, _ := os.ReadFile(p)
        mime := detectMime(p)
        a := Attachment{Name: filepath.Base(p), MimeType: mime}
        if strings.HasPrefix(mime, "image/") {
            a.Data = base64.StdEncoding.EncodeToString(raw)
        } else {
            a.Text = string(raw)
        }
        result = append(result, a)
    }
    return result, nil
}
```

数据库存储：附件 meta（文件名、类型）存 JSON 字符串，不存原始数据（太大）：

```go
type Message struct {
    Attachments string `gorm:"type:text" json:"attachments"` // JSON []AttachmentMeta
}
```

---

## 七、数据库

### 7.1 路径约定

统一用 `os.UserHomeDir()` + 应用专属目录，**不要用相对路径**：

```go
home, _ := os.UserHomeDir()
dataDir := filepath.Join(home, ".my-app")
os.MkdirAll(dataDir, 0755)
dbPath := filepath.Join(dataDir, "app.db")
```

### 7.2 GORM + SQLite 初始化

```go
import (
    "gorm.io/driver/sqlite"
    "gorm.io/gorm"
)

var DB *gorm.DB

func InitDB(path string) error {
    var err error
    DB, err = gorm.Open(sqlite.Open(path), &gorm.Config{})
    if err != nil { return err }
    return DB.AutoMigrate(&Note{}, &Tag{}, ...)
}
```

### 7.3 AutoMigrate 注意事项

- GORM AutoMigrate 只能**新增字段**，不会删除或修改已有字段（安全）
- 新增字段时必须在 GORM tag 里指定 `default`，否则旧数据行的该字段为零值
- 修改字段类型需要手动迁移或用 `DB.Exec("ALTER TABLE ...")`

### 7.4 FTS5 全文检索

构建时必须带 `-tags fts5`，否则 FTS5 不可用：

```bash
wails build -tags fts5
# Makefile 里
TAGS := fts5
```

CJK（中日韩）检索必须用 `trigram` tokenizer，否则单字无法命中：

```sql
CREATE VIRTUAL TABLE notes_fts USING fts5(
    note_id UNINDEXED,
    content,
    tokenize='trigram'
)
```

**不要用** `content=` 外部表模式——当原始数据表事务未提交时，FTS5 内容可能为空，调试困难。推荐独立存储（FTS5 表自己存内容）。

### 7.5 排序稳定性

排序字段选 `created_at DESC` 而非 `updated_at DESC`。`updated_at` 会随任何更新操作变化（包括改标题、收藏等），导致列表顺序频繁跳动，用户体验差。

---

## 八、eino 集成

### 8.1 原则：优先用 eino-ext

**不要自己封装 OpenAI SDK**。eino-ext 已覆盖主流供应商，直接用：

```go
import (
    "github.com/cloudwego/eino-ext/components/model/openai"
    "github.com/cloudwego/eino-ext/components/model/claude"
    // 还有：deepseek、gemini、qwen、ark、ollama 等
)

// OpenAI 兼容接口（DeepSeek、通义、火山方舟等都用这个）
llm, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
    Model:   "deepseek-chat",
    APIKey:  apiKey,
    BaseURL: baseURL,  // 自定义 endpoint
})
```

### 8.2 流式输出

```go
stream, err := llm.Stream(ctx, messages)
if err != nil { return err }
defer stream.Close()

for {
    chunk, err := stream.Recv()
    if err != nil { break }  // io.EOF 即结束
    // chunk.Content — 文本内容
    // chunk.ReasoningContent — 思考链（deepseek-r1 等支持）
    // chunk.ToolCalls — 工具调用（流式拼接时需要合并）
}

// 流式 tool_calls 需要合并才能得到完整参数
merged, _ := schema.ConcatMessages(chunks)
toolCalls := merged.ToolCalls
```

### 8.3 Tool / Function Calling

实现 `tool.InvokableTool` 接口：

```go
type MyTool struct{}

func (t *MyTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
    return &schema.ToolInfo{
        Name: "search_notes",
        Desc: "搜索笔记内容",
        ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
            "query": {Type: schema.String, Desc: "搜索关键词", Required: true},
        }),
    }, nil
}

func (t *MyTool) InvokableRun(ctx context.Context, argsJSON string) (string, error) {
    var args struct{ Query string `json:"query"` }
    json.Unmarshal([]byte(argsJSON), &args)
    results := searchNotes(args.Query)
    return formatResults(results), nil
}
```

绑定到 LLM：

```go
tools := []tool.BaseTool{&MyTool{}}
infos := make([]*schema.ToolInfo, 0)
for _, t := range tools {
    info, _ := t.Info(ctx)
    infos = append(infos, info)
}
llm.BindTools(infos)
```

### 8.4 Tool Calling 循环（runToolLoop）

LLM 可能连续调用多次工具，需要循环处理直到无 tool_call 为止：

```go
const maxToolLoops = 10

func runToolLoop(ctx context.Context, llm, tools, messages) string {
    for i := 0; i < maxToolLoops; i++ {
        stream, _ := llm.Stream(ctx, messages)
        chunks := collectChunks(stream)
        merged, _ := schema.ConcatMessages(chunks)

        if len(merged.ToolCalls) == 0 {
            break  // LLM 直接返回文本，结束循环
        }

        // 把 assistant tool_call 消息加入历史
        messages = append(messages, &schema.Message{
            Role: schema.Assistant, ToolCalls: merged.ToolCalls,
        })

        // 执行每个工具，把结果作为 tool 消息加入历史
        for _, tc := range merged.ToolCalls {
            result := runTool(ctx, tools, tc.Function.Name, tc.Function.Arguments)
            messages = append(messages, &schema.Message{
                Role: schema.Tool, Content: result, ToolCallID: tc.ID,
            })
        }
    }
}
```

### 8.5 Skills 系统设计

Skills 本质是**预设的 system prompt 片段**，注入对话上下文：

```go
// DB 中存储 Skill
type Skill struct {
    ID      string `gorm:"primaryKey"`
    Name    string
    Content string  // SKILL.md 格式或纯 system prompt
    Enabled bool
}

// 发消息时，把选中的 skills 拼接到 system prompt
func buildSystemPrompt(basePrompt string, skillIDs []string) string {
    skills, _ := storage.GetSkillsByIDs(skillIDs)
    var parts []string
    if basePrompt != "" { parts = append(parts, basePrompt) }
    for _, s := range skills {
        parts = append(parts, s.Content)
    }
    return strings.Join(parts, "\n\n")
}
```

---

## 九、MCP 工具协议

### 9.1 两种接入方式

```go
import (
    mcpclient "github.com/mark3labs/mcp-go/client"
    mcpTool "github.com/cloudwego/eino-ext/components/tool/mcp"
)

// SSE 方式（HTTP 服务）
cli, _ := mcpclient.NewSSEMCPClient("http://localhost:3000/sse")
cli.Start(ctx)
cli.Initialize(ctx, mcp.InitializeRequest{...})

// Stdio 方式（本地进程，如 npx 服务）
cli, _ := mcpclient.NewStdioMCPClient("npx",
    []string{"-y", "@modelcontextprotocol/server-filesystem", "/"},
)

// 统一获取 tools
tools, _ := mcpTool.GetTools(ctx, &mcpTool.Config{Cli: cli})
```

### 9.2 注意事项

- MCP 客户端连接是长连接，放在后台 goroutine 维护，不要阻塞 UI 线程
- 每次 `StreamChat` 调用时按需连接/重连，不要在 app 启动时统一连接（用户可能未配置）
- Stdio 方式需要目标命令（如 `npx`）在系统 PATH 里

---

## 十、离线 Embedding（本地向量检索）

### 10.1 选型

`knights-analytics/hugot` + `all-MiniLM-L6-v2` ONNX 模型，纯 Go，无 CGO，无外部依赖：

```go
import (
    "github.com/knights-analytics/hugot"
    "github.com/knights-analytics/hugot/pipelines"
)

sess, _ := hugot.NewGoSession(ctx)
pipeline, _ := hugot.NewPipeline(sess, hugot.FeatureExtractionConfig{
    Name:      "all-MiniLM-L6-v2",  // Name 字段必填，否则 panic
    ModelPath: "/path/to/model/dir",
})
```

### 10.2 模型文件

模型文件（约 86MB）不进 git，构建时单独处理：

```
build/models/all-MiniLM-L6-v2/
├── model.onnx
├── tokenizer.json
├── tokenizer_config.json
├── vocab.txt
└── config.json
```

**路径探测优先级**（代码里按顺序找）：

1. macOS app bundle：`Contents/Resources/models/<name>`
2. 工作目录向上找 `build/models/<name>`（开发时 `wails dev` 适用）
3. `~/.cache/chroma/onnx_models/<name>/onnx`（Chroma 缓存，降级兜底）

### 10.3 打包到 app bundle

macOS：
```bash
cp -r build/models/all-MiniLM-L6-v2 \
  build/bin/MyApp.app/Contents/Resources/models/
```

Windows：
```bash
xcopy /E build\models build\bin\models\
```

CI 里从 HuggingFace 下载（见第十四节 CI 部分）。

### 10.4 向量存储

直接存 SQLite（`BLOB` 字段），向量为 `[]float32` 序列化为小端字节序：

```go
// 序列化
func float32SliceToBytes(v []float32) []byte {
    buf := make([]byte, len(v)*4)
    for i, f := range v {
        binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
    }
    return buf
}

// 余弦相似度
func CosineSim(a, b []float32) float32 {
    var dot, na, nb float64
    for i := range a {
        dot += float64(a[i]) * float64(b[i])
        na += float64(a[i]) * float64(a[i])
        nb += float64(b[i]) * float64(b[i])
    }
    if na == 0 || nb == 0 { return 0 }
    return float32(dot / (math.Sqrt(na) * math.Sqrt(nb)))
}
```

---

## 十一、WebDAV 备份

适用于需要云同步/备份场景（不依赖特定云服务）：

```go
import "github.com/studio-b12/gowebdav"

c := gowebdav.NewClient(url, username, password)
c.MkdirAll("/backup/", 0755)

// 上传
f, _ := os.Open(dbPath)
c.WriteStream("/backup/app-20260101.db", f, 0644)

// 下载
reader, _ := c.ReadStream("/backup/app-20260101.db")
defer reader.Close()
io.Copy(destFile, reader)
```

---

## 十二、macOS 适配

### 12.1 标题栏隐藏

让内容延伸到标题栏区域（现代 macOS 风格）：

```go
// main.go
Mac: &mac.Options{
    TitleBar: mac.TitleBarHiddenInset(),
}
```

需要在前端给标题栏区域留出空间（流量灯按钮约占 60-80px 高度）：

```css
.sidebar-header {
    padding-top: calc(var(--space-4) + 20px);  /* 为 traffic lights 留空间 */
}
```

### 12.2 confirm/alert 不可用

Wails 的 macOS WebView **不支持** `window.confirm()` 和 `window.alert()`，调用会静默失败或无响应。

**替代方案**：全部改为 Vue 内联 UI：

```vue
<!-- 二次确认示例 -->
<template v-if="!confirmDelete">
    <button @click="confirmDelete = true">删除</button>
</template>
<template v-else>
    <span>确认删除？</span>
    <button @click="doDelete">确认</button>
    <button @click="confirmDelete = false">取消</button>
</template>
```

---

## 十三、前端工程

### 13.1 CSS 设计 Token

用 CSS 自定义属性统一管理间距、颜色、字体，放在 `assets/tokens.css`：

```css
:root {
    /* 4pt 间距系统 */
    --space-1: 4px;
    --space-2: 8px;
    --space-3: 12px;
    --space-4: 16px;

    /* 颜色用 OKLCH，便于调暗/调亮 */
    --color-accent: oklch(0.65 0.2 260);
    --color-paper: oklch(0.99 0 0);
    --color-text: oklch(0.15 0 0);
}

[data-theme="dark"] {
    --color-paper: oklch(0.14 0 0);
    --color-text: oklch(0.92 0 0);
}
```

### 13.2 主题切换

```ts
// stores/theme.ts
type ThemeMode = 'light' | 'dark' | 'system'

const mode = ref<ThemeMode>(localStorage.getItem('theme') as ThemeMode ?? 'system')

function apply() {
    const isDark = mode.value === 'dark' ||
        (mode.value === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches)
    document.documentElement.setAttribute('data-theme', isDark ? 'dark' : 'light')
}
```

### 13.3 右键菜单

Wails WebView 里实现右键菜单需要注意：

- 监听 `@contextmenu.prevent` 阻止默认行为
- 菜单用 `<Teleport to="body">` 挂载，避免被父元素 `overflow: hidden` 裁剪
- 用 `position: fixed` + `clientX/clientY` 定位
- 通过 `document.addEventListener('mousedown', closeMenu, { once: true })` 点外关闭
- 菜单内部用 `@mousedown.stop` 阻止冒泡，防止触发关闭

### 13.4 Pinia Store 设计原则

- 跨多组件共享的数据放 store（如 conversations、providerMap）
- 只在单个组件内用的状态用 `ref` 本地管理
- store 里放的是数据，不放 UI 状态（如某弹窗是否打开）
- `providerMap`（id → name）这类映射表适合放 store，避免每个组件各自请求

---

## 十四、构建与发布

### 14.1 Makefile

```makefile
TAGS    := fts5
LDFLAGS := -X main.Version=$(VERSION)

build:
	wails build -tags "$(TAGS)" -ldflags "$(LDFLAGS)"

build-windows:
	wails build -tags "$(TAGS)" -ldflags "$(LDFLAGS)" -platform windows/amd64

dev:
	wails dev -tags "$(TAGS)"
```

**必须带 `-tags fts5`**，否则 `go-sqlite3` 的 FTS5 扩展不可用，知识库全文检索会失败。

### 14.2 版本号注入

```go
// main.go
var Version = "dev"  // 由构建时 ldflags 注入

// 暴露给前端
func (a *App) GetVersion() string { return Version }
```

```bash
wails build -ldflags "-X main.Version=1.2.0"
```

### 14.3 应用图标规范

- 源文件：`build/appicon.png`（1024×1024 PNG，RGBA）
- **Dock 图标规范**：内容占画布 80%，四周各留 10% 透明边距（macOS HIG 标准）
- Wails 自动将 `appicon.png` 转为各平台格式

### 14.4 大文件处理（ONNX 模型等）

- 不进 git（`.gitignore` 排除 `*.onnx`）
- CI 中从 HuggingFace 或 CDN 下载
- macOS：复制到 `app.app/Contents/Resources/`
- Windows：复制到 exe 同级 `models/` 目录

### 14.5 GitHub Actions CI（tag 触发自动发布）

```yaml
on:
  push:
    tags: ['v*']

jobs:
  build-mac:
    runs-on: macos-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.22' }
      - run: go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0
      - run: npm install
        working-directory: frontend
      - name: Download model
        run: |
          mkdir -p build/models/all-MiniLM-L6-v2
          curl -L -o build/models/all-MiniLM-L6-v2/model.onnx \
            "https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/onnx/model.onnx"
          # tokenizer.json, vocab.txt, config.json 同理
      - run: wails build -tags fts5 -platform darwin/universal -ldflags "-X main.Version=${{ github.ref_name }}"
      - name: Bundle model
        run: cp -r build/models/all-MiniLM-L6-v2 "build/bin/MyApp.app/Contents/Resources/models/"
      - name: Package
        run: ditto -c -k --keepParent build/bin/MyApp.app MyApp-mac.zip
```

Windows 用 `runs-on: windows-latest`，打包用 `Compress-Archive`。

---

## 十五、陷阱与注意事项

| 问题 | 原因 | 解法 |
|------|------|------|
| Handler 方法带 `context.Context` 参数 | Wails 不支持 | 通过 `SetContext` 在 startup 注入 |
| `window.confirm()` / `window.alert()` 失效 | macOS WebView 不实现 | 改为 Vue 内联 UI 二次确认 |
| FTS5 全文检索不可用 | 未带 `-tags fts5` | 构建命令必须加 `-tags fts5` |
| SQLite 并发写崩溃 | 默认不开 WAL | `DB.Exec("PRAGMA journal_mode=WAL")` |
| 列表排序莫名跳动 | 用 `updated_at` 排序 + 各种更新改了该字段 | 改用 `created_at DESC` |
| 修改 Go handler 后前端类型出错 | Bindings 未重新生成 | 执行 `wails generate module` |
| ONNX 模型加载失败 | `hugot.NewPipeline` 的 `Name` 字段为空 | `FeatureExtractionConfig.Name` 必须填 |
| 旧数据 provider 字段存 type 字符串 | 早期设计用 type 而非 UUID | 加载时做迁移：按 type 找到第一个匹配的 provider ID |
| 收藏/重命名改变列表顺序 | `ToggleFavorite`/`Rename` 更新了 `updated_at` | 用 `UpdateColumn` 只改目标字段，不触碰 `updated_at` |
| WebView 右键菜单被裁剪 | 父元素有 overflow 限制 | 菜单用 `<Teleport to="body">` + `position: fixed` |
| 跨组件数据不同步 | 各组件独立加载数据，无通知机制 | 修改后 `EventsEmit` 通知，订阅方重新拉取 |

---

## 十六、推荐的第三方库清单

| 库 | 版本 | 用途 |
|----|------|------|
| `wailsapp/wails/v2` | v2.12.0 | 桌面框架 |
| `cloudwego/eino` | v0.9.2 | LLM 统一接口 |
| `cloudwego/eino-ext/components/model/openai` | v0.1.13 | OpenAI/兼容接口 |
| `cloudwego/eino-ext/components/model/claude` | v0.1.18 | Claude 接口 |
| `cloudwego/eino-ext/components/tool/mcp` | v0.0.8 | MCP 工具接入 |
| `mark3labs/mcp-go` | v0.54.1 | MCP 客户端 |
| `knights-analytics/hugot` | v0.7.4 | 离线 ONNX Embedding |
| `mattn/go-sqlite3` | v1.14.22 | SQLite（需 fts5 tag） |
| `gorm.io/gorm` | v1.31.1 | ORM |
| `gorm.io/driver/sqlite` | v1.6.0 | GORM SQLite 驱动 |
| `google/uuid` | v1.6.0 | UUID 生成 |
| `studio-b12/gowebdav` | v0.12.0 | WebDAV 备份 |
| `dslipak/pdf` | v0.0.2 | PDF 文本提取 |
| `nguyenthenguyen/docx` | latest | Word 文本提取 |
| `xuri/excelize/v2` | v2.9.1 | Excel 读取 |
| `gopkg.in/yaml.v3` | v3.0.1 | YAML 解析 |
| Vue 3 + Pinia + Vite | latest | 前端框架 |
| marked + highlight.js | latest | Markdown 渲染 + 代码高亮 |
