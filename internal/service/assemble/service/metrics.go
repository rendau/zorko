package service

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/rendau/zorko/internal/infra/metrics"
)

var (
	// сборки архивов по исходу: ok | invalid (архив отвергнут) | failed (сбой)
	metricAssembleTotal = metrics.Factory.NewCounterVec(prometheus.CounterOpts{
		Name: "sourcemap_assemble_total",
	}, []string{"result"})

	metricAssembleDuration = metrics.Factory.NewHistogram(prometheus.HistogramOpts{
		Name:    "sourcemap_assemble_duration_seconds",
		Buckets: []float64{0.1, 0.5, 2, 10, 60, 300},
	})
)

const (
	resultOk      = "ok"
	resultInvalid = "invalid"
	resultFailed  = "failed"
)
