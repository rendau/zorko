package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rendau/zorko/internal/domain/chdisk/model"
)

type fakeChdisk struct {
	locked  bool
	disk    *model.Disk
	days    []*model.Day
	dropped []string
	dropErr error
}

func (f *fakeChdisk) Lock(context.Context) (func(), bool, error) {
	return func() {}, !f.locked, nil
}

func (f *fakeChdisk) Disk(context.Context) (*model.Disk, error) { return f.disk, nil }

func (f *fakeChdisk) Days(context.Context) ([]*model.Day, error) { return f.days, nil }

func (f *fakeChdisk) DropDay(_ context.Context, day *model.Day) error {
	if f.dropErr != nil {
		return f.dropErr
	}
	f.dropped = append(f.dropped, day.Day.Format(time.DateOnly))
	return nil
}

var now = time.Date(2026, 11, 6, 15, 0, 0, 0, time.UTC)

// дни с 1 по 6 ноября (6-е — сегодня), каждый по 10 байт
func days() []*model.Day {
	return lo.Map(lo.Range(6), func(i int, _ int) *model.Day {
		return &model.Day{Day: time.Date(2026, 11, 1+i, 0, 0, 0, 0, time.UTC), Bytes: 10}
	})
}

func run(t *testing.T, f *fakeChdisk) {
	t.Helper()
	s := New(Config{MaxUsedPercent: 80, Now: func() time.Time { return now }}, f)
	require.NoError(t, s.RunOnce(context.Background()))
}

func TestUnderLimitDropsNothing(t *testing.T) {
	f := &fakeChdisk{disk: &model.Disk{Total: 100, Free: 20}, days: days()}
	run(t, f)
	assert.Empty(t, f.dropped)
}

func TestOverLimitDropsOldestUntilCovered(t *testing.T) {
	// занято 95 при пределе 80: превышение 15 → два старых дня по 10
	f := &fakeChdisk{disk: &model.Disk{Total: 100, Free: 5}, days: days()}
	run(t, f)
	assert.Equal(t, []string{"2026-11-01", "2026-11-02"}, f.dropped)
}

func TestPendingCountsAsFreed(t *testing.T) {
	// прошлая проверка уже удалила дни: 15 байт ещё на диске, но неактивны
	f := &fakeChdisk{disk: &model.Disk{Total: 100, Free: 5, Pending: 15}, days: days()}
	run(t, f)
	assert.Empty(t, f.dropped)
}

func TestTodayIsNeverDropped(t *testing.T) {
	f := &fakeChdisk{disk: &model.Disk{Total: 1000, Free: 0}, days: days()}
	run(t, f)
	assert.Equal(t, []string{"2026-11-01", "2026-11-02", "2026-11-03", "2026-11-04", "2026-11-05"}, f.dropped)
}

func TestOtherReplicaHoldsLock(t *testing.T) {
	f := &fakeChdisk{locked: true, disk: &model.Disk{Total: 100, Free: 0}, days: days()}
	run(t, f)
	assert.Empty(t, f.dropped)
}

func TestDropErrorStops(t *testing.T) {
	f := &fakeChdisk{disk: &model.Disk{Total: 100, Free: 0}, days: days(), dropErr: errors.New("boom")}
	s := New(Config{MaxUsedPercent: 80, Now: func() time.Time { return now }}, f)
	require.Error(t, s.RunOnce(context.Background()))
}

func TestDiskUsed(t *testing.T) {
	assert.Equal(t, uint64(80), (&model.Disk{Total: 100, Free: 10, Pending: 10}).Used())
	assert.Equal(t, uint64(0), (&model.Disk{Total: 100, Free: 120}).Used(), "свободно больше общего — не уходит в минус")
	assert.Equal(t, uint64(0), (&model.Disk{Total: 100, Free: 50, Pending: 70}).Used())
	assert.InDelta(t, 80.0, (&model.Disk{Total: 100, Free: 20}).UsedPercent(), 0.001)
	assert.Zero(t, (&model.Disk{}).UsedPercent())
}
