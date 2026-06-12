

# Wenhao vs DeepSeek-Reasonix 对比分析报告

> 生成日期：2025-07-18

---

## 一句话结论

**wenhao ≈ Reasonix + 🎨 GUI 基础设施 + 🔌 更深 MCP 集成 + 🔧 更多开发工具 + 📝 ACP Editor 集成**

两个项目共享完全相同的核心 DNA：都是 Go 单二进制、TOML 配置驱动、MCP 插件扩展、control.Controller → agent.Runner → provider.Stream 的传输无关三层架构。wenhao 在 Reasonix 基础上长出了 11 个独有模块 + 多项独有功能。

---

## 一、技术栈对比

| 层面 | Reasonix | Wenhao |
|------|----------|--------|
| **语言** | Go 1.25+，CGO_ENABLED=0 | Go 1.25+（toolchain go1.26.4），CGO_ENABLED=0 |
| **配置格式** | TOML（BurntSushi/toml，唯一第三方依赖） | TOML（BurntSushi/toml，唯一第三方依赖） |
| **终端 UI** | Bubble Tea v2 + Lipgloss v2 | Bubble Tea v2 + Lipgloss v2 + Goldmark + Chroma |
| **桌面客户端** | Wails v2 | Wails v3 |
| **AI Provider** | OpenAI-compatible + Anthropic Messages API | OpenAI-compatible + Anthropic Messages API |
| **插件协议** | MCP JSON-RPC 2.0（stdio + HTTP） | MCP JSON-RPC 2.0（stdio + Streamable HTTP） |
| **Editor 集成** | ❌ | ACP v1 JSON-RPC 2.0 over stdio |
| **代码智能** | CodeGraph | CodeGraph |
| **沙箱** | macOS Seatbelt | macOS Seatbelt |
| **构建/发布** | Makefile + GoReleaser，6 目标交叉编译 | Makefile + GoReleaser，6 目标交叉编译 |
| **包分发** | npm + Homebrew | npm + Homebrew |

---

## 二、目录结构对比

### Reasonix 顶层

```
reasonix/
├── cmd/
│   ├── reasonix/main.go
│   ├── reasonix-plugin-example/
│   └── e2ebench/
├── internal/
│   ├── cli/          # CLI 入口（Bubble Tea TUI）
│   ├── agent/        # Agent 核心（Session + Run loop）
│   ├── control/      # 传输无关的会话驱动层
│   ├── boot/         # 装配工厂
│   ├── config/       # TOML 配置加载
│   ├── provider/     # LLM 后端抽象 + 注册表
│   ├── tool/         # 工具抽象 + 注册表
│   ├── plugin/       # MCP 客户端
│   ├── permission/   # 权限决策引擎
│   ├── sandbox/      # 沙箱安全
│   ├── skill/        # Skill 系统
│   ├── command/      # 自定义斜杠命令
│   ├── memory/       # 记忆系统
│   ├── event/        # 事件类型定义
│   ├── hook/         # 会话生命周期钩子
│   ├── checkpoint/   # 检查点
│   ├── codegraph/    # CodeGraph 集成
│   ├── lsp/          # LSP 客户端
│   ├── serve/        # HTTP/SSE 前端
│   ├── bot/          # 机器人网关（飞书/QQ）
│   ├── jobs/         # 后台任务调度
│   ├── billing/      # Token 用量计费
│   ├── i18n/         # 国际化
│   ├── diff/         # Diff 引擎
│   ├── instruction/  # 指令构建
│   ├── doctor/       # 诊断报告
│   └── netclient/    # HTTP 客户端封装
├── desktop/          # Wails 桌面应用
├── npm/reasonix/
├── workers/crash-report/
├── site/
├── docs/
├── benchmarks/
├── REASONIX.md
└── reasonix.example.toml
```

### Wenhao 顶层

```
wenhao/
├── cmd/
│   ├── wenhao/main.go
│   ├── wenhao-plugin-example/
│   └── e2ebench/
├── internal/
│   ├── cli/          # CLI 入口（Bubble Tea TUI）
│   ├── agent/        # Agent 核心（多出 orchestrator.go、review_loop.go）
│   ├── control/      # 传输无关的会话驱动层（多出 auto_plan.go、branches.go）
│   ├── boot/         # 装配工厂
│   ├── config/       # TOML 配置（多出 edit.go、mcpjson.go、migrate.go、render.go）
│   ├── provider/     # LLM 后端抽象
│   ├── tool/         # 工具抽象
│   ├── plugin/       # MCP 客户端（多出 prompts.go、resources.go、lazy.go、cache.go）
│   ├── permission/   # 权限决策引擎
│   ├── sandbox/      # 沙箱安全
│   ├── skill/        # Skill 系统（多出 builtins.go、tools.go、index.go）
│   ├── command/      # 自定义斜杠命令（多出 slashtool.go）
│   ├── memory/       # 记忆系统（多出 dream.go、session_memory.go、remember.go）
│   ├── event/        # 事件类型定义
│   ├── hook/         # 会话生命周期钩子（多出 trust.go）
│   ├── checkpoint/   # 检查点
│   ├── codegraph/    # CodeGraph 集成（多出 install.go）
│   ├── lsp/          # LSP 客户端
│   ├── serve/        # HTTP/SSE 前端
│   ├── billing/      # 余额查询
│   ├── i18n/         # 国际化
│   ├── diff/         # Diff 引擎
│   ├── evidence/     # 工具调用凭证（多出 readiness_audit.go）
│   ├── instruction/  # 指令构建
│   ├── doctor/       # 诊断报告
│   ├── netclient/    # HTTP 客户端封装
│   ├── job/          # 后台任务管理（替代 Reasonix 的 jobs/）
│   ├── acp/          # 🆕 ACP v1 协议适配器
│   ├── outputstyle/  # 🆕 输出风格/Persona 系统
│   ├── inspect/      # 🆕 运行时能力投影（GUI 面板数据源）
│   ├── notify/       # 🆕 桌面系统通知
│   ├── sysproxy/     # 🆕 系统代理检测
│   ├── frontmatter/  # 🆕 轻量 frontmatter 解析器
│   ├── fileutil/     # 🆕 文件工具（含 encoding/ 编码检测）
│   ├── fileref/      # 🆕 文件名模糊搜索
│   ├── nilutil/      # 🆕 Nil 检查辅助
│   ├── proc/         # 🆕 进程管理
│   └── mcpdiag/      # 🆕 MCP 连接诊断 + OAuth
├── desktop/          # Wails 桌面应用
├── npm/wenhao/
├── site/
├── docs/
├── benchmarks/
├── scripts/
├── .wenhao/
├── WENHAO.md
├── WENHAO.local.md
├── wenhao.toml
└── wenhao.example.toml
```

---

## 三、wenhao 独有模块详解

### 3.1 功能层新增

| 模块 | 文件 | 功能描述 | 价值 |
|------|------|----------|------|
| **`outputstyle/`** | — | Persona/风格切换系统，内置多风格 + 自定义 Markdown 文件加载 | 用户可以切换 AI 的「人设」和「语气」，与 skill/command 使用相同的 frontmatter 约定 |
| **`inspect/`** | — | 运行时能力投影层，将 config、tool registry、plugin host、command list 序列化为 JSON | 专为 GUI 设置面板设计，桌面端可渲染所有运行时配置 |
| **`notify/`** | `sender_darwin.go`、`sender_linux.go`、`sender_windows.go` | 桌面系统通知，按 OS 分平台实现 | 后台任务完成时弹通知，用户不用一直盯着终端 |
| **`sysproxy/`** | `system_windows.go`、`system_other.go` | 系统代理自动检测，按 OS 分平台 | 自动适配网络环境，企业内网用户无需手动配置代理 |
| **`mcpdiag/`** | — | MCP 连接诊断 + OAuth 认证辅助 | MCP 服务出问题时能快速定位，支持 OAuth 认证流程 |

### 3.2 工具/文件层新增

| 模块 | 文件 | 功能描述 | 价值 |
|------|------|----------|------|
| **`frontmatter/`** | — | 轻量 frontmatter 解析器（`---` 分隔的 key:value 块） | 零额外依赖，不引入 YAML 库 |
| **`fileutil/encoding/`** | — | 文件编码检测（UTF-8、UTF-16 LE/BE 等） | 编辑文件前自动识别编码，防止编码相关问题 |
| **`fileref/`** | — | 文件名模糊搜索（basename 匹配） | 自动补全时快速定位文件，无需打全路径 |
| **`nilutil/`** | — | Nil 检查辅助工具 | 基础工具库 |
| **`proc/`** | — | 进程管理（隐藏/kill 子进程），分平台实现 | 管理 MCP 子进程和 bash 子进程生命周期 |

### 3.3 协议与互操作层新增

| 模块 | 核心文件 | 功能描述 | 价值 |
|------|----------|----------|------|
| **`acp/`** | `protocol.go`、`server.go`、`dispatch.go`、`service.go` | Agent Client Protocol v1 适配器 | 支持 Editor 集成（如 VS Code 插件），将 Editor 的 JSON-RPC 2.0 请求映射到 control.Controller，每个 Editor session 拥有独立 Controller |

---

## 四、wenhao 独有功能特性（核心包内增量）

### 4.1 Agent 核心增强

| 文件 | 功能 | 说明 |
|------|------|------|
| **`agent/orchestrator.go`** | 纯编排模式 | 主 agent 只做任务拆解和子代理调度，自己不直接执行工具调用，作为纯粹的「任务编排者」 |
| **`agent/review_loop.go`** | 评审循环 | 代码生成后自动进入审查→修改循环，提升代码质量 |

### 4.2 Controller 增强

| 文件 | 功能 | 说明 |
|------|------|------|
| **`control/auto_plan.go`** | 自动规划分类器 | 使用廉价分类器模型判断任务复杂度，复杂任务自动进入只读规划模式 |
| **`control/branches.go`** | 对话分支管理 | 在卡点时创建/切换/复刻对话分支（`/tree`、`/branch`、`/switch`、`/rewind`） |

### 4.3 MCP 集成增强

| 文件 | 功能 | 说明 |
|------|------|------|
| **`plugin/prompts.go`** | MCP Prompt → 斜杠命令 | MCP 服务器的 prompt 自动映射为本地斜杠命令 |
| **`plugin/resources.go`** | MCP Resource → @ 引用 | MCP 资源可用 `@server:uri` 语法直接引用 |
| **`plugin/lazy.go`** | MCP 懒连接 | 按需连接 MCP 服务器，减少启动开销 |
| **`plugin/cache.go`** | 工具列表缓存 | 缓存 MCP 工具列表，避免重复请求 |

### 4.4 工具集增强

Reasonix 内置工具：`read_file`、`write_file`、`edit_file`、`bash`、`ls`、`glob`、`grep`、`webfetch`、`todo`

Wenhao 额外新增 8 个内置工具：

| 工具 | 文件 | 说明 |
|------|------|------|
| **`multi_edit`** | `multiedit.go` | 批量编辑多个文件的多处位置 |
| **`delete_range`** | `delete_range.go` | 删除文件中的指定行范围 |
| **`delete_symbol`** | `delete_symbol.go` | 按符号名称删除代码块 |
| **`notebook_edit`** | `notebookedit.go` | Jupyter Notebook 编辑支持 |
| **`complete_step`** | `completestep.go` | 证据驱动步骤完成确认 |
| **`preview`** | `preview.go` | 文件变更预览 |
| **`workspace`** | `workspace.go` | 工作区管理 |
| **`confine`** | `confine.go` | 工具执行范围限制 |
| **`gitignore`** | `gitignore.go` | .gitignore 规则处理 |

### 4.5 Skill 系统增强

| 文件 | 功能 | 说明 |
|------|------|------|
| **`skill/builtins.go`** | 内置技能注册 | 项目自带的内置技能定义 |
| **`skill/tools.go`** | `run_skill` 工具 | 将 Skill 作为工具暴露给模型，模型可自主调用 |
| **`skill/index.go`** | Cache-stable 技能索引 | 构建保持 prefix cache 稳定的技能索引 |

### 4.6 斜杠命令增强

| 文件 | 功能 | 说明 |
|------|------|------|
| **`command/slashtool.go`** | 斜杠命令 → 工具 | 自定义斜杠命令可以作为 Tool 让模型自己调用 |

### 4.7 记忆系统增强

| 文件 | 功能 | 说明 |
|------|------|------|
| **`memory/dream.go`** | 跨会话记忆合并 | 多个会话的记忆自动合并去重 |
| **`memory/session_memory.go`** | 会话内记忆 | 会话级别的临时记忆存储 |
| **`memory/remember.go`** | 持久化保存 | 将记忆持久化到文件系统 |

### 4.8 证据系统增强

| 文件 | 功能 | 说明 |
|------|------|------|
| **`evidence/readiness_audit.go`** | 最终答案就绪审计 | 检查 agent 的最终答案是否引用了实际工具执行证据 |

---

## 五、Reasonix 有而 Wenhao 没有的

| 模块 | 功能 | 说明 |
|------|------|------|
| **`bot/`** | 机器人网关 | 飞书/QQ 适配器，让 Reasonix 作为聊天机器人接入 IM 平台。Wenhao 移除了此模块 |

---

## 六、架构层面差异总结

| 维度 | Reasonix | Wenhao |
|------|----------|--------|
| **内核包数量** | ~28 个 internal 包 | ~34 个 internal 包 |
| **GUI 支持深度** | 桌面 App | 桌面 App + `inspect/` 投影 + `notify/` 通知 + `sysproxy/` 代理 |
| **MCP 集成完成度** | 客户端完整（stdio + HTTP） | 客户端更完整（+ prompt→命令、resource→@引用、懒连接、缓存、诊断、OAuth） |
| **Editor 集成** | 无 | 完整 ACP v1 |
| **IM 接入** | 有 `bot/`（飞书/QQ） | 无 |
| **内置工具数量** | 9 个 | 18 个（+9） |
| **Agent 模式** | 双模型协同 | 双模型协同 + 纯编排 + 评审循环 + 自动规划分类器 |
| **Skill 系统** | 加载 + 执行 | 加载 + 执行 + 内置技能 + 工具暴露 + 缓存索引 |
| **记忆系统** | 层次化记忆 + 自动记忆 | 层次化记忆 + 自动记忆 + 跨会话合并 + 会话内记忆 |

---

## 七、差异速览表

| 功能特性 | Reasonix | Wenhao |
|----------|:---:|:---:|
| 配置驱动 + 多模型 | ✅ | ✅ |
| MCP 插件（stdio + HTTP） | ✅ | ✅ |
| 交互式 TUI（Bubble Tea） | ✅ | ✅ |
| 双模型协同 | ✅ | ✅ |
| 上下文压缩 | ✅ | ✅ |
| 权限系统（allow/ask/deny） | ✅ | ✅ |
| 沙箱（macOS Seatbelt） | ✅ | ✅ |
| Skill 系统 | ✅ | ✅ |
| 自定义斜杠命令 | ✅ | ✅ |
| @ 引用 | ✅ | ✅ |
| 对话分支 | ✅ | ✅ |
| Checkpoint 与回退 | ✅ | ✅ |
| HTTP/SSE Server | ✅ | ✅ |
| 桌面客户端（Wails） | ✅ | ✅ |
| 国际化（中英文） | ✅ | ✅ |
| 零依赖单二进制 | ✅ | ✅ |
| LSP 集成 | ✅ | ✅ |
| CodeGraph 集成 | ✅ | ✅ |
| 会话持久化 | ✅ | ✅ |
| **MCP 深层集成（Prompt→命令、Resource→@引用、懒连接、缓存、诊断、OAuth）** | ❌ | ✅ |
| **ACP v1 Editor 集成** | ❌ | ✅ |
| **Persona/风格切换（outputstyle）** | ❌ | ✅ |
| **GUI 运行时投影（inspect）** | ❌ | ✅ |
| **桌面通知（notify）** | ❌ | ✅ |
| **系统代理检测（sysproxy）** | ❌ | ✅ |
| **文件编码检测（fileutil/encoding）** | ❌ | ✅ |
| **纯编排模式（orchestrator）** | ❌ | ✅ |
| **评审循环（review_loop）** | ❌ | ✅ |
| **自动规划分类器（auto_plan）** | ❌ | ✅ |
| **额外 8 个内置工具** | ❌ | ✅ |
| **斜杠命令暴露为工具（slashtool）** | ❌ | ✅ |
| **跨会话记忆合并（dream）** | ❌ | ✅ |
| **证据就绪审计（readiness_audit）** | ❌ | ✅ |
| **IM 机器人（bot/：飞书/QQ）** | ✅ | ❌ |

---

## 八、架构图示（共享的核心理念）

```
┌───────────────────────────────────────────────────────────┐
│  三个前端共享一个 Controller                                │
│                                                           │
│  CLI TUI          HTTP/SSE          Wails Desktop          │
│  (Bubble Tea)     (serve/)          (desktop/)              │
│       │               │                  │                 │
│       └───────────────┼──────────────────┘                 │
│                       │                                    │
│              control.Controller                            │
│              （传输无关的会话驱动层）                          │
│                       │                                    │
│              agent.Runner.Run()                             │
│              （Agent 运行循环）                               │
│                       │                                    │
│         ┌─────────────┼─────────────┐                      │
│         │             │             │                      │
│    provider.Stream   tool.Execute   compact()               │
│    （LLM 后端）       （工具调用）     （压缩）                │
│                                                           │
│  工具注册表 = 内置工具 + MCP 插件                               │
│  权限门 = Policy + Sandbox                                  │
└───────────────────────────────────────────────────────────┘
```

---

## 九、最终总结

如果把 Reasonix 比作**精悍的跑车**，wenhao 就是**配置拉满的豪华版**——引擎一样，但增加了：

- 🎨 **GUI 基础设施**：inspect 投影 + 桌面通知 + 系统代理检测
- 🔌 **更深 MCP 集成**：prompt→命令、resource→@引用、懒连接、缓存、诊断、OAuth
- 🔧 **更多开发工具**：9 个额外内置工具，覆盖批量编辑、符号删除、notebook 等
- 📝 **ACP Editor 集成**：可被 VS Code 等 Editor 通过标准协议集成
- 🧠 **更多 Agent 模式**：纯编排、评审循环、自动规划分类器
- 💾 **更强的记忆**：跨会话合并、会话内记忆

唯一被 Reasonix 独占的是 `bot/`（IM 机器人网关），如果你不需要飞书/QQ 接入，wenhao 是全方位的超集。
