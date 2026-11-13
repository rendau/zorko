package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/samber/lo"

	eventModel "github.com/rendau/zorko/internal/domain/event/model"
	replayModel "github.com/rendau/zorko/internal/domain/replay/model"
	"github.com/rendau/zorko/internal/infra/metrics"
	queueModel "github.com/rendau/zorko/internal/service/queue/model"
)

var (
	// описания сегментов по исходу: stored | bad_message
	metricSegmentTotal = metrics.Factory.NewCounterVec(prometheus.CounterOpts{
		Name: "replay_processor_segment_total",
	}, []string{"result", "project"})

	metricRetryTotal = metrics.Factory.NewCounter(prometheus.CounterOpts{
		Name: "replay_processor_retry_total",
	})

	// сколько ждал самый старый сегмент пачки от приёма до записи; как и у ошибок, сюда входит
	// ожидание пачки (processor_batch_wait_seconds)
	metricLag = metrics.Factory.NewGauge(prometheus.GaugeOpts{
		Name: "replay_processor_lag_seconds",
	})
)

const (
	maxUrls     = 100
	maxUrlLen   = 2048
	maxErrorIds = 100
)

type Config struct {
	BatchSize    int
	RetryMin     time.Duration
	RetryMax     time.Duration
	StageTimeout time.Duration
}

type Service struct {
	conf          Config
	queue         queueI
	normalizer    normalizerI
	replayService replayServiceI
	projectLabel  projectLabelI

	wg sync.WaitGroup
}

func New(conf Config, queue queueI, normalizer normalizerI, replayService replayServiceI, projectLabel projectLabelI) *Service {
	return &Service{conf: conf, queue: queue, normalizer: normalizer, replayService: replayService, projectLabel: projectLabel}
}

// Start — цикл до отмены ctx; начатая пачка дописывается и подтверждается и после отмены
func (s *Service) Start(ctx context.Context) {
	s.wg.Go(func() {
		for ctx.Err() == nil {
			batch, err := s.queue.PollEvents(ctx, s.conf.BatchSize)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				slog.Error("replayproc: queue.PollEvents", "error", err)
				sleep(ctx, s.conf.RetryMin)
				continue
			}

			if !s.processBatch(ctx, batch) {
				return
			}

			if err = s.retry(ctx, func(c context.Context) error { return s.queue.CommitPolled(c) }); err != nil {
				return
			}
		}
	})
}

func (s *Service) Wait() {
	s.wg.Wait()
}

// processBatch — false, если пачку пришлось бросить из-за остановки
func (s *Service) processBatch(ctx context.Context, batch []*queueModel.Event) bool {
	if len(batch) == 0 {
		return true
	}

	segments := make([]*replayModel.Segment, 0, len(batch))
	for _, item := range batch {
		seg, err := s.decode(item)
		if err != nil {
			metricSegmentTotal.WithLabelValues("bad_message", s.projectLabel.Label(item.ProjectId)).Inc()
			slog.Warn("replayproc: bad segment", "error", err, "replay_id", item.EventId, "project_id", item.ProjectId)
			continue
		}
		segments = append(segments, seg)
	}

	if err := s.retry(ctx, func(c context.Context) error { return s.replayService.InsertSegments(c, segments) }); err != nil {
		return false
	}

	for project, n := range lo.CountValuesBy(segments, func(v *replayModel.Segment) string { return s.projectLabel.Label(v.ProjectId) }) {
		metricSegmentTotal.WithLabelValues("stored", project).Add(float64(n))
	}
	oldest := lo.MinBy(batch, func(a, b *queueModel.Event) bool { return a.ReceivedAt.Before(b.ReceivedAt) })
	metricLag.Set(time.Since(oldest.ReceivedAt).Seconds())

	return true
}

// replayEventRep — поля описания, которых нет у обычного события
type replayEventRep struct {
	ReplayId             string   `json:"replay_id"`
	SegmentId            *int     `json:"segment_id"`
	ReplayType           string   `json:"replay_type"`
	ReplayStartTimestamp *float64 `json:"replay_start_timestamp"`
	Timestamp            *float64 `json:"timestamp"`
	Urls                 []string `json:"urls"`
	ErrorIds             []string `json:"error_ids"`
}

func (s *Service) decode(item *queueModel.Event) (*replayModel.Segment, error) {
	if item.ProjectId == 0 {
		return nil, errors.New("message without project_id")
	}

	var rep replayEventRep
	if err := json.Unmarshal(item.Payload, &rep); err != nil {
		return nil, fmt.Errorf("replay_event: %w", err)
	}
	replayId := replayModel.NormalizeId(rep.ReplayId)
	if replayId == "" || rep.SegmentId == nil || *rep.SegmentId < 0 || *rep.SegmentId > replayModel.MaxSegmentId ||
		rep.ReplayStartTimestamp == nil || rep.Timestamp == nil {
		return nil, errors.New("replay_event: no replay_id, segment_id or timestamps")
	}

	// пользователь, браузер, ОС, релиз, окружение, SDK — как у ошибки (с вычисткой и IP «{{auto}}»)
	ev, err := s.normalizer.Normalize(&eventModel.RawEvent{
		EventId:    replayId,
		ProjectId:  item.ProjectId,
		ReceivedAt: item.ReceivedAt,
		ClientIp:   item.ClientIp,
		Payload:    item.Payload,
	})
	if err != nil {
		return nil, fmt.Errorf("normalize: %w", err)
	}

	urls := lo.FilterMap(rep.Urls, func(v string, _ int) (string, bool) { return truncate(v, maxUrlLen), v != "" })
	errorIds := lo.Uniq(lo.FilterMap(rep.ErrorIds, func(v string, _ int) (string, bool) {
		id := replayModel.NormalizeId(v)
		return id, id != ""
	}))

	return &replayModel.Segment{
		ProjectId:   item.ProjectId,
		ReplayId:    replayId,
		SegmentId:   *rep.SegmentId,
		Type:        lo.Ternary(rep.ReplayType == replayModel.TypeSession, replayModel.TypeSession, replayModel.TypeBuffer),
		Start:       replayModel.SecondsToTime(*rep.ReplayStartTimestamp),
		Timestamp:   replayModel.SecondsToTime(*rep.Timestamp),
		ReceivedAt:  item.ReceivedAt,
		Urls:        urls[:min(len(urls), maxUrls)],
		ErrorIds:    errorIds[:min(len(errorIds), maxErrorIds)],
		Environment: ev.Environment,
		Release:     ev.Release,
		UserId:      ev.User.Id,
		UserEmail:   ev.User.Email,
		UserIp:      ev.User.IpAddress,
		Browser:     ev.Browser,
		Os:          ev.Os,
		Device:      ev.Device,
		SdkName:     ev.SdkName,
		SdkVersion:  ev.SdkVersion,
		Size:        item.BodySize,
	}, nil
}

// retry повторяет f с нарастающей паузой, пока не получится (как у обработчика событий)
func (s *Service) retry(ctx context.Context, f func(context.Context) error) error {
	pause := s.conf.RetryMin
	for {
		attemptCtx, attemptCancel := context.WithTimeout(context.WithoutCancel(ctx), s.conf.StageTimeout)
		err := f(attemptCtx)
		attemptCancel()
		if err == nil {
			return nil
		}

		if ctx.Err() != nil {
			slog.Warn("replayproc: batch abandoned on shutdown", "error", err)
			return errors.Join(err, ctx.Err())
		}

		metricRetryTotal.Inc()
		slog.Error("replayproc: retry", "pause", pause, "error", err)

		sleep(ctx, pause)
		pause = min(pause*2, s.conf.RetryMax)
	}
}

func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// truncate — не длиннее n байт, не разрезая символ UTF-8
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}
