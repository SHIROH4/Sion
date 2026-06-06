package domain

// MemCellRepository manages atomic memory cell persistence.
type MemCellRepository interface {
	SaveMemCell(t string, content string, importance float64, valence, arousal float64, sourceMsg string) error
	ListMemCells(cellType string, limit int) []MemCell
}
