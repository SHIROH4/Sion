package domain

// FactRepository manages fact CRUD, recall tracking, vectorisation, annotation,
// and fact–episode linking.
type FactRepository interface {
	// Fact CRUD.
	SaveFact(content, source string) error
	SaveAtomicFact(input AtomicFactInput) error
	SaveFactWithVector(input AtomicFactInput, vec []float32) (int64, error)
	LoadFacts() []string
	ListActiveFacts(threshold float64) []FactEntry
	GetRecentFacts(since int64) []FactEntry
	ArchiveFact(id int64) error
	CleanArchivedFacts(retentionDays int) int
	ReplaceFact(oldID, newID int64) error
	UpdateFactContent(id int64, content string) error

	// Recall tracking.
	UpdateFactRecall(id int64) error
	BatchUpdateFactRecall(ids []int64) error

	// Vector operations.
	VectorizeFact(id int64, vec []float32) error
	FactsWithoutVectors() ([]FactEntry, error)

	// Annotation.
	UnlabeledFacts() ([]FactEntry, error)
	UpdateFactAnnotations(id int64, role FactRole, startTime, endTime int64) error

	// Fact–Episode linking.
	AttachFactToEpisode(factID, episodeID int64) error
}
