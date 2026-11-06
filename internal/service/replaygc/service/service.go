package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/rendau/zorko/internal/infra/metrics"
)

type replayServiceI interface {
	GcLock(ctx context.Context) (func(), bool, error)
	DeleteOldDays(ctx context.Context, before time.Time) (int, error)
}

type Config struct {
	Retention  time.Duration // сколько хранить записи; TTL ClickHouse — 30 дней в миграции
	Every      time.Duration
	FirstDelay time.Duration
	Now        func() time.Time // для тестов
}

var metricGcDeleted = metrics.Factory.NewCounter(prometheus.CounterOpts{
	Name: "replay_gc_deleted_total",
})

type Service struct {
	conf   Config
	replay replayServiceI
	wg     sync.WaitGroup
}

func New(conf Config, replay replayServiceI) *Service {
	if conf.Retention == 0 {
		conf.Retention = 30 * 24 * time.Hour
	}
	if conf.Every == 0 {
		conf.Every = 24 * time.Hour
	}
	if conf.FirstDelay == 0 {
		conf.FirstDelay = 5 * time.Minute
	}
	if conf.Now == nil {
		conf.Now = time.Now
	}
	return &Service{conf: conf, replay: replay}
}

func (s *Service) Start(ctx context.Context) {
	s.wg.Go(func() {
		timer := time.NewTimer(s.conf.FirstDelay)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			if err := s.RunOnce(ctx); err != nil && ctx.Err() == nil {
				slog.Error("replaygc: failed", "error", err)
			}
			timer.Reset(s.conf.Every)
		}
	})
}

func (s *Service) Wait() {
	s.wg.Wait()
}

// RunOnce — одна уборка; другая копия уже убирает — пропуск.
// Удаляются дни приёма, целиком закончившиеся раньше «сейчас − срок − сутки»: запас на то, что
// ClickHouse убирает свои строки по TTL позже (раз в несколько часов), а интерфейс не должен
// показывать запись без тел.
func (s *Service) RunOnce(ctx context.Context) error {
	unlock, ok, err := s.replay.GcLock(ctx)
	if err != nil {
		return fmt.Errorf("replay.GcLock: %w", err)
	}
	if !ok {
		slog.Info("replaygc: another replica is collecting")
		return nil
	}
	defer unlock()

	start := time.Now()
	n, err := s.replay.DeleteOldDays(ctx, s.conf.Now().UTC().Add(-s.conf.Retention-24*time.Hour))
	metricGcDeleted.Add(float64(n))
	if err != nil {
		return fmt.Errorf("replay.DeleteOldDays: %w", err)
	}
	slog.Info("replaygc: done", "segments", n, "duration", time.Since(start).String())
	return nil
}
