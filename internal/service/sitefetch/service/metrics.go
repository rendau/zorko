package service

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/rendau/zorko/internal/infra/metrics"
)

// походы на сайт по исходу (ok, site_http, site_timeout, …)
var metricFetchTotal = metrics.Factory.NewCounterVec(prometheus.CounterOpts{
	Name: "site_fetch_total",
}, []string{"result"})
