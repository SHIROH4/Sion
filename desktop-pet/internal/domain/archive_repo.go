package domain

// ArchiveRepository manages compressed context archives.
type ArchiveRepository interface {
	SaveArchive(name string, level int, original, summary string) error
	FindArchiveByName(name string) (string, error)
}
