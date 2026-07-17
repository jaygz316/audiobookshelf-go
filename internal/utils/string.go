package utils

import (
	"strings"
)

// NameToLastFirst converts "First Last" to "Last, First".
func NameToLastFirst(name string) string {
	parts := strings.Fields(name)
	if len(parts) > 1 {
		return parts[len(parts)-1] + ", " + strings.Join(parts[:len(parts)-1], " ")
	}
	return name
}

// NullIfEmpty returns nil if s is empty, otherwise returns a pointer to s.
func NullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// NormalizeTitleForSeries normalizes a book title for grouping duplicate narrator/edition versions in a series.
func NormalizeTitleForSeries(title string) string {
	t := strings.ToLower(title)

	// Normalize common punctuation and spacing
	t = strings.ReplaceAll(t, "’", "'")
	t = strings.ReplaceAll(t, "“", "\"")
	t = strings.ReplaceAll(t, "”", "\"")

	// Keywords indicating narrator/edition information to discard
	keywords := []string{
		"narrat", "read by", "unabridged", "abridged", "edition", "version", "audiobook", "narrator",
	}

	hasKeyword := func(s string) bool {
		for _, kw := range keywords {
			if strings.Contains(s, kw) {
				return true
			}
		}
		return false
	}

	// 1. Strip parentheticals and brackets that contain keywords
	for {
		startIdx := strings.Index(t, "(")
		if startIdx == -1 {
			break
		}
		endIdx := strings.Index(t[startIdx:], ")")
		if endIdx == -1 {
			break
		}
		endIdx += startIdx
		content := t[startIdx+1 : endIdx]
		if hasKeyword(content) || len(content) < 3 {
			t = t[:startIdx] + t[endIdx+1:]
		} else {
			// Replace with spaces to avoid joining words
			t = t[:startIdx] + " " + content + " " + t[endIdx+1:]
		}
	}

	for {
		startIdx := strings.Index(t, "[")
		if startIdx == -1 {
			break
		}
		endIdx := strings.Index(t[startIdx:], "]")
		if endIdx == -1 {
			break
		}
		endIdx += startIdx
		content := t[startIdx+1 : endIdx]
		if hasKeyword(content) || len(content) < 3 {
			t = t[:startIdx] + t[endIdx+1:]
		} else {
			t = t[:startIdx] + " " + content + " " + t[endIdx+1:]
		}
	}

	// 2. Strip sections after " - " (space-dash-space) if it contains keywords
	if idx := strings.Index(t, " - "); idx != -1 {
		rightPart := t[idx+3:]
		if hasKeyword(rightPart) {
			t = t[:idx]
		}
	}

	// 3. Strip trailing colon additions like ": a novel" or ": unabridged" if it contains keywords
	if idx := strings.Index(t, ":"); idx != -1 {
		rightPart := t[idx+1:]
		if hasKeyword(rightPart) {
			t = t[:idx]
		}
	}

	// Clean up whitespace and remaining punctuation
	words := strings.Fields(t)
	var cleanWords []string
	for _, w := range words {
		// strip trailing/leading punctuation
		w = strings.Trim(w, ",.-_=:;!@#$%^&*()[]{}'\"/?\\`~")
		if w != "" {
			cleanWords = append(cleanWords, w)
		}
	}

	return strings.Join(cleanWords, " ")
}
