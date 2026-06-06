package memory

import (
	"desktop-pet/internal/app/background"
)

// ---- thin wrappers delegating to app/background ----

func (p *MemoryPlugin) ReflectAndForget() {
	logFn := func(msg string, args ...any) {
		if p.pctx.Logger != nil {
			p.pctx.Logger.Info(msg, args...)
		}
	}
	background.ReflectAndForget(p.store, p.rawLLM, &p.lastReflectAt, logFn)
}

func (p *MemoryPlugin) migrateOldFacts() {
	background.MigrateOldFacts(p.store, p.rawLLM, CleanJSON)
}
