package ingest

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/rendau/zorko/internal/infra/metrics"
)

var (
	// запросы SDK по исходу; project — slug проекта или "unknown" (набор конечен, см. service/projectlabel)
	metricRequestTotal = metrics.Factory.NewCounterVec(prometheus.CounterOpts{
		Name: "ingest_request_total",
	}, []string{"result", "project"})

	// события, ушедшие в очередь
	metricEventAccepted = metrics.Factory.NewCounterVec(prometheus.CounterOpts{
		Name: "ingest_event_accepted_total",
	}, []string{"project"})

	// элементы envelope, которые принимаются и выбрасываются (до следующих шагов), и слишком большие события
	metricItemDropped = metrics.Factory.NewCounterVec(prometheus.CounterOpts{
		Name: "ingest_item_dropped_total",
	}, []string{"type", "project"})

	// сегменты записей сессий по исходу (usecase.ReplayResult*)
	metricReplaySegment = metrics.Factory.NewCounterVec(prometheus.CounterOpts{
		Name: "replay_segment_total",
	}, []string{"result", "project"})

	// принятые тела сегментов, байт (как пришли от SDK)
	metricReplayBytes = metrics.Factory.NewCounterVec(prometheus.CounterOpts{
		Name: "replay_bytes_total",
	}, []string{"project"})

	// новые записи (принятый сегмент 0) по виду: buffer — из-за ошибки, session — доля сессий
	metricReplayNew = metrics.Factory.NewCounterVec(prometheus.CounterOpts{
		Name: "replay_new_total",
	}, []string{"replay_type", "project"})

	// полное время ответа SDK: чтение тела, проверки, запись в очередь
	metricRequestDuration = metrics.Factory.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "ingest_request_duration_seconds",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	}, []string{"project"})
)

const (
	resultAccepted    = "accepted"  // принят, в нём есть хотя бы одна ошибка
	resultNoEvents    = "no_events" // принят, но ошибок в нём нет: служебное SDK (session, client_report…), ошибка больше предела, отброшенная запись
	resultReplay      = "replay"    // принят сегмент записи сессии (ошибок в запросе нет)
	resultRateLimited = "rate_limited"
	resultForbidden   = "forbidden"
	resultTooLarge    = "too_large"
	resultBadRequest  = "bad_request"
	resultUnavailable = "unavailable"

	itemEventTooLarge = "event_too_large"
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
