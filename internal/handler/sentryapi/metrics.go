package sentryapi

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/rendau/zorko/internal/infra/metrics"
)

var (
	// запросы sentry-cli по ручке и коду ответа
	metricRequestTotal = metrics.Factory.NewCounterVec(prometheus.CounterOpts{
		Name: "sentryapi_request_total",
	}, []string{"route", "status"})

	// принятые байты кусков (распакованные)
	metricUploadBytes = metrics.Factory.NewCounter(prometheus.CounterOpts{
		Name: "sourcemap_upload_bytes_total",
	})
)
