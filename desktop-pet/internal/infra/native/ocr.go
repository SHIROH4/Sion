package native

// OCRImage runs macOS Vision OCR on the given image file and returns extracted text.
func OCRImage(imagePath string) (string, error) {
	return ocrImage(imagePath)
}

// OCRActiveScreen captures the frontmost window, runs OCR, and returns a
// ScreenObservation with the extracted text and activity classification.
func OCRActiveScreen() (ScreenObservation, error) {
	return ocrActiveScreen()
}
