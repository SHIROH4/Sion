# Sion

诗音是一只生活在桌面上的猫娘伙伴。她观察你的行为，从互动中学习，主动搭话，逐渐变得更懂你。

**不是 Chat 窗口，不是 IDE 插件——是一个有 Live2D 形象、有情绪、有记忆的桌面伙伴。**

![License](https://img.shields.io/badge/license-MIT-blue)
![Go](https://img.shields.io/badge/Go-1.25+-00ADD8)
![Vue](https://img.shields.io/badge/Vue-3.4-4FC08D)
![Naive UI](https://img.shields.io/badge/Naive_UI-2.43-2080F0)

---

## 核心能力

| 系统 | 功能 | 说明 |
|------|------|------|
| **Live2D 桌宠** | 视线追踪、拖拽、戳一戳、气泡文字 | PIXI.js + pixi-live2d-display |
| **AI 对话** | 代码问答、Debug、技术选型、闲聊 | LLM API 驱动，流式响应 |
| **主动搭话** | 工作提醒、深夜陪伴、饭点关心 | MetaReasoner 四路仲裁 (S1/S2Lite/S2Full/None) |
| **决策引擎** | 策略规则引擎 + LLM function calling | 元认知仲裁器 (SOFAI 2025) |
| **记忆系统** | 四级记忆 (会话/日记/事实/策略) + 向量检索 | SQLite + Ollama embedding |
| **情绪系统** | PAD 三维 + 8 维向量 + 人格参数学习 | LLM 评估 + 规则回退 |
| **自学习** | 策略蒸馏 + 即时修正 + 经验注入 + 人格适应 | 统一反馈处理器 |
| **截图即问** | macOS Vision OCR + 多模态 LLM 分析 | 原生 Vision Framework |
| **屏幕感知** | 定期 OCR 屏幕、识别工作/休闲状态 | 自适应间隔 |
| **联网搜索** | 博查 API 接入，对话中搜索最新技术信息 | API tools 字段 + tool_choice |
| **工具调用** | API tools JSON Schema 字段传入 | DeepSeek 原生 tool calling |

---

## 系统架构

```
┌─────────────────────────────────────────────────────────────────────┐
│                          Sion v0.4.0                                │
│     桌面猫娘 — AI 对话 + 量化认知 + 元认知决策 + 工具调用 + 自学习      │
└─────────────────────────────────────────────────────────────────────┘

                            ┌──────────────────┐
                            │   入口 & 进程管理   │
                            │  main.go          │
                            │  SettingsApp      │
                            │  PetApp (子进程)   │
                            └────────┬─────────┘
                                     │
              ┌──────────────────────┼──────────────────────┐
              │                      │                      │
              ▼                      ▼                      ▼
┌─────────────────────┐  ┌─────────────────────┐  ┌─────────────────┐
│    对话层 (Chat)     │  │  后台认知 (Cognition)│  │ 前端 (Frontend) │
│  用户发消息时触发     │  │  每 5min tick 触发   │  │ Wails + Vue 3   │
│ + Naive UI      │
└─────────┬───────────┘  └──────────┬──────────┘  └────────┬────────┘
          │                         │                      │
          ▼                         ▼                      │
┌──────────────────────────────────────────────────────────────┐      │
│                   1. 对话层 — 消息处理流程                      │      │
├──────────────────────────────────────────────────────────────┤      │
│                                                              │      │
│  用户消息                                                     │      │
│    │                                                         │      │
│    ▼                                                         │      │
│  ┌──────────────────┐                                        │      │
│  │ handler_builder  │  HTTP API: POST /api/chat/send         │      │
│  │ detectSearchIntent│  → 加权评分检测搜索意图                  │      │
│  └────────┬─────────┘                                        │      │
│           │                                                  │      │
│     ┌─────┴─────┐                                            │      │
│     │ 是搜索?    │                                            │      │
│     └─────┬─────┘                                            │      │
│      yes  │  no                                              │      │
│           │                                                  │      │
│     ┌─────▼──────────┐    ┌──────────────────┐               │      │
│     │ Layer 2 激活    │    │ Layer 1 常驻      │               │      │
│     │ baseTools       │    │ get_memory       │               │      │
│     │ + web_search    │    │ + Memorize       │               │      │
│     │ tool_choice=    │    │ tool_choice=auto │               │      │
│     │   "required"    │    │                  │               │      │
│     └────────┬────────┘    └────────┬─────────┘               │      │
│              │                      │                         │      │
│              ▼                      ▼                         │      │
│     ┌────────────────────────────────────────┐                │      │
│     │         ChatSyncWithTools              │                │      │
│     │  Round 0: LLM 强制调工具 → 代码执行     │                │      │
│     │  Round 1: 去掉 tools → LLM 自然回复     │                │      │
│     └────────────────┬───────────────────────┘                │      │
│                      │                                        │      │
│                      ▼                                        │      │
│               ┌──────────────┐                                │      │
│               │ OnBeforeChat │  注入 system prompt            │      │
│               │ + 记忆注入    │  + 记忆上下文 + 搜索结果         │      │
│               └──────────────┘                                │      │
│                      │                                        │      │
│                      ▼                                        │      │
│               ┌──────────────┐                                │      │
│               │  LLM 回复     │  DeepSeek API                 │      │
│               │  猫娘话术      │  流式返回 → SSE → 前端渲染      │      │
│               └──────────────┘                                │      │
│                      │                                        │      │
│                      ▼                                        │      │
│               ┌──────────────┐                                │      │
│               │ OnAfterChat  │  事实提取 + 记忆归档             │      │
│               │ 记忆后处理    │  + 情绪更新 + 驱力调整           │      │
│               └──────────────┘                                │      │
│                                                              │      │
└──────────────────────────────────────────────────────────────┘      │
                                   │                                  │
                                   ▼                                  │
┌──────────────────────────────────────────────────────────────┐      │
│                2. 后台认知层 — 自主决策循环                      │      │
├──────────────────────────────────────────────────────────────┤      │
│                                                              │      │
│  BackgroundLoop (5min tick)                                   │      │
│    │                                                         │      │
│    ├─► 屏幕观察 (60s) → OCR → 工作/休闲分类                    │      │
│    │                                                         │      │
│    ├─► FeatureComputer.ComputeFull()                         │      │
│    │   ┌─────────────────────────────────────┐               │      │
│    │   │  46 维量化特征 (features.go)         │               │      │
│    │   │  ├─ User (14): App/工作/趋势...     │               │      │
│    │   │  ├─ Agent (13): 情绪8维/人格...     │               │      │
│    │   │  ├─ Environment (7): 时段/配额...   │               │      │
│    │   │  └─ Relationship (8): 接受率/拒绝...│               │      │
│    │   └─────────────────────────────────────┘               │      │
│    │                         │                               │      │
│    ├─► NeedModel.Grow()                                      │      │
│    │   ┌─────────────────────────────────────┐               │      │
│    │   │  6 维内源需求 (needs.go)             │               │      │
│    │   │  陪伴 休息 玩耍 好奇 关怀 自主        │               │      │
│    │   │  被动增长 + 饱和衰减 → 情绪调制       │               │      │
│    │   └─────────────────────────────────────┘               │      │
│    │                         │                               │      │
│    ├─► ComputeDrives(feats, needs) → 5 维驱力                │      │
│    │   Social Care Curious Quiet Explore                     │      │
│    │                         │                               │      │
│    ├─► Motivator.ScoreActions()                              │      │
│    │   ┌─────────────────────────────────────┐               │      │
│    │   │  16 个动作统一评分 (actions.go)      │               │      │
│    │   │  ├─ social: speak_casual/inquiry/   │               │      │
│    │   │  │          speak_care               │              │      │
│    │   │  ├─ care:   care_rest/meal/health..│               │      │
│    │   │  ├─ learning: search/observe/      │               │      │
│    │   │  │            reflect/analyze       │               │      │
│    │   │  └─ none                            │               │      │
│    │   │  权重矩阵 + 上下文调制 + 夜间门控     │               │      │
│    │   └─────────────────────────────────────┘               │      │
│    │                         │                               │      │
│    │              ┌──────────┴──────────┐                    │      │
│    │          fast path              LLM fallback             │      │
│    │      (得分差距 > 0.03)      (得分接近 / 极端情绪)          │      │
│    │           │                       │                     │      │
│    │           │              DecisionEngine.Decide()         │      │
│    │           │              完整上下文 + 16个SkillCard       │      │
│    │           │              → structured JSON               │      │
│    │           │                       │                     │      │
│    │           └──────────┬────────────┘                     │      │
│    │                      │                                  │      │
│    ├─► 执行 selected action                                   │      │
│    │   ┌─────────────────────────────────────┐               │      │
│    │   │  ToolRegistry 统一调度 (tool.go)     │               │      │
│    │   │  ├─ speak     → SpeakTool           │               │      │
│    │   │  ├─ search    → SearchTool (博查)    │               │      │
│    │   │  ├─ observe   → VisionLLM 截屏分析   │               │      │
│    │   │  ├─ reflect   → StrategicAgent      │               │      │
│    │   │  └─ analyze   → PatternAnalyzer     │               │      │
│    │   └─────────────────────────────────────┘               │      │
│    │                      │                                  │      │
│    └─► Outcome → Learner.BatchLearn()                        │      │
│        驱力权重更新 + StrategyLab 蒸馏 + 人格参数微调           │      │
│                                                              │      │
└──────────────────────────────────────────────────────────────┘      │
                                   │                                  │
                                   ▼                                  │
┌──────────────────────────────────────────────────────────────┐      │
│                    3. 记忆 & 情绪系统                           │      │
├──────────────────────────────────────────────────────────────┤      │
│                                                              │      │
│  记忆层级 (MemoryLayer)                                       │      │
│  ┌──────────────────────────────────────────┐                │      │
│  │ L0 会话缓冲: 最近20轮对话 (SessionBuffer)  │                │      │
│  │     ↓ OnAfterChat 触发                    │                │      │
│  │ L1 日记+情绪: 每4h / 情绪波动 → DiaryStore │                │      │
│  │     ↓ 向量检索 + LLM 合并                  │                │      │
│  │ L2 事实存储: 原子事实+向量 → Ebbinghaus 遗忘│                │      │
│  │     ↓ chat 时向量召回注入 context           │                │      │
│  │ L3 策略原则: 日反思→LLM生成→向量去重合并    │                │      │
│  └──────────────────────────────────────────┘                │      │
│                                                              │      │
│  情绪模型 (EmotionModel)                                      │      │
│  ┌──────────────────────────────────────────┐                │      │
│  │ PAD-3维: Valence Arousal Dominance       │                │      │
│  │ + 8维向量: Affection Worry Curiosity ... │                │      │
│  │ × LLM评估(云端) + 规则回退(本地)          │                │      │
│  │ × EMA平滑 + 昼夜节律 + 需求调制           │                │      │
│  └──────────────────────────────────────────┘                │      │
│                                                              │      │
│  好奇引擎 (CuriosityEngine)                                   │      │
│  ┌──────────────────────────────────────────┐                │      │
│  │ GapScan: 从已知事实找知识缺口               │                │      │
│  │   → GenerateInquiries: 缺口→探索目标       │                │      │
│  │   → VisualAnalyze: 截屏→VisionLLM→缺口    │                │      │
│  │   → search action 填补缺口                │                │      │
│  └──────────────────────────────────────────┘                │      │
│                                                              │      │
│  自学习 (Learner + StrategicAgent)                            │      │
│  ┌──────────────────────────────────────────┐                │      │
│  │ Learner: 驱力→行动→反馈→RL权重更新         │                │      │
│  │ StrategicAgent: 日反思→策略原则提取         │                │      │
│  │ PatternAnalyzer: 屏幕观测→行为模式挖掘      │                │      │
│  │ Personality: 交互结果→人格参数微调          │                │      │
│  └──────────────────────────────────────────┘                │      │
│                                                              │      │
└──────────────────────────────────────────────────────────────┘      │
                                   │                                  │
                                   ▼                                  │
┌──────────────────────────────────────────────────────────────┐      │
│                    4. 持久化 & 基础设施                         │      │
├──────────────────────────────────────────────────────────────┤      │
│  SQLite (~/.desktop-pet/memory.db)   16 张表                  │      │
│  Ollama (本地 embedding)             向量检索                  │      │
│  LLM Gateway (OpenAI-compatible)     ChatSync/ChatStream      │      │
│  HTTP API (:19840)                   SSE 事件推送             │      │
│  Plugin Manager                      生命周期 + 能力发现        │      │
└──────────────────────────────────────────────────────────────┘
```

### 对话层流程

| 步骤 | 模块 | 作用 |
|------|------|------|
| 1. 意图检测 | `detectSearchIntent()` | 加权评分判断是否搜索意图，阈值 2 分 |
| 2. 工具组合 | `filterToolsByNames()` | Layer 1: get_memory+Memorize 常驻；Layer 2: web_search 按需 |
| 3. 上下文注入 | `OnBeforeChat` | 注入 system prompt + 记忆上下文 + 搜索结果 |
| 4. LLM 调用 | `ChatSyncWithTools` | Round 0: tool_choice=required 强制调工具；Round 1: 自然回复 |
| 5. 后处理 | `OnAfterChat` | 事实提取、情绪更新、记忆压缩归档 |

### 决策层流程

| 步骤 | 模块 | 作用 |
|------|------|------|
| 1. 特征计算 | `FeatureComputer` | 46 维量化特征：用户状态(14) + Agent状态(13) + 环境(7) + 关系(8) |
| 2. 需求增长 | `NeedModel.Grow()` | 6 维内源需求被动增长 + 饱和衰减 |
| 3. 驱力计算 | `ComputeDrives()` | 5 维驱力：Social Care Curious Quiet Explore |
| 4. 动作评分 | `Motivator.ScoreActions()` | 16 动作 × 权重矩阵 × 上下文调制 × 夜间门控 |
| 5. 路由决策 | fast path / LLM fallback | 得分差 > 0.03 → fast path；否则 LLM 看完整上下文 + SkillCard 决策 |
| 6. 工具执行 | `ToolRegistry` | 统一调度 SpeakTool / SearchTool / Observe / Reflect / Analyze |
| 7. 结果学习 | `Learner` + `StrategicAgent` | RL 权重更新 + 策略蒸馏 + 人格微调 |

### 记忆层级

| 层级 | 存储 | 触发 | 用途 |
|------|------|------|------|
| L0 | SessionBuffer (内存, 20轮) | 每轮对话 | 短期上下文、话题连贯 |
| L1 | DiaryStore (SQLite + 向量) | 每4h / 情绪波动 | 情感日记、长期回顾 |
| L2 | Facts (SQLite + 向量) | 每轮提取 + Ebbinghaus遗忘 | 原子事实、知识检索 |
| L3 | StrategyPrinciples (SQLite + 向量) | 每日反思 | 行为策略、经验蒸馏 |

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
