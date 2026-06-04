# Light

> 一款轻量、优雅的本地 AI 对话客户端

![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Windows-blue)
![License](https://img.shields.io/badge/license-MIT-green)

---

## 简介

Light 是一款基于 [Wails](https://wails.io) + [eino](https://github.com/cloudwego/eino) 构建的桌面 AI 对话客户端，支持多模型供应商、联网搜索、Skills 技能系统、MCP 工具协议、本地知识库等特性，界面简洁，开箱即用。

## 功能特性

- **多模型支持**：OpenAI、Claude、DeepSeek、Gemini、通义千问、火山方舟、Ollama 本地模型等
- **三种对话模式**：
  - **问答**：普通对话，可挂载联网搜索、Skills、MCP 工具
  - **知识库**：挂载本地知识库进行文档问答
  - **任务模式（Agent）**：基于 eino ReAct Agent 自主规划、执行多步骤任务（文件读写、执行命令、调用工具），全程推理链可见
- **流式输出**：打字机效果实时展示 AI 回复，支持思考链（Thinking）展示
- **联网搜索**：集成 Tavily Search API，AI 可自动调用搜索获取最新信息
- **本地知识库**：上传文档（TXT/MD/PDF/DOCX/Excel），基于 FTS5 全文检索，AI 挂载知识库问答
- **Skills 广场**：上传 ZIP 包导入技能（SKILL.md 格式），问答时多选调用
- **智能体**：内置多种角色（通用助手、代码专家、写作助手等），支持自定义
- **MCP 协议**：支持 stdio / SSE 两种方式接入 MCP 工具服务
- **任务产物区**：Agent 任务涉及的文件（生成 / 读取）自动归集为产物卡片，点击即可打开或在文件夹中定位，持久化存储、历史可查
- **执行计划（Plan 模式）**：可选开启，Agent 在执行前生成结构化计划并实时更新进度，每步骤状态可追踪
- **任务附件**：任务模式支持上传图片 / 文档作为上下文，附件内容直接注入 Agent 执行流
- **统一消息布局**：任务模式与问答模式采用一致的左侧头像 + 标签 + 内容布局，视觉体验统一
- **安全执行**：危险 Shell 命令（黑名单可配置）执行前弹窗确认；文件操作限制在工作目录内
- **上下文控制**：一键清除上下文分割线，同一对话内切换话题
- **明暗主题**：浅色 / 深色 / 跟随系统，持久化记忆
- **自动标题**：首条消息自动生成对话标题，大模型 + 规则双重兜底
- **数据备份**：支持 WebDAV（坚果云 / Nextcloud / Alist 等）一键备份与恢复

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

### 使用任务模式（Agent）

任务模式让 AI 像 Agent 一样自主规划并执行多步骤任务：

1. 在对话输入框左下角切换为「任务」模式
2. 选择工作目录（Agent 的文件操作限制在此目录内）
3. 描述任务目标，AI 会自主调用工具（搜索、文件读写、执行命令等）逐步完成
4. 推理链全程可见（思考 / 工具调用 / 结果），可随时中断
5. 涉及的文件会在回复下方以**产物卡片**展示，点击即可打开

**执行计划（Plan 模式）**：在 **设置 → 任务设置** 中开启后，Agent 会在执行前生成结构化计划（有序步骤列表），每步执行后实时更新状态，全程进度可追踪。

**任务附件**：任务模式支持附带图片或文档，内容直接注入 Agent 上下文，无需额外指令。

> 危险命令（如 `rm -rf`）执行前会弹窗确认，黑名单可在 **设置 → 通用设置** 中配置。
> 默认不主动写文件，仅当你明确要求「保存 / 导出 / 生成文件」时才落地。

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

---

> 详细问题复盘见 [docs/KNOWLEDGE_BASE_POSTMORTEM.md](docs/KNOWLEDGE_BASE_POSTMORTEM.md)
>
> 完整实现技术文档见 [docs/KNOWLEDGE_BASE_IMPL.md](docs/KNOWLEDGE_BASE_IMPL.md)
>
> 任务模式设计：[功能设计](docs/superpowers/specs/2026-06-03-task-mode-design.md) · [ReAct 架构](docs/superpowers/specs/2026-06-04-task-react-architecture.md) · [产物机制 & 自适应执行](docs/superpowers/specs/2026-06-04-task-artifact-mechanism.md)
>
> 功能规划与 TODO 见 [docs/TODO.md](docs/TODO.md)

## Changelog

### v1.5.0
- 执行计划（Plan 模式）：Agent 执行前生成结构化计划，逐步追踪完成进度
- 任务附件：任务模式支持图片 / 文档附件，内容直接注入 Agent 上下文
- 任务消息布局统一：与问答模式对齐，左侧头像 + 标签 + 内容，视觉一致
- 优化推理链展示：大段内容摘要（content_rollback > 1200 字符）不再重复展示于推理链
- 修复 plan 产物去重：同一 plan_id 始终保留最新状态，任务结束自动补全剩余步骤为"已完成"

### v1.4.0
- 通用产物机制（`<!--ARTIFACT:...-->`）：文件读写、URL、计划均统一以产物卡片展示
- 自适应 MaxStep：按任务复杂度动态扩展最大步骤数
- 产物持久化：任务消息产物写入 DB，历史对话可恢复

### v1.3.x
- 任务模式 Agent（eino ReAct）：文件读写、Shell 执行、联网搜索
- 推理链折叠展示：thinking / tool_call / tool_result 全链路可见
- 流式输出、危险命令确认弹窗、工作目录安全限制

## License

MIT
