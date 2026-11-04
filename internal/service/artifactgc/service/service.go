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

type artifactServiceI interface {
	GcLock(ctx context.Context) (func(), bool, error)
	DeleteStaleBundles(ctx context.Context, retention time.Duration) (int64, error)
	DeleteOrphanFiles(ctx context.Context) (int, error)
	DeleteOldChunks(ctx context.Context, before time.Time) (int, error)
	DeleteOldSiteFetch(ctx context.Context, olderThan time.Duration) (int64, error)
}

type Config struct {
	Retention  time.Duration // архив без ошибок по нему дольше этого удаляется
	Every      time.Duration
	FirstDelay time.Duration
}

var metricGcDeleted = metrics.Factory.NewCounterVec(prometheus.CounterOpts{
	Name: "sourcemap_gc_deleted_total",
}, []string{"kind"})

type Service struct {
	conf     Config
	artifact artifactServiceI
	wg       sync.WaitGroup
}

func New(conf Config, artifact artifactServiceI) *Service {
	if conf.Retention == 0 {
		conf.Retention = 30 * 24 * time.Hour
	}
	if conf.Every == 0 {
		conf.Every = 24 * time.Hour
	}
	if conf.FirstDelay == 0 {
		conf.FirstDelay = 5 * time.Minute
	}
	return &Service{conf: conf, artifact: artifact}
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
				slog.Error("artifactgc: failed", "error", err)
			}
			timer.Reset(s.conf.Every)
		}
	})
}

func (s *Service) Wait() {
	s.wg.Wait()
}

// RunOnce — одна уборка; другая копия уже убирает — пропуск
func (s *Service) RunOnce(ctx context.Context) error {
	unlock, ok, err := s.artifact.GcLock(ctx)
	if err != nil {
		return fmt.Errorf("artifact.GcLock: %w", err)
	}
	if !ok {
		slog.Info("artifactgc: another replica is collecting")
		return nil
	}
	defer unlock()

	start := time.Now()
	bundles, err := s.artifact.DeleteStaleBundles(ctx, s.conf.Retention)
	metricGcDeleted.WithLabelValues("bundle").Add(float64(bundles))
	if err != nil {
		return fmt.Errorf("artifact.DeleteStaleBundles: %w", err)
	}
	files, err := s.artifact.DeleteOrphanFiles(ctx)
	metricGcDeleted.WithLabelValues("file").Add(float64(files))
	if err != nil {
		return fmt.Errorf("artifact.DeleteOrphanFiles: %w", err)
	}
	chunks, err := s.artifact.DeleteOldChunks(ctx, time.Now().Add(-24*time.Hour))
	metricGcDeleted.WithLabelValues("chunk").Add(float64(chunks))
	if err != nil {
		return fmt.Errorf("artifact.DeleteOldChunks: %w", err)
	}
	sites, err := s.artifact.DeleteOldSiteFetch(ctx, s.conf.Retention)
	metricGcDeleted.WithLabelValues("site_fetch").Add(float64(sites))
	if err != nil {
		return fmt.Errorf("artifact.DeleteOldSiteFetch: %w", err)
	}
	slog.Info("artifactgc: done", "bundles", bundles, "files", files, "chunks", chunks, "site_fetch", sites,
		"duration", time.Since(start).String())
	return nil
}
