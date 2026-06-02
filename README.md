# Light

> 一款轻量、优雅的本地 AI 对话客户端

![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Windows-blue)
![License](https://img.shields.io/badge/license-MIT-green)

---

## 简介

Light 是一款基于 [Wails](https://wails.io) + [eino](https://github.com/cloudwego/eino) 构建的桌面 AI 对话客户端，支持多模型供应商、联网搜索、Skills 技能系统、MCP 工具协议、本地知识库等特性，界面简洁，开箱即用。

## 功能特性

- **多模型支持**：OpenAI、Claude、DeepSeek、Gemini、通义千问、火山方舟、Ollama 本地模型等
- **流式输出**：打字机效果实时展示 AI 回复，支持思考链（Thinking）展示
- **联网搜索**：集成 Tavily Search API，AI 可自动调用搜索获取最新信息
- **本地知识库**：上传文档（TXT/MD/PDF/DOCX/Excel），基于 FTS5 全文检索，AI 挂载知识库问答
- **Skills 广场**：上传 ZIP 包导入技能（SKILL.md 格式），问答时多选调用
- **智能体**：内置多种角色（通用助手、代码专家、写作助手等），支持自定义
- **MCP 协议**：支持 stdio / SSE 两种方式接入 MCP 工具服务
- **上下文控制**：一键清除上下文分割线，同一对话内切换话题
- **明暗主题**：浅色 / 深色 / 跟随系统，持久化记忆
- **自动标题**：首条消息自动生成对话标题，大模型 + 规则双重兜底

## 下载

前往 [Releases](https://github.com/YXR-ORG/Light/releases) 页面下载最新版本：

| 平台 | 文件 | 说明 |
|------|------|------|
| macOS | `Light-mac-universal.zip` | 支持 Apple Silicon & Intel |
| Windows | `Light-windows-amd64.zip` | 64位，解压后直接运行 |

> Windows 版需要系统安装 [WebView2 Runtime](https://developer.microsoft.com/en-us/microsoft-edge/webview2/)（Windows 11 已内置）

## 快速开始

### 配置模型

1. 打开 **设置 → 模型供应商**
2. 添加供应商并填入 API Key
3. 在模型列表中启用需要的模型

### 联网搜索

1. 前往 [Tavily](https://app.tavily.com) 申请免费 API Key（1000次/月）
2. 打开 **设置 → 通用设置** 填入 Key
3. 对话输入框点击 🌐 图标开启联网

### 导入 Skills

Skills 是可被 AI 主动调用的技能包，格式为包含 `SKILL.md` 的 ZIP 文件：

```markdown
---
name: my-skill
description: 技能描述
---

# 技能指令内容
你是一个...
```

打开 **设置 → Skills 广场** 上传 ZIP 即可导入。

## 发布新版本

```bash
git tag v1.x.x
git push origin v1.x.x
```

GitHub Actions 会自动构建 macOS + Windows 两个平台并发布到 [Releases](https://github.com/YXR-ORG/Light/releases)。

- `v1.0.0` → 正式版
- `v1.1.0-beta.1` → 预发布版（自动标记为 pre-release）

## 本地开发

### 环境要求

- Go 1.21+
- Node.js 18+
- [Wails CLI](https://wails.io/docs/gettingstarted/installation) v2

### 启动开发模式

```bash
wails dev
```

### 构建

```bash
# 使用 Makefile（推荐，自动带 fts5 tag）
make build        # 构建
make install      # 构建并安装到 /Applications（macOS）

# 手动构建（必须带 -tags fts5，否则知识库 FTS5 不可用）
# macOS Universal
wails build -tags fts5 -platform darwin/universal

# Windows
CC=x86_64-w64-mingw32-gcc wails build -tags fts5 -platform windows/amd64
```

### 图标规范

图标源文件：`build/appicon.png`（原始备份：`build/appicon.png.bak`）

**Dock 图标尺寸标准：内容占画布 80%，四周各留 10% 透明边距。**

生成脚本（每次更换图标后执行）：

```python
from PIL import Image
import numpy as np

src = "build/appicon.png.bak"  # 原始全出血图
dst = "build/appicon.png"

img = Image.open(src).convert("RGBA")
arr = np.array(img)
alpha = arr[:,:,3]
rows = np.any(alpha > 10, axis=1)
cols = np.any(alpha > 10, axis=0)
rmin, rmax = np.where(rows)[0][[0,-1]]
cmin, cmax = np.where(cols)[0][[0,-1]]
content = img.crop((cmin, rmin, cmax+1, rmax+1))

canvas_size = 1024
padding = 102  # 80% 内容占比
target = canvas_size - padding * 2
canvas = Image.new("RGBA", (canvas_size, canvas_size), (0,0,0,0))
canvas.paste(content.resize((target, target), Image.LANCZOS), (padding, padding))
canvas.save(dst)
```

生成 icns 并部署：

```bash
mkdir -p /tmp/Light.iconset
python3 << 'EOF'
from PIL import Image
img = Image.open("build/appicon.png").convert("RGBA")
for s in [16,32,64,128,256,512,1024]:
    img.resize((s,s), Image.LANCZOS).save(f"/tmp/Light.iconset/icon_{s}x{s}.png")
    if s <= 512:
        img.resize((s*2,s*2), Image.LANCZOS).save(f"/tmp/Light.iconset/icon_{s}x{s}@2x.png")
EOF
iconutil -c icns /tmp/Light.iconset -o /tmp/Light.icns
cp /tmp/Light.icns "/Applications/Light.app/Contents/Resources/iconfile.icns"
touch "/Applications/Light.app"
find ~/Library/Caches -name "com.apple.iconservices*" -delete
killall iconservicesd; killall Dock
```

## 技术栈

| 层 | 技术 |
|----|------|
| 桌面框架 | [Wails](https://wails.io) v2 |
| AI 框架 | [eino](https://github.com/cloudwego/eino) |
| 前端 | Vue 3 + TypeScript + Vite |
| 数据库 | SQLite（GORM + FTS5 trigram） |
| 搜索 | Tavily Search API |

## 数据存储

所有数据（对话记录、API Key、设置）存储在本地：

```
~/.wails-chat/chat.db                          # 主数据库
~/.wails-chat/knowledgebases/{id}/kb.db        # 每个知识库独立数据库
```

API Key 不会上传到任何服务器。

## 知识库使用

1. 打开 **设置 → 知识库**，点击「新建知识库」
2. 进入知识库，点击「上传文件」（支持 TXT / MD / PDF / DOCX / Excel）
3. 等待文档状态变为「就绪」
4. 回到对话界面，点击输入框左下角「问答」切换为「知识」模式
5. 选择知识库，发送问题

**跨文档问题**（如"A 和 B 有什么共同点"）：AI 会自动分别搜索每个实体，再综合推理回答。

## 知识库 TODO

当前知识库基于 FTS5 全文检索（trigram tokenizer），能可靠处理中文任意子串匹配。以下三个方向是下一步的演进路径，按优先级排序：

### TODO 1：文档摘要索引（短期，低成本）

**问题**：FTS5 只能匹配关键词，无法理解"这篇文档讲的是什么"。文档量大时，模型需要多次搜索才能定位到正确文档。

**方案**：文档上传时，用 LLM 生成每个文档的摘要（人物、主题、关键事件）和关键实体列表，存入 `summaries` 表。搜索时先搜摘要层定位文档，再搜 chunk 层取内容（两阶段检索）。

```sql
CREATE TABLE summaries (
  doc_id TEXT PRIMARY KEY,
  summary TEXT,        -- LLM 生成的文档摘要
  key_entities TEXT,   -- JSON 数组，如 ["张嘎", "奶奶", "鬼子"]
  created_at DATETIME
);
```

**预期效果**：减少无效搜索轮次，跨文档问题定位更准确。

---

### TODO 2：向量检索 + RRF 混合（中期，根本解决语义问题）

**问题**：关键词检索无法理解语义。"勇敢的少年"和"机智的小鬼"语义相同，但 FTS5 找不到关联。这是当前架构的根本局限。

**方案**：内嵌 `all-MiniLM-L6-v2` ONNX 模型（384 维，约 22MB），通过 `onnxruntime-go` 在本地运行，完全离线，无需 Ollama 或任何外部服务。

1. 模型文件打包进 app bundle（`Contents/Resources/models/all-MiniLM-L6-v2.onnx`）
2. 文档就绪后，后台异步向量化所有 chunks，写入已预留的 `vectors` 表
3. 搜索时并行执行 FTS5（关键词）+ 向量相似度（语义）两路检索
4. 用 **RRF（Reciprocal Rank Fusion）** 合并两路结果，取长补短

```
用户问题 → embedding → 向量相似度检索 ─┐
                                        ├→ RRF 融合 → top-k chunks → LLM
用户问题 → FTS5 关键词检索 ────────────┘
```

**已就绪**：`vectors` 表已预留（`embedding BLOB`），schema 不需要变更。待实现：`onnxruntime-go` 集成 + embedding pipeline + 向量相似度检索。

---

### TODO 3：知识图谱 / 实体关系索引（长期，复杂场景）

**问题**：文档间的实体关系（人物关系、事件因果、概念层级）无法被检索层感知。适合企业知识库、法律文档、医疗记录等有明确实体关系的场景。

**方案**：
1. 文档上传时，用 LLM 抽取实体和关系（`孙小仙 is_character_in 公平国往事`）
2. 存入图结构（可用 SQLite 的邻接表模拟，无需引入图数据库）
3. 查询时识别问题中的实体，图遍历找关联实体，结合向量检索召回相关 chunk

**适用场景**：文档间有明确关联关系时（如人物关系谱、产品文档体系）。对创意推断类问题（"张嘎进入公平国会发生什么"）帮助有限，因为两个文档本来就没有关系，是用户在做跨文档创意推断。

---

> 详细问题复盘见 [docs/KNOWLEDGE_BASE_POSTMORTEM.md](docs/KNOWLEDGE_BASE_POSTMORTEM.md)
>
> 完整实现技术文档见 [docs/KNOWLEDGE_BASE_IMPL.md](docs/KNOWLEDGE_BASE_IMPL.md)

## License

MIT
