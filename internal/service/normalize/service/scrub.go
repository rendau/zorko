package service

import (
	"regexp"
	"strings"
)

const filtered = "[Filtered]"

// ключи, значения которых вырезаются (подстрока, без учёта регистра)
var sensitiveKeyParts = []string{
	"password", "passwd", "secret", "token", "api_key", "apikey", "auth",
	"cookie", "session", "csrf", "card", "cvv",
}

var cardNumberRe = regexp.MustCompile(`\b(?:\d[ -]?){12,18}\d\b`)

// scrub вырезает чувствительные значения во всём дереве JSON на месте
func scrub(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			if isSensitiveKey(k) && child != nil {
				x[k] = filtered
				continue
			}
			x[k] = scrub(child)
		}
		return x
	case []any:
		// пары [ключ, значение] (заголовки запроса, теги списком)
		if len(x) == 2 {
			if k, ok := x[0].(string); ok && isSensitiveKey(k) {
				x[1] = filtered
				return x
			}
		}
		for i, child := range x {
			x[i] = scrub(child)
		}
		return x
	case string:
		return scrubCardNumbers(x)
	default:
		return v
	}
}

func isSensitiveKey(k string) bool {
	k = strings.ToLower(k)
	for _, part := range sensitiveKeyParts {
		if strings.Contains(k, part) {
			return true
		}
	}
	return false
}

// scrubCardNumbers — 13–19 цифр (можно через пробел или дефис), прошедшие проверку Луна
func scrubCardNumbers(s string) string {
	if len(s) < 13 {
		return s
	}
	return cardNumberRe.ReplaceAllStringFunc(s, func(m string) string {
		if luhn(m) {
			return filtered
		}
		return m
	})
}

func luhn(s string) bool {
	sum, n, double := 0, 0, false
	for i := len(s) - 1; i >= 0; i-- {
		c := s[i]
		if c < '0' || c > '9' {
			continue
		}
		d := int(c - '0')
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		n++
		double = !double
	}
	return n >= 13 && n <= 19 && sum%10 == 0
}
