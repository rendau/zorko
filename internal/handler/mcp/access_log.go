package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	oauthService "github.com/rendau/zorko/internal/domain/oauth/service"
)

// rpcPeekMaxBytes — сколько тела JSON-RPC читается заранее ради метода в логе;
// большее тело уходит дальше нетронутым и пишется без метода.
const rpcPeekMaxBytes = 64 * 1024

// statusRecorder запоминает статус ответа для access-лога. Unwrap даёт
// http.ResponseController добраться до Flush потоковых (SSE) ответов.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *statusRecorder) Unwrap() http.ResponseWriter {
	return s.ResponseWriter
}

// withAccessLog пишет каждый запрос MCP-порта. Клиенты приходят через
// Cloudflare и traefik, поэтому адрес — из их заголовков; cf_ray находит запрос
// в событиях безопасности Cloudflare. Учётные данные не пишутся, только их вид.
func withAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		rpcMethod := peekRPCMethod(r)
		rec := &statusRecorder{ResponseWriter: w}

		next.ServeHTTP(rec, r)

		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}
		slog.Info("mcp: request",
			"method", r.Method,
			"path", r.URL.Path,
			"rpc_method", rpcMethod,
			"status", status,
			"duration_ms", time.Since(started).Milliseconds(),
			"credential", credentialKind(r),
			"client_ip", clientIP(r),
			"cf_ray", r.Header.Get("Cf-Ray"),
			"user_agent", r.UserAgent(),
		)
	})
}

// peekRPCMethod читает JSON-RPC метод (или методы пакета) у POST и возвращает
// прочитанные байты обратно в начало тела.
func peekRPCMethod(r *http.Request) string {
	if r.Method != http.MethodPost || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return ""
	}

	head, err := io.ReadAll(io.LimitReader(r.Body, rpcPeekMaxBytes+1))
	r.Body = struct {
		io.Reader
		io.Closer
	}{io.MultiReader(bytes.NewReader(head), r.Body), r.Body}
	if err != nil || len(head) > rpcPeekMaxBytes {
		return ""
	}

	type rpcMessage struct {
		Method string `json:"method"`
	}

	head = bytes.TrimSpace(head)
	if bytes.HasPrefix(head, []byte("[")) {
		var batch []rpcMessage
		if json.Unmarshal(head, &batch) != nil {
			return ""
		}
		methods := make([]string, 0, len(batch))
		for _, m := range batch {
			methods = append(methods, m.Method)
		}
		return strings.Join(methods, ",")
	}

	var msg rpcMessage
	if json.Unmarshal(head, &msg) != nil {
		return ""
	}
	return msg.Method
}

func credentialKind(r *http.Request) string {
	credential := bearerToken(r.Header.Get("Authorization"))
	switch {
	case credential == "":
		return "none"
	case strings.HasPrefix(credential, oauthService.AccessTokenPrefix):
		return "oauth"
	}
	return "unknown"
}

func clientIP(r *http.Request) string {
	if v := r.Header.Get("Cf-Connecting-Ip"); v != "" {
		return v
	}
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		first, _, _ := strings.Cut(v, ",")
		return strings.TrimSpace(first)
	}
	if v := r.Header.Get("X-Real-Ip"); v != "" {
		return v
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
