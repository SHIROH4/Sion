package memory

import "desktop-pet/internal/domain"

// emotionTag returns a human-readable emotion label from valence/arousal.
func EmotionTag(valence, arousal float64) string {
	switch {
	case valence > 0.3 && arousal > 0.3:
		return "[开心兴奋]"
	case valence > 0.3 && arousal < -0.3:
		return "[平静满足]"
	case valence > 0.3:
		return "[愉悦]"
	case valence < -0.3 && arousal > 0.3:
		return "[焦虑不安]"
	case valence < -0.3 && arousal < -0.3:
		return "[低落疲惫]"
	case valence < -0.3:
		return "[不开心]"
	case arousal > 0.5:
		return "[激动]"
	case arousal < -0.5:
		return "[平静]"
	default:
		return "[中性]"
	}
}

// memCellTypeTag returns a display label for a MemCell type.
func MemCellTypeTag(t domain.MemCellType) string {
	switch t {
	case domain.CellFact:
		return "事实"
	case domain.CellPrefer:
		return "偏好"
	case domain.CellEvent:
		return "事件"
	case domain.CellEmotion:
		return "情绪时刻"
	case domain.CellSkill:
		return "技能"
	case domain.CellRelation:
		return "关系"
	default:
		return "记忆"
	}
}
