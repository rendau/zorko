package service

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/rendau/zorko/internal/infra/metrics"
)

var (
	// события по исходу обработки
	metricEventTotal = metrics.Factory.NewCounterVec(prometheus.CounterOpts{
		Name: "processor_event_total",
	}, []string{"result"})

	// время этапов обработки пачки
	metricBatchDuration = metrics.Factory.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "processor_batch_seconds",
		Buckets: []float64{0.005, 0.02, 0.1, 0.5, 2, 10},
	}, []string{"stage"})

	// задержка обработки: от приёма самого старого события пачки до записи в базы
	metricLag = metrics.Factory.NewGauge(prometheus.GaugeOpts{
		Name: "processor_lag_seconds",
	})

	// повторы записи после сбоя баз
	metricRetryTotal = metrics.Factory.NewCounterVec(prometheus.CounterOpts{
		Name: "processor_retry_total",
	}, []string{"stage"})
)

const (
	resultStored         = "stored"
	resultCountedOnly    = "counted_only"
	resultBadMessage     = "bad_message"
	resultProjectDeleted = "project_deleted"

	stageNormalize = "normalize"
	stagePg        = "pg"
	stageCh        = "ch"
	stageCommit    = "commit"
)
