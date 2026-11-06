// Package service — диск ClickHouse: сколько занято и какие дни данных zorko на нём (ADR-017)
package service

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/samber/lo"

	"github.com/rendau/zorko/internal/domain/chdisk/model"
)

const lockKey = 7302003 // zorko: предохранитель диска ClickHouse

const partitionLayout = "20060102" // partition by toYYYYMMDD(...)

// Tables — таблицы zorko с партициями по дням (migrations-ch); другие таблицы базы не трогаются
var Tables = []string{"event_stat", "event", "replay_segment", "replay_issue"}

type Service struct {
	repoCh RepoChI
	repoDb RepoDbI
	tables []string
}

func New(repoCh RepoChI, repoDb RepoDbI, tables []string) *Service {
	return &Service{repoCh: repoCh, repoDb: repoDb, tables: tables}
}

// Lock — одна проверка на все копии; ok=false — проверяет другая копия
func (s *Service) Lock(ctx context.Context) (func(), bool, error) {
	unlock, ok, err := s.repoDb.TryLock(ctx, lockKey)
	if err != nil {
		return nil, false, fmt.Errorf("repoDb.TryLock: %w", err)
	}
	return unlock, ok, nil
}

func (s *Service) Disk(ctx context.Context) (*model.Disk, error) {
	d, err := s.repoCh.Disk(ctx)
	if err != nil {
		return nil, fmt.Errorf("repoCh.Disk: %w", err)
	}
	return d, nil
}

// Days — дни данных zorko, старые первыми; партиции не по дню пропускаются
func (s *Service) Days(ctx context.Context) ([]*model.Day, error) {
	parts, err := s.repoCh.Partitions(ctx, s.tables)
	if err != nil {
		return nil, fmt.Errorf("repoCh.Partitions: %w", err)
	}

	byDay := map[time.Time]*model.Day{}
	for _, p := range parts {
		day, err := time.Parse(partitionLayout, p.Id)
		if err != nil || !slices.Contains(s.tables, p.Table) {
			continue
		}
		d := byDay[day]
		if d == nil {
			d = &model.Day{Day: day}
			byDay[day] = d
		}
		d.Bytes += p.Bytes
		d.Partitions = append(d.Partitions, p)
	}

	days := lo.Values(byDay)
	slices.SortFunc(days, func(a, b *model.Day) int { return a.Day.Compare(b.Day) })
	for _, d := range days {
		slices.SortFunc(d.Partitions, func(a, b *model.Partition) int { return cmp.Compare(a.Table, b.Table) })
	}
	return days, nil
}

// DropDay удаляет день во всех таблицах zorko; удаление партиции мгновенное, место
// освобождается через несколько минут (неактивные куски)
func (s *Service) DropDay(ctx context.Context, day *model.Day) error {
	for _, p := range day.Partitions {
		// имя и номер — только из своего списка и даты: в DDL они попадают как текст
		if !slices.Contains(s.tables, p.Table) {
			return fmt.Errorf("table %q is not a zorko table", p.Table)
		}
		if _, err := time.Parse(partitionLayout, p.Id); err != nil || len(p.Id) != len(partitionLayout) {
			return fmt.Errorf("partition %q is not a day", p.Id)
		}
		if err := s.repoCh.DropPartition(ctx, p.Table, p.Id); err != nil {
			return fmt.Errorf("repoCh.DropPartition: %w", err)
		}
	}
	return nil
}
