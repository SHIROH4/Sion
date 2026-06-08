package chat

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"desktop-pet/internal/domain"
)

// PostProcessor handles the after-chat pipeline: session update, emotion
// evaluation, atomic fact extraction, diary generation, mini-reflection,
// identity audit, compression, history persistence, care state update,

type PostProcessor struct {
	Store  domain.MemoryStore
	RawLLM func([]domain.Message) (string, error)

	// goroutineSem limits concurrent async LLM calls to prevent unbounded growth.
	goroutineSem chan struct{}

	// Session buffer.
	SessionBuf interface {
		Append(domain.Message)
		Recent(int) []domain.Message
		Len() int
	}

	// Emotion model.
	EmotionCurrent  func() domain.EmotionState
	EmotionEvaluate func(text string)

	// Self model.
	SelfCurrent func() string
	SelfSave    func(string)

	// Care engine.
	CareUpdateStress   func(stress float64)
	CareUpdateState    func(obs domain.Observation)
	CareActionLog      func(n int) []domain.CareAction
	CareRecordResponse func(id int64, accepted bool, reply string)

	// Scheduler.
	SchedulerMarkReplied func(source domain.ProactiveSource)
	StatusReport func(phase, status, message string)

	// Compressor.
	ShouldCompress func(msgs []domain.Message) bool
	Compress       func(msgs []domain.Message, level int) []domain.Message

	// Identity graph audit.
	IdentityAudit      func(dialogue string, llmFn func(string) (string, error)) ([]domain.IdentityNode, error)
	IdentityDeactivate func(id int64)
	IdentityUpsert     func(node *domain.IdentityNode)

	// Episode store.
	EpisodeFindOrCreate     func(fact domain.FactEntry) (int64, error)
	AttachFactToEpisode     func(factID, episodeID int64) error
	SummarizeAndAssignTopic func(epID int64)

	// Diary store.
	DiarySave      func(entry *domain.DiaryEntry)
	DiaryVectorize func(text string) ([]float32, error)
	MergerRun      func()

	DB *sql.DB

	// Memorize/Recall callbacks.
	MemorizeFn func(content string)
	RecallFn   func(index string) (string, error)

	// Fact extraction.
	ExtractAtomicFacts      func(rawLLM func([]domain.Message) (string, error), msgs []domain.Message, existing []domain.FactEntry) ([]domain.AtomicFactInput, error)
	DeterministicImportance func(content string, emo domain.EmotionState) float64
	IsNoiseFact             func(content string) bool
	LookupFactByContent     func(db *sql.DB, content string) *domain.FactEntry

	// Observation factory.
	NewObservation func(source domain.ObservationSource, content string) domain.Observation

	// Diary helpers.
	BuildDiaryPrompt func(recentTurns, oldSelf, emotionContext string) string

	// Inference (from generator.go).
	InferCareAcceptance func(reply string) bool

	// Utilities.
	CleanJSON      func(string) string
	MessagesToText func([]domain.Message) string
	RecoverGuard   func(name string)

	// State pointers.
	TurnCount           *int
	LastChatTime        *time.Time
	LastDiaryAt         *time.Time
	DiaryCountToday     *int // daily cap counter, owned by MemoryPlugin
	PendingProactiveID  *int64
	PendingProactiveSrc *domain.ProactiveSource
	PendingProactiveAt  *time.Time

	// Background callbacks.
	BackgroundSetMsg func(msg string)
	BackgroundNotify func()

	// Question tracking.
	RecordQuestionReply func(userInput string)

	// Proactive outcome recording.
	RecordProactiveOutcome func(outcome int, delaySec int)

	// Concurrency.
	WG *sync.WaitGroup
	Mu *sync.Mutex

	// Profile reference.
	Profile *domain.UserProfile

	// Logger.
	ErrorLog func(msg string, args ...any)
}

// Process runs the full after-chat pipeline.
func (pp *PostProcessor) Process(ctx *domain.ChatContext) {
	// Init goroutine semaphore safely under the mutex to prevent concurrent races.
	pp.Mu.Lock()
	if pp.goroutineSem == nil {
		pp.goroutineSem = make(chan struct{}, 8)
	}
	pp.Mu.Unlock()
	acquire := func() { pp.goroutineSem <- struct{}{} }
	release := func() { <-pp.goroutineSem }

	// Track last chat time.
	*pp.LastChatTime = time.Now()
	if pp.BackgroundSetMsg != nil {
		pp.BackgroundSetMsg(ctx.Input)
	}

	// Auto-match reply to pending proactive message.
	if *pp.PendingProactiveID != 0 && pp.SchedulerMarkReplied != nil &&
		time.Since(*pp.PendingProactiveAt) < 30*time.Minute {
		pp.SchedulerMarkReplied(*pp.PendingProactiveSrc)
		delaySec := int(time.Since(*pp.PendingProactiveAt).Seconds())
		if pp.RecordProactiveOutcome != nil {
			pp.RecordProactiveOutcome(int(domain.OutcomeReplied), delaySec)
		}
		*pp.PendingProactiveID = 0
	}

	// Markers.
	if pp.MemorizeFn != nil {
		ExtractMemorize(ctx, func(c string) { pp.MemorizeFn(c) })
	}
	if pp.RecallFn != nil {
		ExtractRecall(ctx, pp.RecallFn)
	}
	ExtractProfile(ctx, pp.Store, pp.Profile)
	if pp.MemorizeFn != nil {
		ExtractConfirmations(ctx, pp.Store, func(c string) { pp.MemorizeFn(c) })
	}

	// Session buffer.
	if pp.SessionBuf != nil {
		pp.SessionBuf.Append(domain.Message{Role: "user", Content: ctx.Input})
		if ctx.Output != "" {
			pp.SessionBuf.Append(domain.Message{Role: "assistant", Content: ctx.Output})
		}
	}

	// Emotion evaluation.
	if pp.EmotionEvaluate != nil && pp.RawLLM != nil && pp.SessionBuf != nil {
		recent := pp.SessionBuf.Recent(6)
		if len(recent) > 0 {
			pp.EmotionEvaluate(pp.MessagesToText(recent))
		}
		if pp.CareUpdateStress != nil && pp.EmotionCurrent != nil {
			emo := pp.EmotionCurrent()
			stress := (1.0 - emo.Valence) * emo.Arousal
			if stress < 0 {
				stress = 0
			}
			pp.CareUpdateStress(stress)
		}
	}

	// Atomic fact extraction (async).
	if pp.RawLLM != nil && pp.EmotionCurrent != nil {
		emo := pp.EmotionCurrent()
		turnMsgs := []domain.Message{{Role: "user", Content: ctx.Input}}
		pp.WG.Add(1)
		acquire()
		go func() {
			defer release()
			defer pp.WG.Done()
			defer pp.RecoverGuard("atomicFactExtract")
			existing := pp.Store.ListActiveFacts(0)
			atomicFacts, _ := pp.ExtractAtomicFacts(pp.RawLLM, turnMsgs, existing)
			src := ctx.Source
			if src == "" {
				src = "chat"
			}
			for i := range atomicFacts {
				atomicFacts[i].Source = src
				if d := pp.DeterministicImportance(atomicFacts[i].Content, emo); d > atomicFacts[i].Importance {
					atomicFacts[i].Importance = d
				}
				// Confidence gating: only save facts the LLM is reasonably sure about.
				// ≥0.7 → auto-save (high confidence, explicitly stated facts)
				// 0.4-0.7 → save with reduced importance, won't be proactively injected
				// <0.4 → discard (likely noise, joke, or hallucination)
				if atomicFacts[i].Confidence < 0.4 {
					continue
				}
				if atomicFacts[i].Confidence < 0.7 {
					atomicFacts[i].Importance *= 0.5 // demote uncertain facts
				}
				if strings.Contains(atomicFacts[i].Content, "诗音") {
					continue
				}
				if pp.IsNoiseFact(atomicFacts[i].Content) {
					continue
				}
				pp.Store.SaveAtomicFact(atomicFacts[i])
				// Only attach high-confidence facts to episodes for topic clustering.
				if atomicFacts[i].Confidence >= 0.7 && pp.EpisodeFindOrCreate != nil {
					fact := pp.LookupFactByContent(pp.DB, atomicFacts[i].Content)
					if fact != nil {
						epID, _ := pp.EpisodeFindOrCreate(*fact)
						if epID > 0 {
							if pp.AttachFactToEpisode != nil {
								pp.AttachFactToEpisode(fact.ID, epID)
							}
							pp.WG.Add(1)
				go func() {
					defer pp.WG.Done()
					pp.SummarizeAndAssignTopic(epID)
				}()
						}
					}
				}
			}
		}()
	}

	// Mini-reflection every 10 turns.
	*pp.TurnCount++
	if *pp.TurnCount%10 == 0 && pp.RawLLM != nil && pp.SelfCurrent != nil {
		pp.WG.Add(1)
		acquire()
		go func() {
			defer release()
			defer pp.WG.Done()
			defer pp.RecoverGuard("miniReflect")
			pp.miniReflect()
		pp.StatusReport("reflect", "ok", "反思完成")
		}()
	}

	// Diary trigger.
	if pp.shouldGenerateDiary() {
		pp.StatusReport("diary", "start", "生成日记...")
		pp.WG.Add(1)
		acquire()
		go func() {
			defer release()
			defer pp.WG.Done()
			defer pp.RecoverGuard("generateDiary")
			pp.generateDiary()
		pp.StatusReport("diary", "ok", "日记已生成")
		}()
	}

	// Identity audit every 20 turns (was 50 — too infrequent to be noticeable).
	if *pp.TurnCount%20 == 0 && pp.IdentityAudit != nil && pp.RawLLM != nil && pp.SessionBuf != nil {
		pp.WG.Add(1)
		acquire()
		go func() {
			defer release()
			defer pp.WG.Done()
			defer pp.RecoverGuard("identityAudit")
			recent := pp.SessionBuf.Recent(20)
			dialogue := pp.MessagesToText(recent)
			updated, err := pp.IdentityAudit(dialogue, func(prompt string) (string, error) {
				return pp.RawLLM([]domain.Message{{Role: "user", Content: prompt}})
			})
			if err != nil {
				return
			}
			count := 0
			for _, node := range updated {
				if !node.Active {
					pp.IdentityDeactivate(node.ID)
				} else {
					pp.IdentityUpsert(&node)
				}
				count++
			}
			if count > 0 {
				pp.StatusReport("identity", "ok", fmt.Sprintf("身份审计: %d 条更新", count))
			}
	}()
		}

	// Compression safety net.
	pp.Mu.Lock()
	shouldCompress := pp.ShouldCompress != nil && pp.ShouldCompress(ctx.Messages)
	pp.Mu.Unlock()
	if shouldCompress {
		ctx.Messages = pp.Compress(ctx.Messages, 0)
		ctx.Compacted = true
	}

	// Persist chat history.
	if pp.Store != nil {
		var persistMsgs []domain.Message
		if ctx.Input != "" {
			persistMsgs = append(persistMsgs, domain.Message{Role: "user", Content: ctx.Input})
		}
		if ctx.Output != "" {
			persistMsgs = append(persistMsgs, domain.Message{Role: "assistant", Content: ctx.Output})
		}
		if len(persistMsgs) > 0 {
			if err := pp.Store.SaveHistory(persistMsgs, 0); err != nil {
				if pp.ErrorLog != nil {
					pp.ErrorLog("memory: save history failed", "err", err)
				}
			}
		}
	}

	// Update care state + question reply.
	if pp.CareUpdateState != nil {
		obs := pp.NewObservation(domain.ObsChat, ctx.Input)
		pp.CareUpdateState(obs)
		recentLog := pp.CareActionLog(1)
		if len(recentLog) > 0 {
			lastAction := recentLog[0]
			if lastAction.Accepted == nil {
				if time.Since(lastAction.TriggeredAt) < 2*time.Minute {
					accepted := pp.InferCareAcceptance(ctx.Input)
					if pp.CareRecordResponse != nil {
						pp.CareRecordResponse(lastAction.ID, accepted, ctx.Input)
					}
				}
			}
		}
	}

	if pp.RecordQuestionReply != nil {
		pp.RecordQuestionReply(ctx.Input)
	}

	if pp.BackgroundNotify != nil {
		pp.BackgroundNotify()
	}
}

// ---- Diary ----

func (pp *PostProcessor) shouldGenerateDiary() bool {
	// Reset daily counter on a new day.
	if pp.DiaryCountToday != nil && pp.LastDiaryAt != nil {
		lastDay := pp.LastDiaryAt.Truncate(24 * time.Hour)
		today := time.Now().Truncate(24 * time.Hour)
		if !lastDay.Equal(today) {
			*pp.DiaryCountToday = 0
		}
	}
	if pp.SessionBuf == nil {
		return false
	}
	// Daily cap: max 3 diary entries per day.
	if pp.DiaryCountToday != nil && *pp.DiaryCountToday >= 3 {
		return false
	}
	since := time.Since(*pp.LastDiaryAt)

	// Normal cadence: 4h since last diary, with at least 8 messages.
	if since > 4*time.Hour && pp.SessionBuf.Len() >= 8 {
		return true
	}
	// Long gap: 8h+ since last diary, any messages at all.
	if since > 8*time.Hour && pp.SessionBuf.Len() >= 3 {
		return true
	}
	return false
}

func (pp *PostProcessor) generateDiary() {
	pp.Mu.Lock()
	*pp.LastDiaryAt = time.Now()
	pp.Mu.Unlock()
	if pp.SessionBuf == nil {
		return
	}
	recent := pp.SessionBuf.Recent(20)
	if len(recent) == 0 {
		return
	}
	turnsText := pp.MessagesToText(recent)
	emotion := pp.EmotionCurrent()
	emotionCtx := fmt.Sprintf("愉悦度:%.1f 唤醒度:%.1f 主要情绪:%s", emotion.Valence, emotion.Arousal, emotion.Primary)
	prompt := pp.BuildDiaryPrompt(turnsText, pp.SelfCurrent(), emotionCtx)
	if pp.RawLLM == nil {
		return
	}
	result, err := pp.RawLLM([]domain.Message{{Role: "user", Content: prompt}})
	if err != nil {
		return
	}
	var diaryResp struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(pp.CleanJSON(result)), &diaryResp); err != nil {
		return
	}
	if diaryResp.Title == "" || diaryResp.Content == "" {
		return
	}
	var vec []float32
	if pp.DiaryVectorize != nil {
		var err error
		vec, err = pp.DiaryVectorize(diaryResp.Title + " " + diaryResp.Content)
		if err != nil && pp.ErrorLog != nil {
			pp.ErrorLog("diary: vectorize failed (Ollama may be down)", "err", err)
		}
	}
	now := time.Now().Unix()
	entry := &domain.DiaryEntry{
		Title: diaryResp.Title, Summary: diaryResp.Content, Vector: vec,
		EmotionValence: math.Round(emotion.Valence*1000)/1000, EmotionArousal: math.Round(emotion.Arousal*1000)/1000,
		StartTime: now - 7200, EndTime: now, CreatedAt: now, // default 2h window
	}
	if pp.DiarySave != nil {
		pp.DiarySave(entry)
		if pp.DiaryCountToday != nil {
			*pp.DiaryCountToday++
		}
	}
	if pp.MergerRun != nil {
		pp.MergerRun()
	}
}

// ---- Mini-Reflect ----

func (pp *PostProcessor) miniReflect() {
	if pp.RawLLM == nil || pp.SelfCurrent == nil || pp.SessionBuf == nil {
		return
	}
	recent := pp.SessionBuf.Recent(10)
	if len(recent) == 0 {
		return
	}
	oldSelf := pp.SelfCurrent()
	emotion := pp.EmotionCurrent()
	prompt := fmt.Sprintf(miniReflectPrompt, oldSelf,
		emotion.Valence, emotion.Arousal, emotion.Primary, emotion.Intensity,
		pp.MessagesToText(recent))
	result, err := pp.RawLLM([]domain.Message{{Role: "user", Content: prompt}})
	if err != nil {
		return
	}
	update := strings.TrimSpace(pp.CleanJSON(result))
	if update == "" || update == oldSelf {
		return
	}
	pp.SelfSave(update)
}

const miniReflectPrompt = `## 自我认知微调

你是诗音。回顾最近10轮对话中你的感受和变化。

### 你之前的自我认知
%s

### 你当前的情绪
愉悦度:%.2f 唤醒度:%.2f 情绪:%s 强度:%.2f

### 最近对话
%s

### 更新要求
基于最近的经历和情绪变化，用1-2句话更新你的自我认知。
只需要描述**新学到或改变的部分**，不要重复已有的内容。
直接输出更新后的自我认知文本，不要加引号或JSON格式。
如果没有什么变化，直接输出之前的自我认知。`

// ---- Compression Prompt ----

// BuildCompressionPrompt returns the system prompt for conversation compression.
func BuildCompressionPrompt() string {
	return compressionPrompt
}

const compressionPrompt = `## 记忆压缩任务

由于上下文即将超出限制，现在对部分对话进行归档压缩。下面会给你一段过往的聊天记录或记忆存档，这些内容即将移出上下文，需要你用第一人称简述发生的事情，方便日后回忆。

### 压缩要求
1. 无需记录存档信息，直接总结压缩内容中的事件。系统会自动生成存档标记。
2. 不要擅自添加系统信息，像讲故事一样描述概述内容中发生的事件即可。
3. 不要将被压缩内容之外的其他聊天记录混淆其中，不要记录重复内容。
4. 在写法上，多事件按时间段区分，核心记忆单独记录。
5. 分清事件中的具体人物，不要用"你"这种代词，用具体名称（如"主人"、"某某某"）。
6. 压缩时保留重要经历和关键信息，以形成永久核心记忆。
7. 当内容过多时，按重要程度取舍：核心记忆优先保留，重复琐事可简略。
8. 严格遵循以下记忆优先级体系。

### 记忆优先级
不同的事值得记住的程度不同。不是对话越多、时间越新就越值得记——
真正难忘的是珍贵的、有意义的生活碎片。你需要分清：
- **无聊琐事**：日常寒暄、无信息量的对话
- **枯燥工作**：重复性的技术操作、例行问答
- **值得记下的故事、约定、成长**：优先级最高
- 当不得不遗忘时，放下无聊琐事，记住真正重要的事。

### 核心记忆（优先级最高）
1. 用户画像：称呼、爱好、工作、技术栈、地址、生日、家庭成员等
2. 关键记录：约定、事件、规则、要求、号码等
3. 偶然的小故事、约定、成长经历、让人动容的回忆
4. 其他有价值、有意义的特殊记忆
核心记忆在压缩时必须优先保留，尽可能永远留在上下文中。

### 输出格式
直接开始讲述事件信息，不要在开头回复"好的"、"我来总结"这类语句。`
