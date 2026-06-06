package main

import (
	"desktop-pet/internal/infra/native"
	"fmt"
)

func main() {
	fmt.Println("=== OCRActiveScreen 完整测试 ===")

	obs, err := native.OCRActiveScreen()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	}
	fmt.Printf("AppName:      %q\n", obs.AppName)
	fmt.Printf("WindowTitle:  %q\n", obs.WindowTitle)
	fmt.Printf("IsWorking:    %v\n", obs.IsWorking)
	fmt.Printf("OCRText (%d chars):\n%s\n", len(obs.OCRText), obs.OCRText)
}
