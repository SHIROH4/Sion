package memory

import "strings"

// InferCareAcceptance returns false for strong rejection keywords, true otherwise.
func InferCareAcceptance(reply string) bool {
	strongReject := []string{
		"别烦我", "烦死了", "忙着呢", "别吵", "别管", "别说了", "闭嘴", "滚",
		"不要你管", "不用你管", "你很烦", "走开", "别来",
	}
	for _, p := range strongReject {
		if strings.Contains(reply, p) {
			return false
		}
	}
	softReject := []string{
		"不用", "不要", "不了", "不饿", "不渴", "不累", "不困",
		"算了", "下次", "等会", "晚点", "稍后", "再说", "先不",
		"现在不", "今天不", "暂时不", "不需要",
	}
	for _, p := range softReject {
		if strings.Contains(reply, p) {
			return false
		}
	}
	return true
}
