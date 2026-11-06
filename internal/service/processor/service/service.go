package service

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/samber/lo"

	eventModel "github.com/rendau/zorko/internal/domain/event/model"
	issueModel "github.com/rendau/zorko/internal/domain/issue/model"
	replayModel "github.com/rendau/zorko/internal/domain/replay/model"
	queueModel "github.com/rendau/zorko/internal/service/queue/model"
)

type Config struct {
	BatchSize      int
	StorePerMinute int64
	// пауза между повторами после сбоя базы: от RetryMin, удваивается до RetryMax
	RetryMin time.Duration
	RetryMax time.Duration
	// StageTimeout — предел одной попытки записи: «тихо» пропавшая база (соединение висит, ошибки нет)
	// не должна останавливать обработку навсегда
	StageTimeout time.Duration
}

type Service struct {
	conf         Config
	queue        queueI
	normalizer   normalizerI
	symbolicator symbolicatorI // nil — карты исходников выключены
	grouper      grouperI
	issueService issueServiceI
	eventService eventServiceI
	replayLinker ReplayLinker // nil — записи сессий выключены
	projectLabel projectLabelI

	wg sync.WaitGroup
}

func New(conf Config, queue queueI, normalizer normalizerI, symbolicator symbolicatorI, grouper grouperI,
	issueService issueServiceI, eventService eventServiceI, replayLinker ReplayLinker, projectLabel projectLabelI) *Service {
	return &Service{
		conf:         conf,
		queue:        queue,
		normalizer:   normalizer,
		symbolicator: symbolicator,
		grouper:      grouper,
		issueService: issueService,
		eventService: eventService,
		replayLinker: replayLinker,
		projectLabel: projectLabel,
	}
}

// Start запускает цикл обработки до отмены ctx. Начатая пачка дописывается и подтверждается и
// после отмены; пачка, которая при остановке упирается в сбой базы, бросается — её получит другая копия.
func (s *Service) Start(ctx context.Context) {
	s.wg.Go(func() {
		for ctx.Err() == nil {
			events, err := s.queue.PollEvents(ctx, s.conf.BatchSize)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				slog.Error("processor: queue.PollEvents", "error", err)
				sleep(ctx, s.conf.RetryMin)
				continue
			}

			if !s.processBatch(ctx, events) {
				return
			}

			commitErr := s.retry(ctx, stageCommit, func(c context.Context) error { return s.queue.CommitPolled(c) })
			if commitErr != nil {
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

	start := time.Now()
	events := s.normalizeBatch(batch)
	metricBatchDuration.WithLabelValues(stageNormalize).Observe(time.Since(start).Seconds())
	if len(events) == 0 {
		return true
	}

	// перевод стека по картам — до отпечатка: склейка идёт по исходным именам (ADR-008).
	// Не обязателен: при сбое кадры остаются как есть, пачка идёт дальше.
	if s.symbolicator != nil {
		start = time.Now()
		s.symbolicator.Symbolicate(context.WithoutCancel(ctx), events)
		metricBatchDuration.WithLabelValues(stageSymbolicate).Observe(time.Since(start).Seconds())
	}
	for _, ev := range events {
		ev.Grouping = s.grouper.Group(ev)
	}

	// одинаковые отпечатки пачки → одна прибавка к проблеме
	groups := lo.GroupBy(events, func(ev *eventModel.Event) issueModel.HashKey {
		return issueModel.HashKey{ProjectId: ev.ProjectId, Hash: ev.Grouping.Hash}
	})
	keys := lo.Uniq(lo.Map(events, func(ev *eventModel.Event, _ int) issueModel.HashKey {
		return issueModel.HashKey{ProjectId: ev.ProjectId, Hash: ev.Grouping.Hash}
	}))
	hits := lo.Map(keys, func(k issueModel.HashKey, _ int) *issueModel.Hit { return buildHit(groups[k]) })

	minute := time.Now().UTC().Truncate(time.Minute)

	var results []*issueModel.HitResult
	start = time.Now()
	err := s.retry(ctx, stagePg, func(c context.Context) error {
		var err error
		results, err = s.issueService.Record(c, hits, s.conf.StorePerMinute, minute)
		return err
	})
	if err != nil {
		return false
	}
	metricBatchDuration.WithLabelValues(stagePg).Observe(time.Since(start).Seconds())

	all := make([]*eventModel.Event, 0, len(events))
	full := make([]*eventModel.Event, 0, len(events))
	var countedOnly, projectDeleted []*eventModel.Event
	for i, k := range keys {
		res := results[i]
		groupEvents := groups[k]

		if res.IssueId == 0 {
			projectDeleted = append(projectDeleted, groupEvents...)
			continue
		}

		for j, ev := range groupEvents {
			ev.IssueId = res.IssueId
			all = append(all, ev)
			if int64(j) < res.StoreAllowed {
				full = append(full, ev)
			} else {
				countedOnly = append(countedOnly, ev)
			}
		}
	}
	s.countEvents(resultProjectDeleted, projectDeleted)

	// связи с записями сессий — для всех ошибок, а не только сохранённых полностью (ADR-014)
	links := replayLinks(all)

	start = time.Now()
	// при сбое ClickHouse повторяется только запись в него: счётчики в Postgres уже прибавлены.
	// Повтор целиком безопасен: таблицы ReplacingMergeTree схлопывают одинаковые строки.
	err = s.retry(ctx, stageCh, func(c context.Context) error {
		if err := s.eventService.Store(c, all, full); err != nil {
			return err
		}
		if s.replayLinker != nil && len(links) > 0 {
			return s.replayLinker.InsertLinks(c, links)
		}
		return nil
	})
	if err != nil {
		return false
	}
	metricBatchDuration.WithLabelValues(stageCh).Observe(time.Since(start).Seconds())

	s.countEvents(resultStored, full)
	s.countEvents(resultCountedOnly, countedOnly)

	oldest := lo.MinBy(batch, func(a, b *queueModel.Event) bool { return a.ReceivedAt.Before(b.ReceivedAt) })
	metricLag.Set(time.Since(oldest.ReceivedAt).Seconds())

	return true
}

// replayLinks — ошибки с тегом replayId (SDK ставит его, если запись идёт или будет отправлена из-за ошибки)
func replayLinks(events []*eventModel.Event) []*replayModel.Link {
	return lo.FilterMap(events, func(ev *eventModel.Event, _ int) (*replayModel.Link, bool) {
		replayId := replayModel.NormalizeId(ev.Tags[replayTag])
		if replayId == "" {
			return nil, false
		}
		return &replayModel.Link{
			ProjectId: ev.ProjectId,
			IssueId:   ev.IssueId,
			ReplayId:  replayId,
			EventId:   ev.EventId,
			Timestamp: ev.Timestamp,
		}, true
	})
}

// replayTag — тег ошибки с номером записи сессии
const replayTag = "replayId"

// countEvents прибавляет события к счётчику исхода по проектам
func (s *Service) countEvents(result string, events []*eventModel.Event) {
	byProject := lo.CountValuesBy(events, func(ev *eventModel.Event) string { return s.projectLabel.Label(ev.ProjectId) })
	for project, n := range byProject {
		metricEventTotal.WithLabelValues(result, project).Add(float64(n))
	}
}

func (s *Service) normalizeBatch(batch []*queueModel.Event) []*eventModel.Event {
	events := make([]*eventModel.Event, 0, len(batch))
	for _, item := range batch {
		if item.ProjectId == 0 {
			metricEventTotal.WithLabelValues(resultBadMessage, s.projectLabel.Label(item.ProjectId)).Inc()
			slog.Warn("processor: message without project_id", "event_id", item.EventId)
			continue
		}

		ev, err := s.normalizer.Normalize(&eventModel.RawEvent{
			EventId:    item.EventId,
			ProjectId:  item.ProjectId,
			ReceivedAt: item.ReceivedAt,
			ClientIp:   item.ClientIp,
			Payload:    item.Payload,
		})
		if err != nil {
			metricEventTotal.WithLabelValues(resultBadMessage, s.projectLabel.Label(item.ProjectId)).Inc()
			slog.Warn("processor: bad event", "error", err, "event_id", item.EventId, "project_id", item.ProjectId)
			continue
		}

		events = append(events, ev)
	}
	return events
}

// buildHit — прибавка по событиям одного отпечатка; данные новой проблемы — по первому событию
func buildHit(events []*eventModel.Event) *issueModel.Hit {
	first := events[0]
	earliest := lo.MinBy(events, func(a, b *eventModel.Event) bool { return a.Timestamp.Before(b.Timestamp) })
	latest := lo.MaxBy(events, func(a, b *eventModel.Event) bool { return a.Timestamp.After(b.Timestamp) })

	return &issueModel.Hit{
		ProjectId:    first.ProjectId,
		Hash:         first.Grouping.Hash,
		HashVersion:  first.Grouping.Version,
		Title:        first.Grouping.Title,
		Culprit:      first.Grouping.Culprit,
		Type:         first.Grouping.Type,
		Value:        first.Grouping.Value,
		Level:        first.Level,
		Platform:     first.Platform,
		Count:        int64(len(events)),
		FirstSeen:    earliest.Timestamp,
		LastSeen:     latest.Timestamp,
		FirstRelease: earliest.Release,
		LastRelease:  latest.Release,
	}
}

// retry повторяет f с нарастающей паузой, пока не получится. Сама f выполняется без отмены
// (начатая запись дописывается при остановке), а после ошибки при остановке повторы прекращаются.
func (s *Service) retry(ctx context.Context, stage string, f func(context.Context) error) error {
	pause := s.conf.RetryMin
	for {
		attemptCtx, attemptCancel := context.WithTimeout(context.WithoutCancel(ctx), s.conf.StageTimeout)
		err := f(attemptCtx)
		attemptCancel()
		if err == nil {
			return nil
		}

		if ctx.Err() != nil {
			slog.Warn("processor: batch abandoned on shutdown", "stage", stage, "error", err)
			return errors.Join(err, ctx.Err())
		}

		metricRetryTotal.WithLabelValues(stage).Inc()
		slog.Error("processor: retry", "stage", stage, "pause", pause, "error", err)

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
