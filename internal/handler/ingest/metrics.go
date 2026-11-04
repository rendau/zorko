package ingest

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/rendau/zorko/internal/infra/metrics"
)

var (
	// запросы SDK по исходу
	metricRequestTotal = metrics.Factory.NewCounterVec(prometheus.CounterOpts{
		Name: "ingest_request_total",
	}, []string{"result"})

	// события, ушедшие в очередь
	metricEventAccepted = metrics.Factory.NewCounter(prometheus.CounterOpts{
		Name: "ingest_event_accepted_total",
	})

	// элементы envelope, которые принимаются и выбрасываются (до следующих шагов), и слишком большие события
	metricItemDropped = metrics.Factory.NewCounterVec(prometheus.CounterOpts{
		Name: "ingest_item_dropped_total",
	}, []string{"type"})
)

const (
	resultAccepted    = "accepted"
	resultRateLimited = "rate_limited"
	resultForbidden   = "forbidden"
	resultTooLarge    = "too_large"
	resultBadRequest  = "bad_request"
	resultUnavailable = "unavailable"
)

// knownItemTypes — набор конечен, чтобы тип из запроса не раздул число меток метрики
var knownItemTypes = map[string]bool{
	"transaction": true, "session": true, "sessions": true, "attachment": true, "client_report": true,
	"replay_event": true, "replay_recording": true, "replay_video": true, "profile": true,
	"profile_chunk": true, "check_in": true, "log": true, "span": true, "statsd": true,
	"metric_buckets": true, "feedback": true, "user_report": true, "otel_log": true,
}

func itemTypeLabel(t string) string {
	if knownItemTypes[t] {
		return t
	}
	return "other"
}
