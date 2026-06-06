package domain

// DiaryRepository manages diary entry persistence and vector search.
type DiaryRepository interface {
	Save(entry *DiaryEntry) error
	Search(queryVector []float32, topK int) ([]DiaryEntry, error)
	ListRecent(limit int) []DiaryEntry
	Delete(id int64) error
	ArchiveDiary(id int64) error
	Count() int
	VectorizeDiary(id int64, vec []float32) error // update vector for existing entry
}
