package util

import (
	"net"
	"net/url"
	"regexp"
	"strings"
)

var hostPatternRe = regexp.MustCompile(`^(\*\.)?([a-z0-9]([a-z0-9-]*[a-z0-9])?\.)*[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// NormalizeOriginPattern — разрешённый сайт ключа: хост (`shop.mdev.kz`), маска (`*.mdev.kz`)
// или `*` (любой). Схема, порт и путь, если их вписали, отбрасываются.
func NormalizeOriginPattern(v string) (string, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "*" {
		return v, true
	}
	if strings.Contains(v, "://") {
		u, err := url.Parse(v)
		if err != nil {
			return "", false
		}
		v = u.Host
	}
	if host, _, err := net.SplitHostPort(v); err == nil {
		v = host
	}
	v, _, _ = strings.Cut(v, "/")
	if !hostPatternRe.MatchString(v) {
		return "", false
	}
	return v, true
}

// OriginAllowed — хост подходит хотя бы под одну маску; пустой список — разрешено всё.
// `*.mdev.kz` подходит и для `shop.mdev.kz`, и для самого `mdev.kz`.
func OriginAllowed(host string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}
	host = strings.ToLower(host)
	for _, p := range patterns {
		switch {
		case p == "*", p == host:
			return true
		case strings.HasPrefix(p, "*."):
			base := p[2:]
			if host == base || strings.HasSuffix(host, "."+base) {
				return true
			}
		}
	}
	return false
}
