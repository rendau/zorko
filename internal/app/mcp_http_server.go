package app

import (
	"net/http"
	"time"

	"github.com/rendau/zorko/internal/errs"
	"github.com/rendau/zorko/internal/infra/metrics"
)

// McpHttpServerCreate — сервер MCP для AI-агентов (ADR-020). Ответы — обычный JSON, без потоков:
// таймаут записи покрывает самый долгий инструмент (агрегаты ClickHouse).
func McpHttpServerCreate(port string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              ":" + port,
		Handler:           recoverMiddleware(handler),
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      time.Minute,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    64 * 1024,
	}
}

// mcpToolObserver — вызовы инструментов MCP в тех же метриках, что gRPC: protocol="mcp", method —
// имя инструмента, code отличает сбой от отказа по делу
func mcpToolObserver(tool string, err error, duration time.Duration) {
	if !metrics.Enabled {
		return
	}

	responseStatus, code := "ok", "ok"
	if err != nil {
		responseStatus, code = "error", errs.Code(err)
	}

	metricRequestCounter.WithLabelValues("mcp", tool, responseStatus, code).Inc()
	metricResponseDuration.WithLabelValues("mcp", tool).Observe(duration.Seconds())
}
