# Light

> 一款轻量、优雅的本地 AI 对话客户端

![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Windows-blue)
![License](https://img.shields.io/badge/license-MIT-green)

---

## 简介

Light 是一款基于 [Wails](https://wails.io) + [eino](https://github.com/cloudwego/eino) 构建的桌面 AI 对话客户端，支持多模型供应商、联网搜索、Skills 技能系统、MCP 工具协议等特性，界面简洁，开箱即用。

## 功能特性

- **多模型支持**：OpenAI、Claude、DeepSeek、Gemini、通义千问、火山方舟、Ollama 本地模型等
- **流式输出**：打字机效果实时展示 AI 回复，支持思考链（Thinking）展示
- **联网搜索**：集成 Tavily Search API，AI 可自动调用搜索获取最新信息
- **Skills 广场**：上传 ZIP 包导入技能（SKILL.md 格式），问答时多选调用
- **智能体**：内置多种角色（通用助手、代码专家、写作助手等），支持自定义
- **MCP 协议**：支持 stdio / SSE 两种方式接入 MCP 工具服务
- **上下文控制**：一键清除上下文分割线，同一对话内切换话题
- **明暗主题**：浅色 / 深色 / 跟随系统，持久化记忆
- **自动标题**：首条消息自动生成对话标题，大模型 + 规则双重兜底

## 截图

> 浅色模式

![Light Mode](docs/screenshot-light.png)

> 深色模式

![Dark Mode](docs/screenshot-dark.png)

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
# macOS Universal
wails build -platform darwin/universal

# Windows
CC=x86_64-w64-mingw32-gcc wails build -platform windows/amd64
```

## 技术栈

| 层 | 技术 |
|----|------|
| 桌面框架 | [Wails](https://wails.io) v2 |
| AI 框架 | [eino](https://github.com/cloudwego/eino) |
| 前端 | Vue 3 + TypeScript + Vite |
| 数据库 | SQLite（GORM） |
| 搜索 | Tavily Search API |

## 数据存储

所有数据（对话记录、API Key、设置）存储在本地：

```
~/.wails-chat/chat.db
```

API Key 不会上传到任何服务器。

## License

MIT
