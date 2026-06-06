import Vision
import AppKit
import Foundation

// OCR Helper — reads an image file path from stdin, outputs extracted text to stdout.
// Usage: ./ocr_helper <image_path>

func main() {
    guard CommandLine.arguments.count > 1 else {
        print("usage: ocr_helper <image_path>")
        exit(1)
    }

    let imagePath = CommandLine.arguments[1]
    guard FileManager.default.fileExists(atPath: imagePath) else {
        print("file not found: \(imagePath)")
        exit(1)
    }

    guard let image = NSImage(contentsOfFile: imagePath),
          let cgImage = image.cgImage(forProposedRect: nil, context: nil, hints: nil) else {
        print("failed to load image: \(imagePath)")
        exit(1)
    }

    let semaphore = DispatchSemaphore(value: 0)
    var outputText = ""

    let request = VNRecognizeTextRequest { request, error in
        defer { semaphore.signal() }
        if let error = error {
            outputText = "ocr error: \(error.localizedDescription)"
            return
        }
        guard let observations = request.results as? [VNRecognizedTextObservation] else {
            outputText = ""
            return
        }
        let lines = observations.compactMap { obs in
            obs.topCandidates(1).first?.string
        }
        outputText = lines.joined(separator: "\n")
    }

    request.recognitionLevel = .accurate
    request.usesLanguageCorrection = true
    request.recognitionLanguages = ["zh-Hans", "zh-Hant", "ja", "en"]

    let handler = VNImageRequestHandler(cgImage: cgImage, options: [:])

    DispatchQueue.global(qos: .userInitiated).async {
        try? handler.perform([request])
    }

    semaphore.wait()
    print(outputText)
}

main()
