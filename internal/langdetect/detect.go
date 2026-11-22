// Package langdetect provides a dependency-free language heuristic tuned for
// Russian/English news content. It is intentionally small and predictable.
package langdetect

import "strings"

// Detect returns "ru" or "en" (or "" for text without letters).
func Detect(text string) string {
	var cyrillic, latin int
	for _, r := range text {
		switch {
		case r >= 'А' && r <= 'я', r == 'Ё', r == 'ё':
			cyrillic++
		case (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z'):
			latin++
		}
	}
	total := cyrillic + latin
	if total == 0 {
		return ""
	}
	if cyrillic*100/total >= 25 {
		return "ru"
	}
	return "en"
}

// Normalize maps language tags like ru-RU, en_US to a two letter code.
func Normalize(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	if code == "" {
		return ""
	}
	if i := strings.IndexAny(code, "-_"); i > 0 {
		code = code[:i]
	}
	switch code {
	case "ru", "en", "de", "fr", "es", "it", "pt", "uk", "tr", "ar", "zh", "ja":
		return code
	}
	return code
}

// IsCyrillic reports whether the text is predominantly Cyrillic.
func IsCyrillic(text string) bool { return Detect(text) == "ru" }
