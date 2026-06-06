package memory

import (
	"strings"

	"desktop-pet/internal/domain"
)

// PredefinedTopics returns the built-in topic categories.
var PredefinedTopics = []string{
	"主人-身份信息",
	"主人-技术工作",
	"主人-游戏偏好",
	"主人-生活习惯",
	"主人-社交关系",
	"诗音-自我成长",
	"系统-技术问题",
}

var topicSeedText = map[string]string{
	"主人-身份信息": "主人的个人信息，包括姓名、生日、年龄、职业、所在地、联系方式等基本身份资料",
	"主人-技术工作": "主人是程序员做后端开发，使用Go语言、agent技术、写代码、修bug、升级系统、开发新功能、项目迭代、技术学习",
	"主人-游戏偏好": "主人玩王者荣耀打排位上分渡劫、瓦罗兰特、APEX等MOBA和FPS游戏，喜欢打游戏放松娱乐",
	"主人-生活习惯": "主人吃饭喝水作息睡眠，午饭晚饭吃了什么喝了什么，熬夜加班到凌晨，需要休息提醒，工作太忙忘记按时吃饭",
	"主人-社交关系": "主人的朋友同事社交圈子，QQ微信聊天群组，和别人的互动交流",
	"诗音-自我成长": "诗音作为猫娘AI的自我认知成长变化，学到新能力，理解主人更深，记忆系统升级进化",
	"系统-技术问题": "系统遇到的技术问题和解决方案，记忆系统bug修复、功能改进、配置调整、性能优化等开发维护工作",
}

var topicKeywords = map[string][]string{
	"主人-身份信息": {"名字", "姓名", "称呼", "生日", "年龄", "职业", "所在地", "地址", "学生", "程序员", "工程师"},
	"主人-技术工作": {"Go", "agent", "代码", "写", "开发", "编程", "bug", "修", "项目", "后端", "API", "升级", "Rust", "技术", "学习", "逻辑"},
	"主人-游戏偏好": {"王者", "荣耀", "瓦罗兰特", "APEX", "游戏", "排位", "上分", "渡劫", "段位", "MOBA", "FPS", "打", "玩"},
	"主人-生活习惯": {"吃", "喝", "饭", "咖啡", "水", "睡", "休息", "熬夜", "凌晨", "作息", "提醒", "注意", "放松", "午", "晚", "煲仔"},
	"主人-社交关系": {"朋友", "同事", "社交", "QQ", "微信", "群", "聊", "别人", "同学"},
	"诗音-自我成长": {"诗音", "猫娘", "记忆系统", "学会", "能", "能力", "认识", "理解", "升级", "进化", "自我", "成长", "变化"},
	"系统-技术问题": {"bug", "修复", "系统", "配置", "优化", "性能", "维护", "升级", "功能", "改进", "测试", "崩溃", "问题"},
}

// TopicRepo is the minimal interface TopicService needs from the infra layer.
type TopicRepo interface {
	domain.TopicRepository
	Initialize(topics []string) error
	UpdateCentroid(topicID int64) error
	FindTopicIDByName(name string) int64
	SetCentroidRaw(topicID int64, vec []float32)
}

// TopicService manages topic lifecycle, centroid seeding, and backfill.
type TopicService struct {
	TopicRepo // embedded — promotes ListTopics, FindBestTopic, AssignEpisodeToTopic, UpdateCentroid, etc.
	vectorize func(string) ([]float32, error)
}

// NewTopicService creates a TopicService.
func NewTopicService(repo TopicRepo) *TopicService {
	return &TopicService{TopicRepo: repo}
}

// SetVectorize injects the embedding function.
func (s *TopicService) SetVectorize(fn func(string) ([]float32, error)) {
	s.vectorize = fn
}

// Initialize creates predefined topics if they don't exist.
func (s *TopicService) Initialize() error {
	return s.TopicRepo.Initialize(PredefinedTopics)
}

// SeedCentroids computes initial centroids for topics that don't have one yet.
func (s *TopicService) SeedCentroids() {
	if s.vectorize == nil {
		return
	}
	for _, name := range PredefinedTopics {
		id := s.TopicRepo.FindTopicIDByName(name)
		if id == 0 {
			continue
		}
		text := name
		if desc, ok := topicSeedText[name]; ok {
			text = text + " " + desc
		}
		vec, err := s.vectorize(text)
		if err != nil || len(vec) == 0 {
			continue
		}
		s.TopicRepo.SetCentroidRaw(id, vec)
	}
}

// BackfillTopicAssignments re-evaluates all unassigned episodes.
func (s *TopicService) BackfillTopicAssignments(episodeStore *EpisodeStore) int {
	eps := episodeStore.ListActive()
	assigned := 0
	for _, ep := range eps {
		if ep.TopicID != 0 || len(ep.Centroid) == 0 {
			continue
		}
		topicID, score := s.TopicRepo.FindBestTopic(ep.Centroid)
		facts := episodeStore.GetFacts(ep.ID)
		kid, kscore := s.FindBestTopicKeyword(facts)
		if kscore > score {
			topicID, score = kid, kscore
		}
		if score >= 0.3 && topicID > 0 {
			s.TopicRepo.AssignEpisodeToTopic(ep.ID, topicID)
			s.TopicRepo.UpdateCentroid(topicID)
			assigned++
		}
	}
	return assigned
}

// FindBestTopicKeyword falls back to keyword matching.
func (s *TopicService) FindBestTopicKeyword(episodeFacts []domain.FactEntry) (int64, float64) {
	var bestID int64
	var bestScore float64

	for _, topicName := range PredefinedTopics {
		keywords, ok := topicKeywords[topicName]
		if !ok {
			continue
		}
		hits := 0
		for _, f := range episodeFacts {
			for _, kw := range keywords {
				if strings.Contains(f.Content, kw) {
					hits++
					break
				}
			}
		}
		score := float64(hits) / float64(len(episodeFacts))
		if score > bestScore {
			bestScore = score
			bestID = s.TopicRepo.FindTopicIDByName(topicName)
		}
	}
	return bestID, bestScore
}
