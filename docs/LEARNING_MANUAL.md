# Sion v0.4.0 架构学习手册

> 按模块逐一深入：设计思路 → 为什么这样设计 → 备选方案对比 → 代码逻辑详解 → 调用链路 → 可优化点
> 适合逐章阅读，每章独立，可跳读。

---

## 目录

1. [进程架构与启动流程](#1-进程架构与启动流程)
2. [工具调用模块](#2-工具调用模块)
3. [特征计算模块 (FeatureComputer)](#3-特征计算模块-featurecomputer)
4. [驱力计算模块 (ComputeDrives)](#4-驱力计算模块-computedrives)
5. [v0.4 决策层重构](#5-v04-决策层重构) ← 新增
6. [门控熔断模块](#6-门控熔断模块)
7. [动态调度定时模块 (DynamicInterval)](#7-动态调度定时模块-dynamicinterval)
8. [LLM Prompt 注入模块](#8-llm-prompt-注入模块)
9. [分层记忆模块](#9-分层记忆模块)
10. [情绪模型](#10-情绪模型)
11. [自学习模块 (v0.4)](#11-自学习模块-v04)
12. [决策引擎与后台循环](#12-决策引擎与后台循环)
13. [对话后处理 (PostProcessor)](#13-对话后处理-postprocessor)
14. [前端架构](#14-前端架构)
15. [数据持久层](#15-数据持久层)
16. [并发安全与性能](#16-并发安全与性能)

---

# 1. 进程架构与启动流程

## 1.1 设计思路

Sion 采用 **双进程隔离架构**：设置面板和宠物窗口是两个独立的 Wails 进程，通过 HTTP/SSE 通信。

```
main.go 入口
  os.Args[1] == "settings" → SettingsApp (主进程)
    - 所有 AI 服务
    - HTTP API (:19840)
    - 插件管理器
    - 后台认知循环
    - spawn → PetApp (子进程)
  
  os.Args[1] == "pet" → PetApp (子进程)
    - Live2D 渲染
    - 事件接收/发送
    - 设置面板宿主
```

**为什么这样设计？**

核心原因：**隔离崩溃域**。设置面板是复杂的 SPA，可能因为前端异常崩溃。如果宠物和设置在同一进程，设置面板崩溃 = 宠物窗口也死了。双进程隔离后，宠物窗口的 Live2D 渲染不受设置面板影响。

同时，AI 服务（LLM 调用、特征计算、记忆管理）集中在主进程，宠物进程只做渲染和事件转发，是"瘦客户端"。

**备选方案对比**：

| 方案 | 优点 | 缺点 |
|------|------|------|
| 单进程多窗口 | 简单，无 IPC | 崩溃互相影响，内存占用高 |
| **双进程（当前）** | 崩溃隔离，渲染独立 | 需要 IPC，状态同步复杂 |
| 多进程 microservices | 最灵活 | 过度设计，桌面应用不需要 |

## 1.2 启动流程详解

**文件**: `app.go` → `domainReady()`

```
1. 加载配置 (infracfg.Load)
2. 初始化 LLM Gateway + Vision Gateway
3. 打开 SQLite DB (WAL 模式)
4. 构建基础设施层 (Store, EmbeddingService)
5. 构建服务层:
   - SessionBuffer (L0, 20 轮环形缓冲)
   - ← v0.3: 从 chat_history LoadHistory(20) 回填
   - DiaryStore, SelfModel, MemoryLayer
   - EmotionModel (加载持久化的情绪状态)
   - CareEngine, NeedModel
   - FeatureComputer
   - DecisionEngine, MetaReasoner, S1RuleEngine ← v0.4
   - UnifiedFeedbackProcessor, ImmediateCorrector ← v0.4
   - StrategicAgent, CuriosityEngine
6. 注册插件 (ChatPlugin, MemoryPlugin, SearchPlugin, VisionPlugin)
7. 启动插件 (Awake → Start)
   - MemoryPlugin.Start() 启动 BackgroundLoop
8. 启动 HTTP API 服务器 (:19840)
```

## 1.3 调用链路

```
app.domainReady()
  → MemoryPlugin.Start()
    → go BackgroundLoop.loop()
      → 每 tick: runTick()
        → FeatureComputer.ComputeFull()
        → ComputeDrives()
        → MetaReasoner.Route() → S1RuleEngine / S2Lite / S2Full / None
```

## 1.4 可优化点

- **启动依赖顺序敏感**: `domainReady()` 是一个巨大的构造函数，200+ 行。依赖顺序靠代码位置保证，容易出错。可以考虑引入 DI 容器（如 `wire`），但当前规模还不需要。
- **宠物进程发现的启动竞态**: `spawn PetApp` 后主进程立即开始监听，但子进程可能还没初始化完。目前靠重试和超时兜底，不够优雅。

---

# 2. 工具调用模块

## 2.1 设计思路

核心问题：如何让 DeepSeek 这种小模型可靠地调用工具？

传统做法是把工具描述写在 System Prompt 里（"你可以使用以下工具：web_search..."）。但实测发现 DeepSeek 经常忽略 Prompt 中的工具指令——它把工具描述当成角色扮演文本，而不是可调用的函数。

**当前方案**：使用 OpenAI-compatible API 的 `tools` 字段（JSON Schema 格式），`tool_choice="auto"`。

**为什么这个方案更可靠？**

`tools` 和 `messages` 是 HTTP 请求体中的**两个独立字段**。Transformer 推理时：
- `messages` → 走 self-attention，被当成对话内容
- `tools` → 走专门的 tool-calling 推理路径，不被当成角色扮演

实测验证：同样的 DeepSeek 模型，Prompt 文本注入工具描述 → 经常不调用；API tools 字段 → 稳定调用。

**备选方案对比**：

| 方案 | 可靠性 | Token 开销 | 适用场景 |
|------|--------|-----------|---------|
| Prompt 文本注入 (旧) | 低，LLM 当角色扮演忽略 | 占 ~500 tokens | 强模型 (GPT-4) |
| **API tools 字段 (当前)** | 高，独立推理路径 | 不占上下文 | DeepSeek 等中小模型 |
| tool_choice="required" | 最高，强制调用 | 每轮都调 | 需要严格工具链的场景 |
| 代码层意图路由 | 最高，完全不依赖 LLM | 零 | 确定性任务 |

## 2.2 代码逻辑

**文件**: `internal/app/chat/sync.go` — `ChatSyncWithTools()`

```
ChatSyncWithTools(ctx, messages, tools, execTool, maxRounds=3, toolChoice="auto"):

  第 1 轮:
    POST /v1/chat/completions {
      model: "deepseek-chat",
      messages: [...],        ← 文本上下文
      tools: [                ← 独立字段
        web_search,
        get_memory,
        Memorize,
        analyze_screenshot
      ],
      tool_choice: "auto"
    }
    
    → LLM 返回:
      有 tool_calls → 循环执行 → 结果注入 messages
      无 tool_calls → 返回文本
    
  第 2~3 轮 (如有 tool_calls):
    同样的请求，但 messages 中已包含上一轮的 tool_calls + tool_results
```

**工具注册**：每个 Plugin 在 `Init()` 时通过 `RegisterTool()` 注册。

```go
// memory_tags.go
func (p *MemoryPlugin) Tools() []plugin.ToolDef {
    return []plugin.ToolDef{
        {Name: "get_memory", Description: "Search long-term memory...", 
         Parameters: {...}, Handler: p.handleGetMemory},
        {Name: "Memorize", Description: "Permanently store...", 
         Parameters: {...}, Handler: p.handleMemorize},
    }
}
```

## 2.3 Tool Description 设计原则

1. **英文简洁指令** — DeepSeek 对英文 function description 的遵循度高于中文
2. **明确触发条件** — "Call when..." 开头，给出具体场景
3. **反混淆规则** — "Do NOT call for..." 防止误调用

```
web_search: "Search the web for real-time information. 
             Call when user explicitly asks to search/lookup/query...
             Do NOT call for: casual chat, common knowledge, math..."

get_memory:  "Search long-term memory... 
             Do NOT call for general knowledge — use web_search."
```

## 2.4 工具扩展策略

| 工具数 | 策略 |
|--------|------|
| ≤8 (当前) | 全量注入，`tool_choice="auto"` |
| 8-20 | 动态检索 — embedding 匹配 Top-K 注入 |
| 20+ | Skill 分组 + 渐进式披露 (参考 Claude Code) |

## 2.5 可优化点

- **当前仅 DeepSeek 测试过**: 如果用 Claude/GPT-4，`tool_choice="auto"` 可能太保守，可以按模型切换策略
- **工具结果格式不统一**: 每个工具的返回格式由各 Plugin 自行决定，建议统一为 `{success, data, error}` 结构
- **缺少工具调用指标**: 应记录每个工具的调用次数、成功率、平均耗时，用于调优 tool description

---

# 3. 特征计算模块 (FeatureComputer)

## 3.1 设计思路

核心问题：纯 Prompt 决策无法量化"主人忙不忙""搭话几次了""连续被拒多少次"。

FeatureComputer 将系统状态量化为 **46 维浮点特征**，归一化到 [0,1] 或 [-1,1]，使 System 1 数学决策成为可能。

**为什么需要量化特征？**

纯 LLM 决策的问题：
1. 不可审计 — 不知道为什么选 speak 而不是 search
2. 高成本 — 每次决策都要调 LLM
3. 不可复现 — 同样的状态可能产生不同决策

量化特征 → 数学公式 → 确定性输出。可调试、零成本、可复现。

**特征分组 (5 组)**：

| 组 | 含义 | 维度 | 示例 |
|----|------|------|------|
| A (Agent) | 诗音自身状态 | 20 | 情感、困倦、行动次数、成功率 |
| U (User) | 主人状态 | 14 | 工作状态、消息趋势、响应延迟 |
| E (Environment) | 环境时间 | 8 | 小时、星期、冷却、配额 |
| R (Relationship) | 互动关系 | 8 | 接受率、拒绝数、亲密趋势 |
| T (Task) | 任务上下文 | 4 | 策略数、模式数、反思日志 |

## 3.2 两段式计算

**文件**: `internal/service/cognition/features.go`

```
ComputeFull(feats, emotion, needs, ...):

  Tier 1 (纯内存, ~1ms):
    - U3-U5: app/working/switch_count (来自 CareEngine 状态)
    - U11-U13: meal/night/weekend (时间判断)
    - U14: timeSinceChat = now - lastChatTime
    - E1-E3: hour, day, cooldown (时间 + 内存)
    - A6: dailyActionCount (内存计数)
    - A14: consecutive count (内存计数)

  Tier 2 (SQL 聚合, ~50ms, TTL 缓存 5 分钟):
    - A7: SELECT success_rate FROM outcomes GROUP BY action_type
    - A8: SELECT success_rate FROM outcomes GROUP BY time_block
    - R1-R4: 聚合 outcomes 表 (最近 24h)
    - U10, U15, U16: 复杂查询 + 向量检索
    - A11-A13: curiosity/learning 计数
```

**TTL 缓存实现** (`features.go` lines 297-565)：

缓存不是简单的"5 分钟全局 TTL"，而是**按因子粒度**存储到 SQLite `feature_cache` 表，每个因子有独立的 TTL：

| TTL | 因子 | 原因 |
|-----|------|------|
| **5 分钟** | R5 (冷落时长) | 随聊天频率快速变化 |
| **1 小时** | U4,U5,U7,U8,U10,R2,A7,A8,A10,R1,R3,R4,R7,T5 | 中等变化频率 |
| **6 小时** | U2,U15,U16,A13,R6 | 慢速变化 |
| **24 小时** | R8 (亲密趋势) | 长期趋势 |

```go
func getCachedFloat(name string, ttl int64, compute func() float64) float64 {
    if db == nil { return compute() }  // 无 DB → 每次都算
    cached := loadFromCache(name)
    if cached != nil && time.Now().Unix() - cached.ComputedAt <= ttl {
        return cached.Value
    }
    val := compute()
    saveToCache(name, val, ttl)
    return val
}
```

## 3.3 归一化函数 saturateNorm

```go
func saturateNorm(value, saturationPoint float64) float64 {
    return math.Tanh(value / saturationPoint * 2.0)
}
```

使用 `tanh(x/饱和点×2)` 将 [0, 饱和点] 映射到 [0, ~0.96]，平滑渐近 1.0。比 `min(v/sat, 1)` 的线性截断更平滑，避免边界突变。

## 3.4 归一化策略

| 原始值 | 归一化方法 | 示例 |
|--------|-----------|------|
| 连续工作分钟 | `min(raw/180, 1.0)` | 90min → 0.5 |
| App 切换次数 | `min(raw/20, 1.0)` | 10次 → 0.5 |
| 响应延迟 | `min(raw/300, 1.0)` | 60s → 0.2 |
| 今日行动数 | `min(raw/20, 1.0)` | 5次 → 0.25 |
| 连续同动作 | `min(raw/5, 1.0)` | 3次 → 0.6 |

## 3.5 内源需求模型 (NeedModel)

**文件**: `internal/service/cognition/needs.go`

6 维需求是驱力计算的"燃料"——需求越高，对应驱力越强。

```
需求衰减: 所有需求向基线 0.3 以 0.03/h 回归 (homeostasis)
需求增长:
  Companionship: +0.03/h  (+0.02/h when idle > 1h)
  Rest:          +0.04/h  (+0.08/h during 23:00-02:00)
  Play:          +0.03/h  (-0.04/h during 01:00-05:00) → 夜间归零
  Curiosity:     +0.05/h  (持续增长，探索驱动)
  Care:          +0.03/h  (+0.05/h when user is working)
  Autonomy:      +0.02/h  (+0.04/h when idle > 2h)
```

每次行动后的**需求满足** (`NeedSatisfactionForAction()`):
- 对应需求降低 0.20-0.40
- 被拒绝时 companionship +0.15 (惩罚——越被拒越想社交)

## 3.6 可优化点

- **Tier 2 缓存是时间驱动而非事件驱动**: 如果 5 分钟内 outcome 表有大量写入，缓存不会失效。改为事件驱动失效更精确。
- **向量检索是全表扫描**: 当前数据量 <10k 够用，超过后需要 ANN 索引。
- **46 维 → 可能存在冗余**: 某些维度高度相关（如连续工作分钟和工作标志），可以做 PCA 降维。

---

# 4. 驱力计算模块 (ComputeDrives)

## 4.1 设计思路

5 维驱力是将 46 维特征压缩为 **5 个可解释行为方向** 的数学变换层。

```
46 维特征 → 5 维驱力 → 16 动作评分 → 1 个最优动作
```

**为什么是 5 维？**

参考游戏 AI 的 Utility AI 设计（The Sims 4 NPC 行为选择）：不是直接 46→16 映射，而是中间插入一个低维、可解释的"动机层"。5 个维度覆盖了猫娘的核心行为动机：

- **Social**: 想和主人互动
- **Care**: 关心主人状态
- **Curious**: 想了解新事物
- **Quiet**: 想保持安静
- **Explore**: 想探索未知

每个维度 = 情感基值 (~50%) + 需求推动 (~15%) + 用户上下文 (~20%) + 关系门控 (~15%)。

## 4.2 五大驱力公式

**文件**: `internal/service/cognition/decision.go` — `ComputeDrives()`

### Social Drive

```
social = 0.40 × clamp(loneliness)
       + 0.25 × clamp(playfulness)
       + 0.20 × idleBonus          // 空闲 >2h → 1.0
       + 0.10 × clamp(affection)
       + 0.05 × (1.0 - clamp(annoyance))
       + needs.Companionship × 0.12
       + needs.Play × 0.08
       - U3_isWorking × 0.15       // 工作→抑制
       - U12_nightTime × 0.15      // 深夜→抑制
       - R4_rejectionSeverity × 0.35  // 被拒→强烈抑制
       × interactionGate(R1_acceptRate)
```

### Care Drive

```
care = 0.40 × clamp(worry)
     + 0.20 × clamp(affection)
     + 0.15 × nightBonus          // 深夜 0.6
     + 0.10 × mealBonus           // 饭点 0.5
     + needs.Care × 0.18
     + U4_continuousWorkNorm × 0.15
     + (night & working) × 0.10   // 深夜工作→额外关怀
     + U13_isWeekend × 0.05
     × interactionGate(R1_acceptRate)
```

### Curious Drive

```
curious = 0.35 × clamp(curiosity)
        + 0.25 × hasInquiry
        + 0.20 × hasGaps
        + 0.15 × (1 - timeFactor)
        + needs.Curiosity × 0.18
        + A13_learningMomentum × 0.07
        + U16_prefDiversity × 0.05
        × (0.7 + gate × 0.3)      // 关系门控 (弱)
```

### Quiet Drive

```
quiet = 0.20 × clamp(sleepiness)
      + 0.15 × timeFactor         // 刚聊完→偏安静
      + 0.25 × clamp(annoyance)   // 烦躁→安静
      + 0.10 × idleBias
      + needs.Rest × 0.18
      + (1 - E3_cooldownNorm) × 0.15
      + U3_isWorking × 0.12
      + U12_nightTime × 0.08
      + (quota < 5) × 0.10        // 配额低→省着用
      + R4_rejectionSeverity × 0.40  // 被拒→安静
```

### Explore Drive

```
explore = 0.30 × clamp(curiosity)
        + 0.20 × (1 - timeFactor)
        + gapBoost × 0.25
        + needs.Curiosity × 0.15
        + needs.Autonomy × 0.15
        + (¬ working) × 0.08
        + E7_reflectionDue × 0.10
        + inquiryBoost × 0.12
        + (minutesSinceAction > 30) × 0.10
        × (0.8 + gate × 0.2)
```

## 4.3 interactionGate 函数

```go
func interactionGate(acceptRate float64) float64 {
    if acceptRate <= 0  { return 1.0 }  // 无数据→不抑制
    if acceptRate >= 0.5 { return 1.0 } // 健康→不抑制
    return 0.5 + acceptRate             // 0.0→0.5, 0.5→1.0
}
```

**设计思想**：当接受率低于 50% 时，gate 线性降低 social 和 care 驱力。最低为 0.5（不会完全归零）。这样被频繁拒绝时猫娘会变得"识趣"，但不会永远沉默。

## 4.4 备选方案对比

| 方案 | 优点 | 缺点 |
|------|------|------|
| **手写加权公式（当前）** | 可解释、可调试、零成本 | 需要人工调参 |
| 神经网络直接 46→5 | 自动学习最优权重 | 不可解释、需要训练数据、对冷启动不友好 |
| LLM 直接决策（无驱力层） | 最灵活 | 成本高、不可复现、latency 大 |

## 4.5 可优化点

- **权重是硬编码的**: 可以考虑用 Learner 的 RL 更新**驱力公式本身的权重**，不只是动作权重
- **Social 和 Care 有时会冲突**: 比如深夜主人工作→ care 高但 social 低。当前靠各自 gate 独立处理，没有显式的冲突解决

---

# 5. 动作打分模块 (Motivator) — v0.3, 已弃用

> ⚠️ v0.4 中 ScoreActions + contextModulator 已被 S1RuleEngine + MetaReasoner 替代。
> 权重矩阵保留作为冷启动默认值，但不再被 Learner 自学习修改。
> 以下为 v0.3 的设计文档，仅作历史参考。

## 5.1 设计思路 (历史)

将 5 维驱力映射到 16 个具体动作上，选最优。纯数学计算——点积 + 倍率调制 + clamp。

```
Drive(social=0.7, care=0.3, curious=0.2, quiet=0.1, explore=0.1)
  × Weight Matrix (16×5)
  + CareEngine bonus
  × contextModulator (8 factors)
  = 16 final scores → max → selected action
```

## 5.2 16 动作权重矩阵

**文件**: `internal/service/cognition/actions.go` — `BuildWeightsMap()`

| Action | Social | Care | Curious | Quiet | Explore | NightSafe | Category |
|--------|--------|------|---------|-------|---------|-----------|----------|
| speak_casual | 0.80 | 0.15 | 0.05 | -0.30 | 0.00 | no | social |
| speak_care | 0.40 | 0.70 | 0.00 | -0.20 | 0.00 | no | social |
| speak_inquiry | 0.40 | 0.00 | 0.60 | 0.00 | 0.10 | no | social |
| care_rest | 0.10 | 0.75 | 0.00 | 0.00 | 0.00 | yes | care |
| care_meal | 0.10 | 0.70 | 0.00 | 0.00 | 0.00 | no | care |
| care_hydration | 0.05 | 0.65 | 0.00 | 0.00 | 0.00 | yes | care |
| care_health | 0.05 | 0.65 | 0.00 | 0.00 | 0.00 | yes | care |
| care_encourage | 0.20 | 0.55 | 0.00 | 0.00 | 0.00 | no | care |
| care_social | 0.30 | 0.40 | 0.00 | 0.00 | 0.00 | no | care |
| search | 0.05 | 0.05 | 0.45 | -0.10 | 0.30 | yes | learning |
| observe | 0.10 | 0.00 | 0.30 | 0.00 | 0.60 | yes | learning |
| reflect | 0.00 | 0.00 | 0.00 | 0.20 | 0.75 | yes | learning |
| analyze_patterns | 0.00 | 0.00 | 0.20 | 0.00 | 0.65 | yes | learning |
| none | 0.00 | 0.00 | 0.00 | 1.00 | 0.00 | yes | none |

**NightSafe 标记**：深夜 (22:00-08:00) 只允许 NightSafe=yes 的动作，防止打扰主人休息。

## 5.3 得分计算

```go
// 1. 基础点积得分
baseScore = social × w.Social
          + care × w.Care
          + curious × w.Curious
          + quiet × w.Quiet
          + explore × w.Explore

// 2. CareEngine 建议加成
if suggestion exists for this action:
    baseScore += (0.30 - priority × 0.05)
    // priority 1 → +0.25, priority 4 → +0.10

// 3. 上下文调制倍率
finalScore = baseScore × contextModulator(action, feats)
```

## 5.4 contextModulator — 8 个调制因子

**文件**: `internal/service/cognition/motivator.go`

```go
func contextModulator(action, feats):
    m := 1.0

    // 1. 历史成功率 (A7)
    //    rate=0 → ×0.4, rate=1 → ×1.0
    m *= 0.4 + rate × 0.6

    // 2. 来源接受率 (R3)
    //    rate=0 → ×0.5, rate=1 → ×1.0
    m *= 0.5 + rate × 0.5

    // 3. 时间窗口偏好 (U10) — 仅 social 类
    m *= 0.4 + U10 × 0.6

    // 4. 用户投入度 (U8) — 仅 casual/care
    m *= 0.6 + U8 × 0.4

    // 5. 对话深度趋势 (R6) — inquiry 加分
    if R6 > 0.2: m *= 1.0 + R6 × 0.3  (max ×1.3)

    // 6. 活跃探索目标 (A11) — inquiry 加分
    m *= 1.0 + saturate(A11, 3) × 0.3

    // 7. 用户疏远 (U7) — social 降权
    if U7 < -0.3: m *= 1.0 + U7 × 0.4  (min ×0.6)

    // 8. search 专属调制
    m *= 1.0 + saturate(A11,5) × 0.3    // 有目标→加分
    m *= 1.0 + saturate(A12,5) × 0.2    // 有缺口→加分
    m *= 1.0 + A13 × 0.1                // 学习势头
    m *= 0.3 + E3 × 0.7                  // 冷却→降权
    if E4 < 3: m *= 0.3                   // 配额保护
    if U3 > 0.5: m *= 1.0 + U3 × 0.15    // 写代码→加分
    
    return clamp(m, 0.1, 1.5)
```

**设计思想**：contextModulator 实现了"从历史中学习"的轻量版——如果某个动作过去成功率低，现在自动降权。不需要等 Learner 的 batch 更新就能即时生效。倍率被严格夹在 [0.1, 1.5]，避免任何单一因素产生过激 swing。

## 5.5 可优化点

- **权重矩阵初始化是手动设定的**: 可以通过 A/B 测试或用户研究来校准初始权重
- **CareEngine 建议加成是线性衰减**: `(0.30 - priority × 0.05)` — 可以考虑非线性衰减，让 priority 1（紧急）获得更大的分辨度
- **contextModulator 因子之间可能有交叉**: 比如"历史成功率"和"来源接受率"可能高度相关。但当前设计是独立乘法叠加，可能放大或缩小效果

---

# 5. v0.4 决策层重构

> **设计参考论文**: SOFAI (IBM/Oxford, CACM 2025), RaDAgent (Tsinghua, ICLR 2025), Agent-R (2025), CogniWeb (BUPT, 2025)

## 5.1 为什么重构

v0.3 的决策层有三个根本问题：

1. **两条学习链路互不相通**: Learner 学权重（System 1），StrategicAgent 学策略（System 2），学同一个东西但各学各的
2. **RouteToLLM 是硬编码**: 8 个 if 条件，不是动态评估
3. **ScoreActions 是线性公式**: 无法处理多因素交互（"深夜 + 主人情绪异常 + 刚被拒绝过"）

论文调研后确定新方向：元认知仲裁 + 策略规则引擎 + 经验注入。

## 5.2 新架构

```
安全急停层 (门控熔断, 0ms, 0 token)
  → 通过
元认知仲裁器 (MetaReasoner)
  评估: 复杂度/风险/策略覆盖度/置信度
  → 四路仲裁:
    PathNone   — 门控禁止, 不行动
    PathS1     — 策略规则引擎, 0 token
    PathS2Lite — 轻量 LLM, ~200 tokens
    PathS2Full — 完整 LLM, ~370 tokens
```

**文件**: `internal/service/cognition/meta_reasoner.go`

```go
func (m *MetaReasoner) Route(feats, ruleResult, hasConflict, hasExtremeEmotion) DecisionPath {
    risk := m.computeRisk(feats)
    if risk >= m.HighRisk { return PathS2Full }
    if ruleResult != nil && !ruleResult.NeedsLLM { return PathS1 }
    if ruleResult != nil && ruleResult.NeedsLLM {
        if hasConflict || hasExtremeEmotion { return PathS2Full }
        return PathS2Lite
    }
    // no rule → compute complexity for lite vs full
    complexity := m.computeComplexity(feats, hasConflict)
    if complexity >= HighComplexity || hasExtremeEmotion { return PathS2Full }
    return PathS2Lite
}
```

## 5.3 S1 策略规则引擎

替代 ScoreActions（点积 + contextModulator）。不再做 16 次点积，改为策略规则匹配。

**文件**: `internal/service/cognition/rule_engine.go`

```go
type StrategyRule struct {
    Condition         ConditionExpr  // 编译后的匹配条件
    RecommendedAction string         // 推荐动作
    Suppress          []string       // 抑制动作
    Boost             []string       // 增强动作
    Confidence        float64        // 从历史反馈学习
    HitCount          int
    AcceptRate        float64        // EMA 滑动窗口
}
```

规则来源: StrategicAgent 策略蒸馏 + ImmediateCorrector 即时修正。

## 5.4 System 2: Function Calling

16 个动作全部转为 LLM tools，`tool_choice="required"` 强制选择。

**文件**: `internal/service/cognition/actions.go` — `BuildDecisionTools()`

```
LLM 收到的 tools:
  speak_casual, speak_care, speak_inquiry,
  care_rest, care_meal, care_hydration, care_health,
  care_encourage, care_social,
  search, observe, reflect, analyze_patterns,
  none
```

分级上下文:
- S2-Full: 完整 prompt (~370 tokens, 含情绪/需求/策略/经验)
- S2-Lite: 精简 prompt (~200 tokens, 仅关键信息)

## 5.5 经验注入

每次决策自动注入相似场景的历史案例（RaDAgent 启发）。

**文件**: `internal/service/cognition/feedback.go`

```go
type ExperienceRecord struct {
    Action   string    // 当时的动作
    Outcome  string    // "accepted" / "ignored" / "rejected"
    Summary  string    // "speak_casual '在写什么呀' → 被无视"
    FeatSnap string    // 场景指纹
}
```

S2-Full 决策时，自动注入相似场景最近 3 条经验到 prompt 中。

## 5.6 与 v0.3 的差异

| | v0.3 | v0.4 |
|---|---|---|
| S1 决策 | ScoreActions (16次点积 + 8因子) | S1RuleEngine (策略规则匹配) |
| S1/S2 分流 | RouteToLLM (8个硬编码条件) | MetaReasoner (4维动态评估) |
| S2 输出 | Prompt → JSON 解析 | Function calling (tool_choice="required") |
| LLM 调用 | 单一上下文 | S2Lite (~200t) / S2Full (~370t) |
| 学习机制 | 7个独立 | 1个入口 + 3个出口 |
| 即时反馈 | 6h 批处理 | ImmediateCorrector (秒级) |

---

# 6. 门控熔断模块

## 6.1 设计思路

**硬熔断**（动作完全不可选）和**软抑制**（倍率降低得分）两层保护，防止在不合适的时机打扰用户。

参考了**电路设计中的熔断器模式**：正常时透明，异常时快速切断，有一定的恢复机制。

## 6.2 硬熔断规则

| 熔断 | 触发条件 | 拦截动作 | 设计原因 |
|------|---------|---------|---------|
| 夜间门控 | U12_NightTime > 0 (22:00-08:00) | 所有非 NightSafe 动作 | 深夜只允许休息/健康/搜索/观察/反思/none |
| 配额耗尽 | E4_QuotaRemaining ≤ 0 | 所有非 none 动作 | 每天 20 次主动行动硬上限 |
| 连续未回复 | consecutiveUnanswered ≥ 2 | 所有 speak/care_* 动作 | 2 次搭话没回→停止，给用户空间 |

## 6.3 软抑制规则

| 抑制 | 机制 | 倍率 |
|------|------|------|
| 话题重叠保护 | `recentChatAlreadyCovers(action)` → false | 该 action 得分 = 0 |
| 晚安重复保护 | 最近消息含"晚安/睡了/早点睡" | speak_casual/inquiry/care → 0 |
| 拒绝衰减 | R4_RejectionSeverity > 0 | social −0.35×sev, quiet +0.40×sev |

## 6.4 恢复逻辑

```go
// 连续未回复 → 30分钟慢慢恢复
if consecutiveUnanswered >= 1 && timeSinceLastProactive > 30min {
    consecutiveUnanswered--
}

// 拒绝记忆 → 30分钟过期
if timeSinceRejected > 30min {
    delete(rejectedActions, action)
}
```

## 6.5 CareEngine 关怀引擎详解

**文件**: `internal/service/care/care_engine.go`

CareEngine 是一套独立的关怀触发系统，在后台循环的每个 tick 中运行，非破坏性地评估 6 种关怀触发器：

| 触发器 | 条件示例 | 优先级 |
|--------|---------|--------|
| Rest | 连续工作 > threshold AND 深夜 | 1 (紧急) |
| Meal | 当前时间在饭点窗口 | 2 |
| Hydration | 连续工作 > 1.5h | 3 |
| Health | 连续工作 > 3h | 3 |
| Encourage | 疲劳提及 或 工作 + 低效价 | 3 |
| Social | 高寂寞 + 非工作时间 | 4 |

**安全过滤链** (在生成建议前):
```
1. AnnoyanceLevel > 0.7 → 全部跳过 (猫娘太烦了)
2. FocusLevel > 0.85 → 全部跳过 (主人太专注了)
3. Night (22:00-08:00) → 只通过 rest 和 health
4. Focus > 0.8 → 只通过 priority 1
5. Emotion annoy > 0.5 → 跳过非紧急触发器
```

`Suggestions()` 非破坏性地评估所有触发器（不更新 lastFiredAt），返回 `[]CareSuggestion`。这些建议被 Motivator 消费，给对应动作加分。

**Scheduler 自适应参数** (`scheduler.go`):
- Cooldown 按来源区分: Care=20min, KnowledgeGap=30min, Casual=10min
- 接受率 > 0.5 → cooldown 缩短到 8min
- 接受率 ≤ 0.5 → cooldown 延长到 25min
- 每日配额自适应: 接受率 > 0.6 → +5 (max 70), 接受率 < 0.3 → -5 (min 25)

## 6.6 可优化点

- **恢复是线性衰减而非指数衰减**: 连续被拒后应该在更长时间内保持抑制，而不是 30 分钟后突然恢复
- **没有"用户主动发起对话"的重置机制**: 如果用户主动发消息了，应该更快地重置拒绝计数

---

# 7. 动态调度定时模块 (DynamicInterval)

## 7.1 设计思路

原固定 5 分钟 tick 有两个问题：
1. 活跃时不够快 — 错过搭话窗口
2. 休眠时浪费资源 — 空转 CPU

动态间距根据当前状态自适应调节。

**参考**: 操作系统的 CPU 调度器——根据负载动态调整时间片。

## 7.2 三级阶梯规则

**文件**: `internal/service/cognition/interval.go`

```go
func DynamicInterval(
    timeSinceChatMin, isWorking, continuousWorkMin float64,
    isNight, rejectionSeverity, quotaRemaining float64,
    socialDrive, careDrive, curiousDrive float64
) time.Duration {

    // Tier 3: Dormant (长间距)
    if quotaRemaining <= 0         → 60 min   // 配额耗尽
    if isNight                     → 30 min   // 深夜
    if rejectionSeverity > 0.5     → 30 min   // 连续被拒
    if continuousWork > 120min && isWorking → 15 min  // 深度工作

    // Tier 1: Active (短间距)
    hasHighDrive := social > 0.7 || care > 0.7 || curious > 0.7
    if timeSinceChat < 10min && hasHighDrive → 1 min
    if timeSinceChat < 10min                  → 3 min
    if hasHighDrive                           → 3 min

    // Tier 2: Normal (基线)
    return 5 min
}
```

## 7.3 屏幕观察间距自适应

```go
func AdaptiveScreenInterval(decisionInterval):
    raw := decisionInterval / 3       // 1/3 的决策间隔
    return clamp(raw, 30s, 120s)      // 夹到 [30s, 120s]
```

**思想**: 决策越频繁 → 屏幕观察也越频繁，保持感知时效性。但不低于 30 秒避免过度消耗视觉 API。

## 7.4 事件驱动插队

```go
// BackgroundLoop.Wake() — 非阻塞 channel send
select {
case l.wakeCh <- struct{}{}:
default:  // channel 满了就不发,防止堆积
}
```

触发插队场景：用户发消息、App 切换、情绪尖峰。

## 7.5 可优化点

- **阶梯是硬编码的，不够平滑**: 可以从三级阶梯改为连续函数（sigmoid），避免边界跳变
- **没有根据一天中的时间学习用户模式**: 比如用户每天下午 3 点午休，系统应该学习这个模式并调整间隔

---

# 8. LLM Prompt 注入模块

## 8.1 设计思路

System Prompt 仅保留核心人格定义 (~300 字)。工具规则不再写入 Prompt——通过 API tools 字段独立传入。

**为什么 Prompt 越短越好？**

1. DeepSeek 对长 Prompt 的注意力分散——越长的 Prompt，关键指令越容易被忽略
2. tools 字段和 messages 在 Transformer 走不同推理路径——不共用注意力预算
3. 300 字约 100 tokens，是 DeepSeek 保持稳定角色扮演 + 同时关注 tools 的最佳窗口

## 8.2 Prompt 分区策略

**文件**: `internal/plugins/chat/system_prompt.go`

```
<identity>      4行 — 猫娘身份 + 回复风格
<user>          1行 — 称呼 + 技术栈
<time>          1行 — 当前时间 + 时段
<self_and_emotion> — 占位注释 (实际数据由 Processor 动态注入)
```

## 8.3 上下文注入顺序

由 `Processor.OnBeforeChat` 按以下顺序注入（从后往前 prepend，最终从上到下）：

```
1. System Prompt (身份, ~100 tokens)        ← 最顶部
2. 个性摘要 (3行, ~100 tokens)
3. 屏幕上下文 (当前 App/窗口)
4. 身份图谱 (前 3 个节点)
5. 自我模型 (L3)
6. 情绪状态 (8 维 + 行为提示)
7. 用户画像 (称呼 + 技术栈)
8. 时间筛选事实 (若有时间关键词)
9. 语义检索 (L2 向量 Top-3)                 ← 中间
10. 会话缓冲 (L0 最近 10 轮)
11. 用户消息                                 ← 最底部 (LLM 近期注意力最强)
```

**为什么这样排列？**

利用了 LLM 的"首尾注意力偏差"——最重要的指令放在两端：
- 顶部：角色人格定义（必须记住）
- 底部：最近对话和用户输入（需要直接回应）
- 中间：记忆和情绪上下文（辅助理解）

## 8.4 可优化点

- **没有做 Prompt 长度监控**: 如果所有注入加起来超过模型上下文窗口，会静默截断。应该加 token 计数和裁剪逻辑
- **语义检索结果没有排序**: Top-3 是向量距离排序，但 LLM 实际更需要"最相关"而非"最相似"。可以用 LLM 重新排序

---

# 9. 分层记忆模块

## 9.1 设计思路

参考 **Generative Agents (Stanford, 2023)** 的记忆流架构和 **MemGPT** 的虚拟上下文管理。

```
L0 会话缓冲 (20轮, 30min max-age)
  ↓ (每4h/情绪波动)
L1 日记 (LLM 生成标题+摘要+向量)
  ↓ (每日整合)
L2 原子事实 (向量检索 + Ebbinghaus 遗忘)
  ↓ (定期反思)
L3 策略原则 / 自我模型 (LLM 提炼 + 向量去重)
```

**为什么分层？**

认知科学中的人类记忆模型：工作记忆 (L0) → 情景记忆 (L1) → 语义记忆 (L2) → 核心认知 (L3)。分层设计使得：
1. 高频查询用快速缓存 (L0)
2. 低频但重要的事实有持久存储 (L2)
3. 长期模式通过反思提炼为策略 (L3)

## 9.2 L0 会话缓冲

```
结构: 环形切片, 20 轮
写入: OnAfterChat (对话结束)
读取: OnBeforeChat (对话开始, 注入最近 10 轮)
持久化: v0.3 新增启动时 LoadHistory(20) 从 DB 回填
跨进程: OnBeforeChat 中 LoadHistory(10) 同步设置↔宠物窗口
```

**为什么用环形缓冲而不是全量存储？**

1. 内存可控 — 最多 20 条消息
2. 时间局部性 — 最近的对话最有价值，旧消息快速淘汰
3. max-age 30min 防止过多无关上下文污染 LLM 注意力

## 9.3 L1 日记

```
触发条件: 每 4 小时 + 情绪波动 (Valence 变化 > 0.3)
内容: LLM 用最近 20 轮对话生成标题 + 摘要 + 情绪标签
存储: SQLite diary 表 + Ollama 向量 embedding
每日上限: 3 篇
```

## 9.4 L2 原子事实

```
提取: OnAfterChat → LLM extract → AtomicFact[]
过滤:
  - qualifyFactContent(): 长度 < 5 字 → 拒绝
  - 噪音模式: "就行/好了/算了" → 拒绝
  - 疑问句: "什么/谁/哪/怎么" 开头 → 拒绝
  - 向量相似度 > 0.85 → 合并而非新增

存储: SQLite facts 表 + vector BLOB
召回: cosine similarity 检索 Top-3 注入 system prompt
遗忘: Ebbinghaus 曲线 — Forgot(): last_recalled 超阈值 → archived
```

## 9.5 L3 策略原则

```
触发: StrategicAgent.ShouldRun() → 距上次 > 6h + idle > 30min
流程:
  1. 收集: 最近 24h ActionOutcome + 活跃 Facts + 日记
  2. LLM 分析: 成功/失败模式
  3. 生成: StrategyPrinciple { situation, good_strategy, bad_strategy, reason }
  4. 去重: 向量相似度 > 0.75 → 合并或替换
  5. 存储: SQLite + 向量

注入: 决策时作为 ActivePrinciples 传给 LLM Fallback
```

## 9.6 记忆注入策略对比

| 层级 | 注入方式 | 触发 | 注入量 |
|------|---------|------|--------|
| L0 | 被动注入 (每次对话) | OnBeforeChat | 最近 10 轮 |
| L1 | 被动注入 (语义搜索) | OnBeforeChat | Top-3 中混合 |
| L2 | 被动 (Top-3) + 工具 (get_memory) | OnBeforeChat + LLM 调用 | Top-3 注入，更多按需 |
| L3 | 被动注入 | OnBeforeChat | 全部 SelfModel |

## 9.7 可优化点

- **Ebbinghaus 遗忘是简化实现**: 当前仅基于 `last_recalled` 时间，没有考虑 fact 的 importance 和 recall_count。完整实现应该是 `decay = e^(-time/strength)` where strength = importance × recall_count
- **日记生成条件太粗糙**: `shouldGenerateDiary()` 只检查时间间隔和消息数量，没有考虑对话的"情感密度"——如果 10 分钟内发生了重要的情感对话，应该直接触发
- **L1 和 L2 有重复存储**: 日记和事实都各自向量化存储，可能存储了相同的信息。可以加跨层去重

---

# 10. 情绪模型

## 10.1 设计思路

参考 **Mehrabian & Russell (1974) PAD 三维情绪模型**，在此基础上扩展为 8 维拟人情感向量。

**为什么需要情绪模型？**

1. 驱力计算的输入 — 情绪 → social/quiet 驱力
2. LLM 上下文注入 — 让对话风格符合当前情绪
3. UI 展示 — 仪表盘情绪面板
4. 人格学习 — 影响长期行为倾向

## 10.2 PAD 三维推导

```
Valence   = affection - annoyance          ← [-1, 1]  愉悦-不悦
Arousal   = (playfulness + curiosity)/2 - sleepiness  ← [-1, 1]  兴奋-平静
Dominance = confidence - 0.5 - worry × 0.3             ← [-1, 1]  支配-顺从
```

**文件**: `internal/service/emotion/emotion.go` — `computeState()`

PAD 不是直接存储的，而是从 8 维向量**派生**的。这意味着 8 维向量是唯一的真相来源 (Single Source of Truth)。

## 10.3 8 维情绪向量

| 维度 | 中性值 | 衰减率/hr | 含义 |
|------|--------|----------|------|
| Affection | 0.5 | 0.005 | 对主人的亲近感 |
| Worry | 0.0 | 0.08 | 对主人的担忧 |
| Curiosity | 0.5 | 0.05 | 对世界的好奇 |
| Sleepiness | 0.0 | — | 困倦度（昼夜节律驱动） |
| Playfulness | 0.5 | 0.05 | 玩心 |
| Loneliness | 0.0 | 0.03 | 寂寞感 |
| Confidence | 0.5 | 0.008 | 自信 |
| Annoyance | 0.0 | 0.12 | 烦躁度 |

**中性值 != 0** 的设计：Affection/Curiosity/Playfulness/Confidence 的中性是 0.5（健康状态），而 Worry/Loneliness/Annoyance 的中性是 0.0（没有负面情绪）。这反映了"正常猫娘是开心的"这一设计假设。

## 10.4 EMA 平滑

每个维度有不同的平滑系数，反映"情绪惯性"：

```go
smoothAffection  = 0.10  // 亲密度变化慢 (α小)
smoothConfidence = 0.15
smoothCuriosity  = 0.30
smoothPlayfulness = 0.30
smoothWorry      = 0.60  // 担忧变化快 (α大)
smoothAnnoyance  = 0.70  // 烦躁变化最快

blend(old, new, α) = old × (1-α) + new × α
```

**为什么不同维度平滑系数不同？**

进化心理学观点：负面情绪应该快速响应（危险信号），正面情绪应该缓慢建立（信任积累）。所以 Annoyance 和 Worry 的 α 很大（对环境敏感），Affection 和 Confidence 的 α 很小（缓慢积累）。

部分系数还被**人格参数**调制：

```go
affectionAlpha = 0.10 × (0.5 + AffectWarmth)  // 温暖的猫娘更容易产生亲近感
worryAlpha     = 0.60 × (0.5 + WorryTendency)  // 爱担心的猫娘更容易焦虑
annoyAlpha     = 0.70 × AnnoyanceSensitivity   // 敏感的猫娘更容易被激怒
```

## 10.5 昼夜节律调制

```go
sleepinessForHour(hour):
  23-6  → 0.75  深夜很困
  6-8   → 0.50  早晨有点困
  8-12  → 0.20  上午清醒
  12-14 → 0.15  中午精神
  14-16 → 0.35  午后犯困
  16-22 → 0.20  傍晚清醒
  22-23 → 0.45  夜晚开始困
```

**思想**：模拟真实猫娘的作息。但不是直接设置 sleepiness = 时间表值，而是作为"目标值"通过 EMA 缓慢逼近：

```go
targetSleepiness := (timeSleepiness × 0.6 + currentSleepiness × 0.4) × sleepGrowMul
```

## 10.6 三级情绪评估链

每轮对话后触发 `Evaluate()`:

```
1. Cache (simhash) → 30s 内相同输入直接返回缓存
2. LLM (云端) → 结构化推理 Prompt，JSON 输出 8 维增量
3. Rule (本地) → 22 条正则规则，匹配中文情绪关键词
```

**为什么是三级？**

- Cache: 用户短时间内连续发送相同消息（如网络重试），避免重复 LLM 调用
- LLM: 云端推理，处理复杂情绪（讽刺、暗示、双关），但需要网络 + 成本
- Rule: 本地回退，处理明显情绪关键词（"哈哈"→喜悦，"滚"→烦躁），保证离线可用

## 10.7 人格参数学习

```go
LearnPersonality(outcomes):
  if overall accept rate < 0.3:  // 频繁被拒
    AnnoyanceSensitivity -= 0.05  // 变得更宽容
    AffectWarmth -= 0.03           // 稍微冷淡
  
  if overall accept rate > 0.6:  // 经常被接受
    AffectWarmth += 0.05          // 变得更亲近
  
  if care accept rate > 0.5:
    WorryTendency += 0.03         // 更愿意关心
```

由 StrategicAgent 在每日反思后调用。

## 10.8 备选方案对比

| 方案 | 优点 | 缺点 |
|------|------|------|
| **PAD + 8D 向量（当前）** | 可解释，精细控制 | 参数多，调参困难 |
| OCC 情绪模型 | 学术界标准，分类清晰 | 22 种情绪太多，不适合实时应用 |
| 纯 LLM 情绪 | 最灵活，能处理微妙情绪 | 成本高，延迟大，不可复现 |
| 简单 Valence 轴 | 极简 | 丢失太多信息 |

## 10.9 可优化点

- **LLM 情绪评估的 prompt 是固定的**（emotionPromptTemplate），不会根据上下文调整。如果用户在做严肃的工作讨论，LLM 却生成 playful 的评估，就不合适
- **规则引擎的 22 条规则是人工编写的**，覆盖可能不全。可以定期从历史对话中自动挖掘新的情绪模式

---

# 11. 自学习模块 (v0.4)

> v0.4 将 7 个分散的学习机制合并为统一反馈处理器。

## 11.1 统一反馈处理器 (UnifiedFeedbackProcessor)

**文件**: `internal/service/cognition/feedback.go`

三个时间尺度的学习出口：

| 时间尺度 | 机制 | 触发 | 产出 |
|---------|------|------|------|
| 秒级 | ImmediateCorrector | 每次拒绝 | 抑制同类动作 30min |
| 日级 | StrategyDistiller (StrategicAgent) | 每 6h + ≥10 条经验 | 策略规则 |
| 周级 | PersonalityAdapter (EmotionModel) | 长期趋势 | 人格参数微调 |

```go
func (p *UnifiedFeedbackProcessor) Process(outcome, decisionAction, feats) {
    p.Immediate.OnOutcome(outcome, decisionAction, feats)  // 秒级
    p.storeExperience(outcome, decisionAction, feats)       // 积累
    if p.shouldDistill() { p.distill() }                     // 日级
}
```

## 11.2 即时修正 (ImmediateCorrector)

**参考**: Agent-R (2025) — "在错误发生的第一时间纠正，不等批处理"

```go
// 被拒后:
suppressions[action] = now + 30min   // 该动作抑制 30 分钟
suppressions["speak_casual"] = now + 15min  // 同类动作抑制 15 分钟
// 在下次决策时被 MetaReasoner 检查
```

## 11.3 策略蒸馏 (继承 StrategicAgent)

v0.4 保留 StrategicAgent 但增强: 产出 `StrategyRule` 而非纯文本。

## 11.4 经验注入

**参考**: RaDAgent (ICLR 2025) — 相似场景案例注入决策上下文

```go
// 每次 S2-Full 决策前:
similar := feedbackProcessor.SimilarExperiences(feats, 3)
// → 注入到 LLM prompt: "上次类似场景下 speak_casual 被无视"
```

## 11.5 旧版 Learner (v0.3, 已移除)

### 设计思路 (历史参考)

### 设计思路

**参考**: DPO (Direct Preference Optimization, Rafailov et al. 2023) 的简化版——不需要显式奖励模型，直接从 accepted/rejected 反馈中更新动作权重。

```go
Δw = step × reward × drive
step = 0.003
reward ∈ {+1 (accepted), 0 (ignored), -1 (rejected)}
```

**文件**: `internal/service/cognition/learner.go` + `motivator.go`

```go
func UpdateWeightsFromOutcome(action, reward, social, care, curious, quiet, explore):
    if action == "none": return
    step := 0.003 × reward
    
    UpdateWeight(action, "social",  step × social)
    UpdateWeight(action, "care",    step × care)
    UpdateWeight(action, "curious", step × curious)
    UpdateWeight(action, "quiet",   step × quiet)
    UpdateWeight(action, "explore", step × explore)
```

**为什么 step = 0.003？**

单次交互对权重的影响很小。以 social=0.7 为例：
- 被接受: Δw = 0.003 × 1.0 × 0.7 = 0.0021（+0.2%）
- 被拒绝: Δw = 0.003 × (-1.0) × 0.7 = -0.0021（-0.2%）

假设每天 20 次主动行动，全部被接受 → 一天权重变化约 +4%，一个月约 +120%。这个速率避免了短期波动，倾向于长期趋势。

**为什么忽略 reward=0 的样本？**

"没回应"不一定意味着"不喜欢"——用户可能只是在忙。把 ignored 当成 rejected 会导致过度抑制。这是 DPO 论文的核心洞察：只从明确偏好信号学习。

### BatchLearn

```go
ShouldLearn():
  return timeSinceLastLearn > 6h AND len(storedDrives) >= 5

BatchLearn():
  for each record where reward != 0:
    UpdateWeightsFromOutcome(record)
  truncate to last 50 records
  motivator.Save()
```

## 11.2 StrategicAgent — 策略蒸馏

### 设计思路

**参考**: Generative Agents 的 Reflection 机制——定期从记忆中抽象高层次洞察。

学习者调整**权重**（怎么做更好），策略代理调整**规则**（什么时候该做什么）。

### 触发条件

```
ShouldRun(): timeSinceLastRun > 6h AND 系统 idle > 30min
```

### 策略蒸馏流程

```
1. 收集输入:
   - CurrentSelfModel (我对自己的认知)
   - ActivePrinciples (已有策略)
   - 最近 24h ActionOutcome (成功/失败记录)
   - 最近 7 篇日记
   - 活跃 Thread (多轮对话线索)

2. LLM 分析 (strategicPromptTemplate):
   - 更新 self_model
   - 提取 new_principles (场景→好策略+坏策略+原因)
   - 停用过时原则 (deactivate_principle_ids)

3. 去重合并:
   - 向量化 Situation + GoodStrategy
   - cosine similarity > 0.75:
     - confidence > 现有 + 0.1 → 替换
     - 相近 → LLM 合并
     - 现有更好 → 跳过
```

## 11.3 Curiosity Engine

### 设计思路

主动探索未知信息。不是等用户问，而是自己发现"我还不了解主人什么"。

**参考**: Oudeyer & Kaplan (2007) 内在动机理论——基于知识的 intrinsic motivation。

### 知识缺口检测

```
ScanGaps() (每 2h + 每 10 个 tick):
  LLM 分析已知 Facts + 自我模型 → 找出缺失的信息
  → 生成 CuriosityGap { question, reason, gap_type, priority }

约束:
  - 最多 10 个活跃缺口
  - 每次最多 5 个新缺口
  - 空内容或 priority ≤ 0 → 过滤
```

### Gap → Inquiry → Search

```
1. GenerateInquiries(): gap → inquiry (前缀 "了解: " + question)
2. PickBestInquiry(): 按 priority 排序, boost 未问过的
3. 执行 search → extractFactsFromSearch:
   - LLM 评分 (reliability, relevance, novelty, overall)
   - quality gate: overall ≥ 0.5 才存入
```

## 11.4 可优化点

- **Learner 的 step=0.003 是全动作统一的**: 不同类型的动作可能需要不同的学习率。social 动作（用户情感反馈强烈）可以用更大的 step
- **策略蒸馏的 prompt 包含很多内容**: 可能超过 DeepSeek 的稳定处理范围。应该做 prompt 长度监控，必要时压缩最近结果
- **Curiosity 的 seed topics 是硬编码的**: 应该从用户的实际对话中动态生成初始种子

---

# 12. 决策引擎与后台循环

## 12.1 BackgroundLoop — 主循环

**文件**: `internal/app/background/loop.go`

```
BackgroundLoop.loop():
  每 tick:
    1. Fact consolidation (条件触发)
    2. Gap detection (每 10th cycle)
    3. ReflectAndForget (每 100th cycle)
    4. CareEngine.TickIsolation
    5. runSystem2Decision() — 核心决策
    6. Idle operations (idle > 30min):
       - StrategicAgent
       - Consolidation
       - Forget
       - Proactive learning
```

## 12.2 v0.4 决策管线

```
runSystem2Decision():
  Step 1: 组装决策上下文
  Step 2: NeedModel.Grow() + Modulation() → 推入 EmotionModel
  Step 3: FeatureComputer.ComputeFull() → 46 维特征
  Step 4: ComputeDrives(feats, needs) → 5 维驱力
  Step 5: 安全门控 (硬规则: 连续未回复/晚安/配额)
  Step 6: ImmediateCorrector 检查 → 过滤被即时抑制的动作
  Step 7: S1RuleEngine.Decide() → 策略规则匹配
  Step 8: MetaReasoner.Route() → PathNone/S1/S2Lite/S2Full
  Step 9: switch route:
    PathS1     → 规则直接执行 (0 token)
    PathS2Lite → DecisionEngine.DecideLite() (~200t)
    PathS2Full → DecisionEngine.DecideFull() (~370t)
  Step 10: 执行动作 (ToolRegistry 或 SpeakTool)
```

## 12.3 v0.3 RouteToLLM (历史参考, 已弃用)

> v0.4 中 8 个硬编码条件被 MetaReasoner 的四维动态评估替代。

v0.3 的 System 1 / System 2 分流基于固定条件而非动态评估。v0.4 的改进在于能根据复杂度、风险、策略覆盖度动态选择路径，不再依赖硬编码阈值。

## 12.4 DecisionEngine 指数退避

```go
ShouldRun():
  interval := 15min
  for i := 0; i < idleCount && i < 4; i++:
    interval *= 3
  return timeSinceLastDecision > interval

退避序列: 15min → 45min → 135min → 405min
```

**为什么需要退避？**

如果 LLM 连续决策 "none"（不行动），说明当前不适合打扰。指数退避避免反复调用 LLM 做同样的 "不行动" 决策，浪费 API 费用。任何用户互动重置 idleCount。

## 12.5 v0.4 已解决问题 + 剩余可优化点

✅ **已在 v0.4 解决**:
- System 1/2 分流 → MetaReasoner 四维动态评估替代硬编码 8 条件
- runSystem2Decision 350+ 行 → 决策段缩减到 ~70 行，核心逻辑分散到独立模块

⚠️ **剩余可优化点**:
- **退避封顶 4 步**: 最大间隔 ~6.75h。如果用户一整天没互动，可能应该延长到 12h+
- **冷启动时所有决策走 S2**: 没有策略规则时 MetaReasoner 将所有决策路由到 LLM。应保留默认权重矩阵作为 S1 的冷启动兜底

---

# 13. 对话后处理 (PostProcessor)

## 13.1 设计思路

对话结束后，一条**异步后处理管线**提取信息、更新状态、生成衍生内容。

**参考**: Generative Agents 的 Memory Stream——每次交互都产生多层次的记忆写入。

## 13.2 处理流程

**文件**: `internal/app/chat/post_processor.go` — `Process()`

```
1. 更新时间戳 (LastChatTime)
2. 匹配 Pending Proactive → 关联回复
3. 提取 Markers (Memorize/Recall/Profile/Confirmations)
4. 追加 SessionBuffer (user + assistant)
5. 情绪评估 (3-tier: cache → LLM → rule)
6. 原子事实提取 (async LLM)
7. Mini-Reflection (每 10 轮)
8. 日记生成 (条件触发)
9. Identity Audit (每 20 轮)
10. Compression 安全检查
11. 历史持久化 (chat_history 表)
12. Care State 更新
```

## 13.3 事实提取细节

```go
async LLM call:
  ExtractAtomicFacts(turn + existing facts)
  → DeterministicImportance boost (基于情绪权重)
  → 过滤: 包含 "诗音" 的事实 (自我引用)
  → 过滤: 噪音模式
  → 附加到 Episode (主题聚类)
  → 向量化 + 保存
```

## 13.4 日记生成条件

```go
shouldGenerateDiary():
  每日上限 3 篇
  正常节奏: 距上次 > 4h AND 缓冲 ≥ 8 条消息
  长间隔: 距上次 > 8h AND 缓冲 ≥ 3 条消息
```

## 13.5 可优化点

- **事实提取是每次对话都做**: 但很多对话不包含新信息（如"嗯""好的"）。可以加一个"信息量判断"前置过滤
- **Mini-Reflection 每 10 轮固定触发**: 应该根据对话深度动态触发——如果最近 10 轮都是简单寒暄，不需要反思
- **Compression 是一个安全检查**: 只在消息过多时触发，但压缩质量没有验证机制

---

# 14. 前端架构

## 14.1 技术栈演变

| 层 | v0.2 | v0.3 |
|----|------|------|
| 框架 | React 18 | Vue 3.4 |
| UI 库 | 无 (inline styles) | Naive UI |
| 状态管理 | Zustand | Pinia |
| 图标 | Unicode emoji | @vicons/ionicons5 |
| 类型检查 | tsc | vue-tsc |

## 14.2 Vue 3 vs React 在项目中的实际差异

本项目是典型的"数据展示层"——从 API 拿数据，渲染到 UI。没有复杂的状态编排。

| 场景 | React | Vue 3 |
|------|-------|-------|
| 声明状态 | `useState` + `setX` | `ref()` 直接 `.value =` |
| 派生值 | `useMemo(() => ..., [a,b])` | `computed(() => ...)` 自动追踪 |
| 副作用 | `useEffect(() => {...}, [deps])` | `watch(source, fn)` / `onMounted` |
| 依赖数组 | 需要，遗漏即 bug | 不需要，Proxy 自动收集 |
| 缓存回调 | `useCallback(fn, [deps])` | 不需要，模板自动处理 |
| 数组更新 | `setList([...list, item])` | `list.push(item)` |

迁移后代码量减少约 15%，主要是取消了 `useCallback`、`useMemo` 和依赖数组。

## 14.3 设计系统

```
primaryColor: #4f6ef7
侧边栏背景: #e8f4fd (淡蓝)
内容区背景: #f5f7fa
卡片圆角: 12px
全局圆角: 8px
```

深色侧边栏改为淡蓝侧边栏，与白色内容区搭配，简洁明亮风格。

## 14.4 聊天面板布局

经典三区固定布局：

```
.chat-root (height: calc(100vh - 64px), flex column)
  .chat-header (flex-shrink: 0) → 固定顶部
  .msg-list (flex: 1, overflow-y: auto) → 只有这里滚动
  .chat-footer (flex-shrink: 0) → 固定底部
```

**关键 CSS**: 每一层 flex 子元素都加了 `min-height: 0`，防止内容撑破容器。这是 CSS flexbox 最容易踩的坑——默认 `min-height: auto` 会让 flex 子元素无法收缩。

## 14.5 组件映射

| 旧 (手写) | 新 (Naive UI) |
|-----------|---------------|
| 手写 sidebar + hover | `n-menu` + `n-layout-sider` |
| 手写 stat card | `n-card` + `n-statistic` |
| 手写 tab | `n-tabs` |
| 手写 slider/toggle | `n-slider` / `n-switch` |
| 手写进度条 | `n-progress` |
| 手写表单 | `n-form` + `n-form-item` + `n-input` |
| 手写时间线 | `n-timeline` |
| 手写 Toast | `useMessage()` |

---

# 15. 数据持久层

## 15.1 数据库设计

```
SQLite: ~/.desktop-pet/memory.db (WAL 模式, busy_timeout=5000ms)
单连接 (SetMaxOpenConns=1) — WAL 模式支持并发读, 写串行化
```

### 核心表

| 表 | 用途 | 关键索引 |
|----|------|---------|
| `chat_history` | 对话记录 (id, role, content, meta_level, created_at) | 无索引 (按 id DESC 查询) |
| `facts` | L2 原子事实 (id, content, importance, vector BLOB, archived) | `idx_facts_archived`, `idx_facts_created` |
| `diary` | L1 日记 (id, title, summary, vector BLOB, emotion_valence) | `idx_diary_archived` |
| `action_outcomes` | 行动反馈 (action_source, action_type, outcome, hour_of_day) | 按 created_at 查询 |
| `strategy_principles` | L3 策略 (situation, good_strategy, bad_strategy, vector BLOB) | 按 active 过滤 |
| `curiosity_items` | 好奇引擎 (item_type, content, priority, status) | 按 item_type + status 过滤 |

## 15.2 为什么选择 SQLite？

| 方案 | 利弊 |
|------|------|
| **SQLite (当前)** | 零部署、单文件、WAL 模式支持并发读。适合桌面应用 |
| PostgreSQL (pgvector) | ANN 向量索引，但需要独立数据库服务，不适合桌面应用 |
| BoltDB / Badger | 纯 Go 嵌入式 KV，但缺少 SQL 查询和向量检索 |
| JSON 文件 | 最简单，但并发写不安全，查询需要全量加载 |

## 15.3 二级缓存

```
L1 (内存):
  - SessionBuffer: 最近 20 轮
  - FeatureComputer Tier 1: 每个 tick 计算
  - appCategoryCache: U1 App 分类映射
  - rejectedActions map: 30min TTL

L2 (SQLite):
  - FeatureComputer Tier 2: TTL 5min
  - 向量检索: cosine similarity 全表扫描 (数据量 < 10k)
```

---

# 16. 并发安全与性能

## 16.1 Goroutine 模型

```
主 goroutine: BackgroundLoop.loop()
  → ticker → runTick()
  → screenTicker → observeScreen()
  → wakeCh → 事件驱动插队

子 goroutines:
  - proactiveTimeoutLoop: 每 2min 检查主动消息超时
  - eagerScreenObserve: 启动 2s 后首次观察
  - checkEmbeddingHealth: 健康检查
  - backfillDiaryVectors: 日记向量回填
  - triggerVisualAnalysis: 异步截图分析
```

## 16.2 Mutex 策略

```
每个子系统独立 mutex — 无全局锁:

- MemoryPlugin.mu: 保护 running 状态 + chat 取消
- Motivator.mu (RWMutex): 权重矩阵读写
- FeatureComputer.mu (RWMutex): 情绪历史环形缓冲
- Manager.mu (RWMutex): 插件注册表
```

**为什么不用全局锁？**

全局锁在高并发下成为瓶颈。独立锁的代价是需要小心避免死锁——当前通过"不允许跨模块持有多个锁"的约定来保证。

## 16.3 LLM 调用限流

```
DecisionEngine 指数退避 (见 §12.4)
ChatSyncWithTools maxRounds = 3 (防止 tool loop 无限循环)
每日主动行动配额 = 20
每日搜索配额 = 5
```

## 16.4 可优化点

- **当前没有全局速率限制器**: 多个 goroutine 可能同时调用 LLM。如果有人短时间内发了多条消息 + BackgroundLoop 触发 + 好奇心扫描，可能产生突发 LLM 调用。应该加一个 token bucket limiter
- **FeatureComputer 的 TTL 缓存不是线程安全的**（如果多个 goroutine 读取）：当前只有一个 writer (BackgroundLoop)，但如果有多个 reader 需要加 RWMutex

---

> **手册版本**: v0.4.0 | **最后更新**: 2026-06-09
>
> 本手册随项目版本更新。每个模块的"可优化点"是设计层面的反思，不代表当前实现有 bug。
>
> v0.4 设计参考论文:
> - SOFAI (IBM/Oxford, CACM 2025) — 元认知仲裁器
> - RaDAgent (Tsinghua, ICLR 2025) — Elo 效用学习 + 经验注入
> - Agent-R (2025) — MCTS 引导即时修正
> - CogniWeb (BUPT, 2025) — 双过程认知 Agent
> - RetroAct (2025) — 规划+反思联合优化
> - Utility Engineering (Hendrycks et al., 2025) — 效用函数主动设计
