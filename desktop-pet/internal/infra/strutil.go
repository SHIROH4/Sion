package infra

import "strings"

// CleanJSON strips markdown code fences and extracts the JSON object from raw LLM output.
// Uses brace-matching to find the first complete JSON object.
func CleanJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	if start := strings.Index(raw, "{"); start >= 0 {
		raw = raw[start:]
		depth, end := 0, -1
		for i, ch := range raw {
			if ch == '{' {
				depth++
			}
			if ch == '}' {
				depth--
				if depth == 0 {
					end = i
					break
				}
			}
		}
		if end >= 0 {
			raw = raw[:end+1]
		}
	}
	return strings.TrimSpace(raw)
}

// Truncate truncates a string to n runes without breaking multi-byte characters.
func Truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "..."
}

// EscapeJSON escapes special characters in a string for JSON string embedding.
func EscapeJSON(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s
}
