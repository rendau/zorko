package service

import (
	"regexp"
	"strings"
)

// изменчивые куски текста → заглушки, чтобы «Order 81723 not found» и «Order 90011 not found»
// давали один отпечаток; порядок важен: сначала длинные шаблоны, числа — последними
var textReplacements = []struct {
	re          *regexp.Regexp
	placeholder string
}{
	{regexp.MustCompile(`(?i)\b[0-9a-f]{8}-?[0-9a-f]{4}-?[0-9a-f]{4}-?[0-9a-f]{4}-?[0-9a-f]{12}\b`), "<uuid>"},
	{regexp.MustCompile(`(?i)[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}`), "<email>"},
	{regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s"'<>]+`), "<url>"},
	{regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`), "<ip>"},
}

var (
	hexWordRe = regexp.MustCompile(`(?i)\b[0-9a-f]{8,}\b`)
	numberRe  = regexp.MustCompile(`\d+`)
)

func normalizeText(s string) string {
	s = strings.TrimSpace(s)
	for _, r := range textReplacements {
		s = r.re.ReplaceAllString(s, r.placeholder)
	}
	// hex от 8 символов — только если в нём есть цифра, иначе это обычное слово («deadline»)
	s = hexWordRe.ReplaceAllStringFunc(s, func(m string) string {
		if strings.ContainsAny(m, "0123456789") {
			return "<hex>"
		}
		return m
	})
	return numberRe.ReplaceAllString(s, "<int>")
}
