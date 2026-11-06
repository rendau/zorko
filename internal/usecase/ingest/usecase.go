package ingest

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/samber/lo"

	eventModel "github.com/rendau/zorko/internal/domain/event/model"
	projectModel "github.com/rendau/zorko/internal/domain/project/model"
	replayModel "github.com/rendau/zorko/internal/domain/replay/model"
	"github.com/rendau/zorko/internal/errs"
	"github.com/rendau/zorko/internal/infra/metrics"
	queueModel "github.com/rendau/zorko/internal/service/queue/model"
	"github.com/rendau/zorko/internal/util"
)

var metricProduceDuration = metrics.Factory.NewHistogram(prometheus.HistogramOpts{
	Name:    "ingest_produce_seconds",
	Buckets: []float64{0.002, 0.005, 0.01, 0.025, 0.05, 0.1, 0.5, 2},
})

var metricReplayStoreDuration = metrics.Factory.NewHistogram(prometheus.HistogramOpts{
	Name:    "replay_store_seconds",
	Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
})

type Usecase struct {
	keyCache     KeyCacheI
	rateLimiter  RateLimiterI
	queue        QueueI
	projectLabel ProjectLabelI

	// записи сессий (ADR-012, ADR-013); replayStore == nil — S3 не настроен, записи не принимаются
	replayStore ReplayStoreI
	replayQueue QueueI
	replayLimit ReplayLimitI
}

func New(keyCache KeyCacheI, rateLimiter RateLimiterI, queue QueueI, projectLabel ProjectLabelI,
	replayStore ReplayStoreI, replayQueue QueueI, replayLimit ReplayLimitI) *Usecase {
	return &Usecase{
		keyCache:     keyCache,
		rateLimiter:  rateLimiter,
		queue:        queue,
		projectLabel: projectLabel,
		replayStore:  replayStore,
		replayQueue:  replayQueue,
		replayLimit:  replayLimit,
	}
}

// ProjectLabel — имя проекта для меток метрик приёма; несуществующий проект — "unknown"
func (u *Usecase) ProjectLabel(projectId int64) string {
	return u.projectLabel.Label(projectId)
}

// Ingest проверяет ключ, сайт и предел, кладёт события в очередь, затем — сегмент записи (если есть).
// Отказы: errs.IngestKeyInvalid, errs.IngestOriginDenied, RateLimitedError; прочее — сбой очереди или S3.
// Отказ по записи — не ошибка, а исход в Rep (ответ 200, событиям запроса он не мешает).
func (u *Usecase) Ingest(ctx context.Context, req *Req) (*Rep, error) {
	key, ok := u.keyCache.Find(req.ProjectId, req.PublicKey)
	if !ok {
		return nil, errs.IngestKeyInvalid
	}

	if req.OriginHost != "" && !util.OriginAllowed(req.OriginHost, key.AllowedOrigin) {
		return nil, errs.IngestOriginDenied
	}

	receivedAt := time.Now().UTC()

	if err := u.ingestEvents(ctx, req, key.Id, key.RateLimitPerSec, receivedAt); err != nil {
		return nil, err
	}

	rep := &Rep{}
	if req.Replay != nil {
		var err error
		rep.ReplayResult, rep.ReplayStop, err = u.ingestReplay(ctx, req, receivedAt)
		if err != nil {
			return rep, err
		}
	}

	return rep, nil
}

func (u *Usecase) ingestEvents(ctx context.Context, req *Req, keyId int64, limitPerSec int32, receivedAt time.Time) error {
	if len(req.Events) == 0 {
		return nil
	}

	if allowed, retryAfter := u.rateLimiter.Allow(keyId, limitPerSec, len(req.Events)); !allowed {
		return RateLimitedError{RetryAfter: retryAfter}
	}

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

// SDK пишет не дольше часа; запас — на повторы отправки. Оба времени — с часов устройства:
// сдвиг часов на них не влияет, а день в S3 и срок хранения считаются по часам сервера
const replayMaxDuration = 61 * time.Minute

// ingestReplay — режим проекта, вид записи, предел в час, затем тело в S3 и описание в очередь (ADR-012, ADR-013)
func (u *Usecase) ingestReplay(ctx context.Context, req *Req, receivedAt time.Time) (string, time.Duration, error) {
	r := req.Replay
	settings := u.projectLabel.Replay(req.ProjectId)

	switch {
	case u.replayStore == nil || settings.Mode == projectModel.ReplayModeOff:
		return ReplayResultDisabled, ReplayStopMode, nil
	case settings.Mode == projectModel.ReplayModeError && r.Type != replayModel.TypeBuffer:
		return ReplayResultTypeNotAllowed, ReplayStopMode, nil
	case settings.Mode != projectModel.ReplayModeError && settings.Mode != projectModel.ReplayModeSession:
		return ReplayResultDisabled, ReplayStopMode, nil
	}

	if r.Start.IsZero() || r.Timestamp.IsZero() || r.Timestamp.After(r.Start.Add(replayMaxDuration)) {
		return ReplayResultInvalid, 0, nil
	}

	if r.SegmentId == 0 {
		if ok, wait := u.replayLimit.Allow(req.ProjectId, settings.HourLimit, receivedAt); !ok {
			return ReplayResultHourLimit, wait, nil
		}
	}

	seg := &replayModel.Segment{ProjectId: req.ProjectId, ReplayId: r.ReplayId, SegmentId: r.SegmentId, ReceivedAt: receivedAt}

	start := time.Now()
	size, err := u.replayStore.PutSegmentBody(ctx, seg, r.Body, r.Compressed)
	metricReplayStoreDuration.Observe(time.Since(start).Seconds())
	if err != nil {
		return ReplayResultUnavailable, 0, fmt.Errorf("replayStore.PutSegmentBody: %w", err)
	}

	err = u.replayQueue.PublishEvents(ctx, []*queueModel.Event{{
		EventId:    r.ReplayId,
		ProjectId:  req.ProjectId,
		ReceivedAt: receivedAt,
		ClientIp:   req.ClientIp,
		Payload:    r.Event,
		BodySize:   size,
	}})
	if err != nil {
		return ReplayResultUnavailable, 0, fmt.Errorf("replayQueue.PublishEvents: %w", err)
	}

	return ReplayResultAccepted, 0, nil
}
