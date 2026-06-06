package domain

// ScreenshotAnalyzer processes screenshots for the user, extracting text via OCR
// and/or multi-modal vision models.
type ScreenshotAnalyzer interface {
	AnalyzeScreenshotText(ocrText, question string) (string, error)
	AnalyzeScreenshotWithImage(base64Data, format, question string) (string, error)
	IsRunning() bool
}
