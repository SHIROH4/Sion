package memory

import "desktop-pet/internal/infra/storage"

// Re-exported from storage to avoid import duplication in existing callers.

const ActiveThreshold = storage.ActiveThreshold
const CoreThreshold = storage.CoreThreshold
const DefaultHalfLifeDays = storage.DefaultHalfLifeDays
const DefaultBoostPerRecall = storage.DefaultBoostPerRecall

var DecayWeight = storage.DecayWeight
