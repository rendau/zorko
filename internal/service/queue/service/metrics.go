package service

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/rendau/zorko/internal/infra/metrics"
)

var (
	// неразобранные сообщения группы обработки по всем разделам темы (по данным брокера);
	// растёт, когда обработка не успевает или стоит, — в отличие от processor_lag_seconds,
	// который обновляется только после разобранной пачки
	// topic — тема (события, сегменты записей)
	metricConsumerLag = metrics.Factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "queue_consumer_lag_messages",
	}, []string{"topic"})

	// неудачные запросы отставания у брокера: значение metricConsumerLag в это время устаревает
	metricConsumerLagErrorTotal = metrics.Factory.NewCounterVec(prometheus.CounterOpts{
		Name: "queue_consumer_lag_error_total",
	}, []string{"topic"})
)
