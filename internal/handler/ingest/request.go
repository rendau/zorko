package ingest

import (
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
)

// publicKeyFromRequest — ключ из X-Sentry-Auth (или Authorization) либо из query sentry_key
func publicKeyFromRequest(r *http.Request) string {
	for _, header := range []string{"X-Sentry-Auth", "Authorization"} {
		if key := parseSentryAuth(r.Header.Get(header)); key != "" {
			return key
		}
	}
	return r.URL.Query().Get("sentry_key")
}

// parseSentryAuth — `Sentry sentry_key=<key>, sentry_version=7, sentry_client=…`
func parseSentryAuth(v string) string {
	v = strings.TrimSpace(v)
	if len(v) < 7 || !strings.EqualFold(v[:7], "sentry ") {
		return ""
	}
	for _, part := range strings.Split(v[7:], ",") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && name == "sentry_key" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// publicKeyFromDsn — ключ из dsn в заголовке envelope
func publicKeyFromDsn(dsn string) string {
	if dsn == "" {
		return ""
	}
	u, err := url.Parse(dsn)
	if err != nil || u.User == nil {
		return ""
	}
	return u.User.Username()
}

// originHost — хост сайта из Origin, иначе из Referer; пусто — запрос не из браузера
func originHost(r *http.Request) string {
	for _, header := range []string{"Origin", "Referer"} {
		v := r.Header.Get(header)
		if v == "" || v == "null" {
			continue
		}
		if u, err := url.Parse(v); err == nil && u.Hostname() != "" {
			return strings.ToLower(u.Hostname())
		}
	}
	return ""
}

// clientIp — первый адрес X-Forwarded-For (запрос идёт через ingress), иначе X-Real-IP, иначе соединение
func clientIp(r *http.Request) string {
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		first, _, _ := strings.Cut(v, ",")
		if ip := net.ParseIP(strings.TrimSpace(first)); ip != nil {
			return ip.String()
		}
	}
	if ip := net.ParseIP(strings.TrimSpace(r.Header.Get("X-Real-IP"))); ip != nil {
		return ip.String()
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// eventId — id события в формате Sentry (32 hex, нижний регистр): из заголовка envelope,
// иначе из тела события, иначе новый
func eventId(candidates ...string) string {
	for _, c := range candidates {
		if id, ok := normalizeEventId(c); ok {
			return id
		}
	}
	return strings.ReplaceAll(uuid.NewString(), "-", "")
}

func eventIdFromPayload(payload []byte) string {
	var v struct {
		EventId string `json:"event_id"`
	}
	_ = json.Unmarshal(payload, &v)
	return v.EventId
}

func normalizeEventId(v string) (string, bool) {
	v = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(v), "-", ""))
	if len(v) != 32 {
		return "", false
	}
	if _, err := hex.DecodeString(v); err != nil {
		return "", false
	}
	return v, true
}
