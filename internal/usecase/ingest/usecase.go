package ingest

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/samber/lo"

	eventModel "github.com/rendau/zorko/internal/domain/event/model"
	"github.com/rendau/zorko/internal/errs"
	"github.com/rendau/zorko/internal/infra/metrics"
	queueModel "github.com/rendau/zorko/internal/service/queue/model"
	"github.com/rendau/zorko/internal/util"
)

var metricProduceDuration = metrics.Factory.NewHistogram(prometheus.HistogramOpts{
	Name:    "ingest_produce_seconds",
	Buckets: []float64{0.002, 0.005, 0.01, 0.025, 0.05, 0.1, 0.5, 2},
})

type Usecase struct {
	keyCache    KeyCacheI
	rateLimiter RateLimiterI
	queue       QueueI
}

func New(keyCache KeyCacheI, rateLimiter RateLimiterI, queue QueueI) *Usecase {
	return &Usecase{
		keyCache:    keyCache,
		rateLimiter: rateLimiter,
		queue:       queue,
	}
}

// Ingest проверяет ключ, сайт и предел и кладёт события в очередь.
// Отказы: errs.IngestKeyInvalid, errs.IngestOriginDenied, RateLimitedError; прочее — сбой очереди.
func (u *Usecase) Ingest(ctx context.Context, req *Req) error {
	key, ok := u.keyCache.Find(req.ProjectId, req.PublicKey)
	if !ok {
		return errs.IngestKeyInvalid
	}

	if req.OriginHost != "" && !util.OriginAllowed(req.OriginHost, key.AllowedOrigin) {
		return errs.IngestOriginDenied
	}

	if len(req.Events) == 0 {
		return nil
	}

	if allowed, retryAfter := u.rateLimiter.Allow(key.Id, key.RateLimitPerSec, len(req.Events)); !allowed {
		return RateLimitedError{RetryAfter: retryAfter}
	}

	receivedAt := time.Now().UTC()

	events := lo.Map(req.Events, func(v *eventModel.RawEvent, _ int) *queueModel.Event {
		return &queueModel.Event{
			EventId:    v.EventId,
			ProjectId:  req.ProjectId,
			ReceivedAt: receivedAt,
			ClientIp:   req.ClientIp,
			Payload:    v.Payload,
		}
	})

	start := time.Now()
	err := u.queue.PublishEvents(ctx, events)
	metricProduceDuration.Observe(time.Since(start).Seconds())
	if err != nil {
		return fmt.Errorf("queue.PublishEvents: %w", err)
	}

	return nil
}
