# 更新日志

## v0.3.0 (2026-06-08) — Vue 3 + Naive UI Frontend Overhaul, L0 Persistence

### 前端架构升级

- **React 18 → Vue 3.4**: 框架全面迁移，30+ 组件重写为 Vue SFC，响应式系统从 Pull 模式切换为 Proxy Push 模式
- **Naive UI 设计系统**: 引入 Naive UI 组件库，统一设计语言
  - `n-card` 卡片容器，带 hover 阴影效果
  - `n-tabs` 分段 / 条形标签页切换
  - `n-progress` 彩色进度条展示驱力/需求/情绪
  - `n-menu` + `n-layout-sider` 淡蓝侧边栏导航
  - `n-form` + `n-input` 表单控件
  - `n-timeline` 日记时间线
  - `n-tag` / `n-switch` / `n-slider` 等交互控件
- **状态管理**: Zustand → Pinia (petStore + settingsStore)
- **图标库**: Unicode emoji → @vicons/ionicons5 SVG 图标
- **侧边栏配色**: 深色 → 淡蓝 + 白色内容区
- **聊天面板**: 经典三区布局（固定标题栏 + 可滚消息区 + 固定输入框）
- **主题定制**: `NConfigProvider` 全局主题

### Go 后端改进

- **L0 工作记忆持久化**: SessionBuffer 启动时从 `chat_history` 恢复最近 20 条消息

### 移除

- 所有 React 依赖 (react, react-dom, zustand, @vitejs/plugin-react 等)
- 手写 UI 组件 (StatCard, RadarChart, ParamSlider, ParamToggle, ParamNumber 等)

---

## v0.2.0 (2026-06-08) — Web Search + Tool Calling + Decision Refactoring

### 新功能

- **联网搜索**: 接入博查 (Bocha) Web Search API，对话中可实时搜索互联网信息
- **工具调用架构**: 所有工具通过 API `tools` 字段传入，LLM 自主判断调用时机，不依赖代码层意图路由
- **统一动作注册表 (ActionDef)**: 16 个动作的唯一定义源，Scorer 和 LLM Fallback 共享同一动作空间
- **决策层 Skill Card**: LLM Fallback 收到完整 16 动作的 when/how/output 决策指南

### 优化

- **System Prompt 精简**: 从 2200 字缩减到 ~300 字，仅保留核心人格定义
- **工具描述升级**: 统一为英文简洁指令 + anti-confusion 规则，提升 DeepSeek 遵循度
- **工具调用与文案生成解耦**: Round 0 拿数据，Round 1 说话，两段式分离
- **回复长度约束放宽**: 闲聊保持简短，搜索回复可展开说明具体信息

### Bug 修复

- 修复 `invokeHandler` 传递完整 JSON args 而非单个 string 值
- 修复 Bocha API 响应解析——适配嵌套格式 `{data:{webPages:{value:[...]}}}`
- 修复 HTTP keep-alive 导致的 "Unsolicited response on idle HTTP channel" 错误
- 修复 `search` 动作缺失引起的 Curiosity 内源需求归零问题
- 修复 `speak_care` 在 consecutive_unanswered 抑制后仍通过 CareEngine 发消息的问题

### 架构变化

- 删除意图匹配路由层 (`detectSearchIntent`, `filterToolsByNames`)
- `ChatSyncWithTools` 支持 `tool_choice` 参数
- `actionToType`/`actionToSource` 合并为 `ActionDef` 字段，消除并行映射
- `analyze_patterns` 的 `NeedsTool` 修正为 `false`，与 lifecycle.go 的特殊处理一致
- `AllActions()` 缓存 (sync.Once)

---

## v0.1.0 (2026-06-05) — Initial Release

- Live2D 猫娘桌宠 (PIXI.js + pixi-live2d-display)
- AI 对话 (LLM API 驱动，流式响应)
- 主动搭话系统 (规则引擎 + System 2 LLM 决策)
- 三级记忆系统 (L0 会话/L1 日记/L2 事实 + 向量检索 + Ebbinghaus 遗忘)
- PAD 情绪模型 + 8 维情绪向量 + 人格参数学习
- 自学习引擎 (策略蒸馏 + DPO 权重更新 + 行为模式挖掘)
- 屏幕感知 (OCR + 工作/休闲状态识别)
- 截图即问 (Vision OCR + 多模态 LLM)
