package background

import (
	"encoding/json"
	"fmt"
	"strings"

	"desktop-pet/internal/domain"
)

// ---- Backfill & migration helpers ----

// BackfillFactVectors computes and stores vectors for all facts without them.
func MigrateOldFacts(store domain.FactRepository, rawLLM func([]domain.Message) (string, error), cleanJSON func(string) string) {
	facts, err := store.UnlabeledFacts()
	if err != nil || len(facts) == 0 {
		return
	}
	var factsText strings.Builder
	for _, f := range facts {
		factsText.WriteString(fmt.Sprintf("- [id:%d] %s\n", f.ID, f.Content))
	}
	prompt := fmt.Sprintf(migrateOldFactsPrompt, factsText.String())
	result, err := rawLLM([]domain.Message{{Role: "user", Content: prompt}})
	if err != nil {
		return
	}
	var updates []struct {
		ID        int64  `json:"id"`
		FactRole  string `json:"fact_role"`
		StartTime int64  `json:"start_time"`
		EndTime   int64  `json:"end_time"`
	}
	if err := json.Unmarshal([]byte(cleanJSON(result)), &updates); err != nil {
		return
	}
	for _, u := range updates {
		if u.FactRole != "" {
			store.UpdateFactAnnotations(u.ID, domain.FactRole(u.FactRole), u.StartTime, u.EndTime)
		}
	}
}

const migrateOldFactsPrompt = "## 事实标注迁移\n\n以下是记忆库中的旧事实...\n%s\n\n### 输出格式\nJSON 数组"
