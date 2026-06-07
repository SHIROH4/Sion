# Sion

诗音是一只生活在桌面上的猫娘伙伴。她观察你的行为，从互动中学习，主动搭话，逐渐变得更懂你。

**不是 Chat 窗口，不是 IDE 插件——是一个有 Live2D 形象、有情绪、有记忆的桌面伙伴。**

![License](https://img.shields.io/badge/license-MIT-blue)
![Go](https://img.shields.io/badge/Go-1.25+-00ADD8)
![React](https://img.shields.io/badge/React-18-61DAFB)

---

## 核心能力

| 系统 | 功能 | 说明 |
|------|------|------|
| **Live2D 桌宠** | 视线追踪、拖拽、戳一戳、气泡文字、表情切换 | 基于 PIXI.js + pixi-live2d-display |
| **AI 对话** | 代码问答、Debug、技术选型、闲聊 | LLM API 驱动，流式响应 |
| **主动搭话** | 长时间工作提醒、深夜陪伴、饭点关心 | 规则引擎 + System 2 LLM 决策 |
| **记忆系统** | 三级记忆 (会话/日记/事实) + 向量检索 + Ebbinghaus 遗忘 | SQLite + Ollama embedding |
| **情绪系统** | PAD 三维 + 8 维向量 + 人格参数学习 | LLM 评估 + 规则回退 |
| **自学习** | 策略原则提取、行为模式挖掘、驱力评分自适应 | 日反思 + RL 权重更新 |
| **截图即问** | macOS Vision OCR + 多模态 LLM 分析 | 原生 Vision Framework |
| **屏幕感知** | 定期 OCR 屏幕、识别工作/休闲状态 | 自适应间隔 |
| **联网搜索** | 博查 API 接入，对话中搜索最新技术信息 | 意图检测 + 强制工具调用 |
| **工具调用** | 三层架构：代码路由 → 工具执行 → LLM 话术 | tool_choice 强制 + 2 轮回复 |

---

## 认知架构

```
                    ┌──────────────────────────────┐
                    │       System 2  LLM 决策       │
                    │  情绪 + 驱力 + 特征 → 行动选择  │
                    └──────────────┬───────────────┘
                                   │
        ┌──────────────┬───────────┼───────────┬──────────────┐
        │              │           │           │              │
   ┌────┴────┐  ┌──────┴──────┐ ┌──┴───┐ ┌────┴────┐ ┌──────┴──────┐
   │ 情绪模型 │  │ 内源需求×6  │ │ 关怀  │ │ 好奇引擎 │ │ 策略实验室   │
   │ PAD+8维 │  │ 被动消退    │ │ 引擎  │ │ 缺口扫描 │ │ 向量去重合并 │
   └─────────┘  └─────────────┘ └──────┘ └─────────┘ └─────────────┘
                                   │
                    ┌──────────────┴───────────────┐
                    │      动态决策间隔              │
                    │  活跃1min → 常规5min → 休眠30min │
                    │  事件驱动插队 (消息/切换App)     │
                    └──────────────────────────────┘
```

**决策流**: 量化特征(46维) → 驱力计算(5维) → 行动评分 → fast path / LLM fallback

**记忆流**: 对话 → SessionBuffer(L0) → 日记+情绪(L1) → 事实+向量(L2) → 策略原则(L3)

**工具调用**: 意图检测 (Go 代码) → 工具过滤 (Layer 1 记忆常驻 + Layer 2 搜索按需) → tool_choice 强制调用 → LLM 自然回复

---

## 快速开始

### 前置依赖

- **Go 1.25+**
- **Node.js 18+**
- **Wails CLI**: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- **Ollama** (本地 embedding): 下载并启动后拉模型
- **LLM API Key** (OpenAI / SiliconFlow / DeepSeek 等)

### 安装

```bash
# 1. 进入项目
cd desktop-pet

# 2. 安装前端依赖
cd frontend && npm install && cd ..

# 3. 拉取本地 embedding 模型
ollama pull CompendiumLabs/bge-small-zh-v1.5-gguf

# 4. 创建配置文件 ~/.desktop-pet/config.yaml
cat > ~/.desktop-pet/config.yaml << 'EOF'
llm_model: deepseek-chat
llm_api_key: sk-your-key-here
llm_base_url: https://api.deepseek.com
user_name: 主人
user_tech_stack:
  - Go
  - TypeScript
embedding_model: hf.co/CompendiumLabs/bge-small-zh-v1.5-gguf
EOF

# 5. 启动
wails dev
```

### 配置参数

| 字段 | 说明 | 默认值 |
|------|------|--------|
| `llm_model` | LLM 模型名 | `deepseek-chat` |
| `llm_api_key` | API Key | (必填) |
| `llm_base_url` | API 地址 | `https://api.deepseek.com` |
| `vision_model` | 视觉模型 (截图分析) | 空 (复用 chat 模型) |
| `emotion_model` | 情绪评估专用模型 | 空 (复用 chat 模型) |
| `embedding_model` | 本地 embedding 模型 | `hf.co/CompendiumLabs/bge-small-zh-v1.5-gguf` |
| `user_name` | 称呼 | `主人` |
| `user_tech_stack` | 技术栈 | `[]` |
| `warm_start` | 初始人格 + 已知事实 | 可选 |
| `bocha_api_key` | 博查搜索 API Key | (可选，用于联网搜索) |
| `bing_search_api_key` | Bing 搜索 API Key | (可选，博查优先) |

### 环境变量

| 变量 | 说明 |
|------|------|
| `OLLAMA_BASE_URL` | Ollama 地址，默认 `http://localhost:11434` |
| `QQ_BOT_APP_ID` / `QQ_BOT_APP_SECRET` | QQ Bot 凭证 (可选) |
| `PET_API_LISTEN_ADDR` | API 监听地址，默认 `127.0.0.1:19840` |
| `PET_API_BASE_URL` | Pet 进程访问 API 的地址 |

### 推荐 LLM 方案

| 方案 | 模型 | 成本 |
|------|------|------|
| 低成本 | DeepSeek-V3 | ~¥1/天 |
| 高质量 | Claude / GPT-4o-mini | ~$1/天 |
| 本地 | Ollama + Qwen2.5-7B | 免费 (需 GPU) |

---

## 项目结构

```
desktop-pet/
├── main.go                    # Wails 双进程入口 (Settings + Pet)
├── app.go                     # DI 容器 + Pet IPC
├── handler_builder.go         # HTTP API 路由注册
│
├── internal/
│   ├── domain/                # 纯类型 + 接口 (零依赖)
│   │   ├── types.go           #   核心实体 (30+ 结构体)
│   │   ├── features.go        #   46 维量化特征定义
│   │   ├── memory.go          #   记忆相关接口
│   │   ├── care.go            #   关怀系统接口
│   │   ├── needs.go           #   6 维内源需求 + 满意度
│   │   └── ...
│   │
│   ├── infra/                 # 基础设施层
│   │   ├── storage/           #   SQLite (16 张表 + 迁移)
│   │   ├── llm/               #   OpenAI-compatible HTTP 网关
│   │   ├── config/            #   YAML 配置 (Viper)
│   │   ├── native/            #   macOS OCR + 窗口拖拽
│   │   └── strutil.go         #   共享字符串工具
│   │
│   ├── service/               # 业务服务层 (7 个包)
│   │   ├── cognition/         #   ⭐ 认知核心
│   │   │   ├── motivator.go   #     驱力计算 + 行动评分 + LLM 路由
│   │   │   ├── features.go    #     46 维特征计算 (Tier1+Tier2)
│   │   │   ├── decision.go    #     System 2 LLM 决策引擎
│   │   │   ├── strategy.go    #     策略实验室 (向量去重合并)
│   │   │   ├── interval.go    #     动态决策间隔
│   │   │   ├── needs.go       #     内源需求模型 (被动消退)
│   │   │   ├── learner.go     #     离线 RL 权重学习
│   │   │   ├── curiosity.go   #     好奇引擎 (缺口扫描+屏幕分析)
│   │   │   └── pattern.go     #     行为模式挖掘
│   │   ├── care/              #   关怀引擎 (触发+冷却+情绪语气)
│   │   ├── emotion/           #   情绪模型 (PAD+8维+EMA平滑+人格)
│   │   ├── memory/            #   记忆系统 (压缩+遗忘+检索+日记)
│   │   ├── identity/          #   AI 自我认知图谱
│   │   ├── diary/             #   日记生成+合并
│   │   └── scheduler/         #   自适应调度器
│   │
│   ├── app/                   # 编排层
│   │   ├── chat/              #   对话流水线
│   │   ├── background/        #   后台认知循环
│   │   └── plugin/            #   插件框架
│   │
│   ├── plugins/               # 插件
│   │   ├── memory/            #   核心插件 (接线所有服务)
│   │   ├── chat/              #   对话插件 (LLM 网关+编排+意图路由)
│   │   ├── search/            #   联网搜索插件 (博查 API, v0.2)
│   │   ├── vision/            #   截图分析
│   │   └── qq/                #   QQ Bot (WebSocket+重连)
│   │
│   └── api/                   # HTTP API + SSE
│
├── frontend/                  # React 前端
│   └── src/
│       ├── App.tsx            #   Pet 浮窗 (Live2D + SSE + 气泡)
│       ├── SettingsApp.tsx    #   设置面板路由
│       ├── components/
│       │   ├── PetCanvas.tsx  #   Live2D 渲染引擎
│       │   └── settings/      #   设置页组件
│       ├── pages/settings/    #   8 个设置页
│       └── store/             #   状态管理 + API 客户端
│
├── build/                     # Wails 构建配置
├── sion/                      # 设计文档
└── native/                    # macOS OCR Swift 源码
```

---

## 系统详情

### 情绪系统
- **PAD 三维**: Valence / Arousal / Dominance
- **8 维向量**: Affection, Worry, Curiosity, Sleepiness, Playfulness, Loneliness, Confidence, Annoyance
- **多层评估**: LLM 结构化推理 → 规则引擎回退 → EMA 平滑
- **昼夜节律**: 困倦度随小时自适应，睡眠时情绪衰减冻结
- **人格学习**: 从交互结果中学习 AnnoyanceSensitivity / AffectionWarmth / WorryTendency

### 内源需求 (6 维)
- 陪伴、休息、玩耍、好奇、关怀、自主
- **被动消退**: 向中性点 0.3 漂移，避免饱和
- **情境加速**: 深夜→休息加速，空闲→陪伴加速，工作→关怀加速
- 持久化到 SQLite，重启不丢失

### 决策系统
- **三级动态间隔**: 活跃 1min → 常规 5min → 休眠 30min
- **事件驱动**: 用户消息/App 切换 → 立即决策
- **Fast path**: 驱力评分 → 直接选择 (大部分决策)
- **LLM fallback**: 仅极端情绪/连续拒/长静默/得分接近时触发
- **深夜门控**: 22:00-08:00 仅允许休息/健康关怀
- **挂机静默**: 连续 2 条未回复 → 停止搭话，30min 衰减

### 记忆系统
- **L0 会话**: 最近 20 轮对话
- **L1 日记**: 情感日记 + 向量 (每 4h，情绪波动触发)
- **L2 事实**: 原子事实提取 + 向量检索 + Ebbinghaus 遗忘曲线
- **L3 策略**: 日反思 → 策略原则 (向量去重 + LLM 合并)

### 自学习
- **策略实验室**: 日反思 → LLM 生成策略 → 向量去重 → 相似合并
- **离线 RL**: 驱力→行动→反馈→权重更新 (DPO 风格)
- **行为模式**: 屏幕观测 → 时序分析 → 习惯挖掘
- **人格适应**: Outcome 反馈 → PersonalityScale 微调

---

## 开发

```bash
wails dev          # 开发模式 (热重载)
wails build        # 生产构建
cd frontend && npm run build  # 仅构建前端
go test ./...      # 运行所有测试
```

### 数据库

SQLite 位于 `~/.desktop-pet/memory.db`，包含 16 张表。主要表：

| 表 | 用途 |
|----|------|
| `chat_history` | 对话历史 |
| `facts` | 原子事实 + 向量 |
| `diary` | 情感日记 + 向量 |
| `identity_nodes` | AI 自我认知图谱 |
| `strategy_principles` | 策略原则 + 向量 |
| `action_outcomes` | 主动行动反馈 |
| `curiosity_items` | 知识缺口 + 探索目标 |

### 调试

- **执行状态**: 设置面板 → 执行状态 → SSE 时间线
- **驱动面板**: 仪表盘 → 概览/情绪/决策 Tab
- **策略实验室**: 查看已学策略及其置信度
- **记忆图谱**: 浏览三级记忆 + 身份节点

---

## 架构决策

| 决策 | 理由 |
|------|------|
| **双进程 (Settings + Pet)** | 设置面板崩溃不影响宠物窗口 |
| **SQLite (非向量数据库)** | 简化部署，单文件，WAL 模式足够 |
| **Ollama 本地 embedding** | 隐私优先，零网络延迟 |
| **量化特征 → 驱力 → 评分** | 可解释、可调参、可自学习 |
| **JSON 拼接 → json.Marshal** | 安全修复，避免注入 |
| **被动消退 homeostasis** | 需求不无限积累，保持区分度 |
| **向量去重合并** | 策略不重复，语义合并更优 |
| **后台动作不触发 LLM** | 节省 API 成本，observe/reflect 走 fast path |

---

## FAQ

**Q: 为什么嵌入模型用 Ollama 而不是 OpenAI API？**
A: 隐私。所有文本向量化都在本地完成，只有 LLM 推理走云端。切换模型只需改 `embedding_model` 配置。

**Q: 诗音不说话怎么办？**
A: 检查执行状态页的 SSE 时间线，观察决策日志。常见原因：深夜门控、挂机静默、连续被拒。

**Q: 可以不用 Live2D 模型吗？**
A: 可以。替换 `frontend/public/model/` 下的文件，然后修改 `ModelSelector` 中的模型路径。

**Q: 支持 Windows/Linux 吗？**
A: macOS 优先。Wails 支持跨平台，但 OCR 和部分 native 功能需要适配。

---

## License

MIT

---

<p align="center">诗音 — 不是工具，是伙伴 (๑•̀ㅂ•́)و✧</p>
