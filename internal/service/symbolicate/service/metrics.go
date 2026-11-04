package service

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/rendau/zorko/internal/infra/metrics"
)

var (
	// кадры по исходу: result — ok или код причины; source — откуда карта
	metricFrameTotal = metrics.Factory.NewCounterVec(prometheus.CounterOpts{
		Name: "symbolicate_frame_total",
	}, []string{"result", "source"})

	metricBatchDuration = metrics.Factory.NewHistogram(prometheus.HistogramOpts{
		Name:    "symbolicate_batch_duration_seconds",
		Buckets: []float64{0.001, 0.01, 0.05, 0.2, 1, 5, 10},
	})

	metricCacheBytes = metrics.Factory.NewGauge(prometheus.GaugeOpts{
		Name: "sourcemap_cache_bytes",
	})
)
