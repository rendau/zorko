// Package service — предохранитель диска ClickHouse (ADR-017): данные хранятся 30 дней (TTL), но
// если диск занят больше предела раньше — удаляются самые старые дни, пока не станет ниже.
package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/rendau/zorko/internal/domain/chdisk/model"
	"github.com/rendau/zorko/internal/infra/metrics"
)

type chdiskServiceI interface {
	Lock(ctx context.Context) (func(), bool, error)
	Disk(ctx context.Context) (*model.Disk, error)
	Days(ctx context.Context) ([]*model.Day, error)
	DropDay(ctx context.Context, day *model.Day) error
}

type Config struct {
	MaxUsedPercent int // предел занятого места на диске, %
	Every          time.Duration
	FirstDelay     time.Duration
	Now            func() time.Time // для тестов
}

var (
	metricUsedPercent = metrics.Factory.NewGauge(prometheus.GaugeOpts{
		Name: "chdisk_used_percent",
		Help: "Занято на диске ClickHouse, % (без уже освобождаемого)",
	})
	metricKeptDays = metrics.Factory.NewGauge(prometheus.GaugeOpts{
		Name: "chdisk_kept_days",
		Help: "Сколько дней данных zorko сейчас хранится в ClickHouse",
	})
	metricDroppedDays = metrics.Factory.NewCounter(prometheus.CounterOpts{
		Name: "chdisk_dropped_days_total",
		Help: "Дни, удалённые раньше срока из-за предела диска",
	})
)

type Service struct {
	conf   Config
	chdisk chdiskServiceI
	wg     sync.WaitGroup
}

func New(conf Config, chdisk chdiskServiceI) *Service {
	if conf.MaxUsedPercent <= 0 || conf.MaxUsedPercent > 100 {
		conf.MaxUsedPercent = 80
	}
	if conf.Every == 0 {
		conf.Every = 10 * time.Minute
	}
	if conf.FirstDelay == 0 {
		conf.FirstDelay = time.Minute
	}
	if conf.Now == nil {
		conf.Now = time.Now
	}
	return &Service{conf: conf, chdisk: chdisk}
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
				slog.Error("chdiskgc: failed", "error", err)
			}
			timer.Reset(s.conf.Every)
		}
	})
}

func (s *Service) Wait() {
	s.wg.Wait()
}

// RunOnce — одна проверка; другая копия уже проверяет — пропуск.
// Удаляются дни строго раньше сегодняшнего (UTC), от старых к новым, пока их объём не покроет
// превышение. Место освобождается через несколько минут — следующая проверка видит его в Pending.
func (s *Service) RunOnce(ctx context.Context) error {
	unlock, ok, err := s.chdisk.Lock(ctx)
	if err != nil {
		return fmt.Errorf("chdisk.Lock: %w", err)
	}
	if !ok {
		return nil
	}
	defer unlock()

	disk, err := s.chdisk.Disk(ctx)
	if err != nil {
		return fmt.Errorf("chdisk.Disk: %w", err)
	}
	days, err := s.chdisk.Days(ctx)
	if err != nil {
		return fmt.Errorf("chdisk.Days: %w", err)
	}

	used := disk.UsedPercent()
	metricUsedPercent.Set(used)
	today := s.conf.Now().UTC().Truncate(24 * time.Hour)
	setKeptDays(days, today)

	limit := disk.Total * uint64(s.conf.MaxUsedPercent) / 100
	if disk.Used() <= limit {
		return nil
	}
	over := disk.Used() - limit

	var freed uint64
	dropped := 0
	for _, day := range days {
		if freed >= over || !day.Day.Before(today) {
			break
		}
		if err = s.chdisk.DropDay(ctx, day); err != nil {
			return fmt.Errorf("chdisk.DropDay(%s): %w", day.Day.Format(time.DateOnly), err)
		}
		freed += day.Bytes
		dropped++
		metricDroppedDays.Inc()
		slog.Warn("chdiskgc: disk over limit, day dropped before retention",
			"day", day.Day.Format(time.DateOnly), "bytes", day.Bytes,
			"used_percent", fmt.Sprintf("%.1f", used), "limit_percent", s.conf.MaxUsedPercent)
	}
	setKeptDays(days[dropped:], today)

	if freed < over {
		// остальное место — не старые дни zorko: сегодняшний день или чужие таблицы (журналы ClickHouse)
		slog.Error("chdiskgc: disk over limit, nothing more to drop",
			"used_percent", fmt.Sprintf("%.1f", used), "limit_percent", s.conf.MaxUsedPercent,
			"over_bytes", over-freed, "dropped_days", dropped)
	}
	return nil
}

// setKeptDays — дней от самого старого хранимого до сегодняшнего включительно
func setKeptDays(days []*model.Day, today time.Time) {
	if len(days) == 0 {
		metricKeptDays.Set(0)
		return
	}
	metricKeptDays.Set(float64(today.Sub(days[0].Day)/(24*time.Hour)) + 1)
}
