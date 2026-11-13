package service

import (
	"context"
	"errors"
	"maps"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// segRepoFake — только запросы дней сегментов; остальные методы RepoChI не вызываются
type segRepoFake struct {
	RepoChI

	days   map[int]time.Time
	err    error
	all    atomic.Int64 // запросов на всю запись
	single atomic.Int64 // точечных запросов
}

func (f *segRepoFake) SegmentsReceivedAt(_ context.Context, _ int64, _ string) (map[int]time.Time, error) {
	f.all.Add(1)
	// копия, как из базы: память не должна видеть более поздние правки f.days
	return maps.Clone(f.days), f.err
}

func (f *segRepoFake) SegmentReceivedAt(_ context.Context, _ int64, _ string, segmentId int) (time.Time, bool, error) {
	f.single.Add(1)
	at, ok := f.days[segmentId]
	return at, ok, f.err
}

func TestSegDays(t *testing.T) {
	ctx := context.Background()
	day := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)

	t.Run("запись читается одним запросом на все сегменты", func(t *testing.T) {
		repo := &segRepoFake{days: map[int]time.Time{0: day, 1: day, 2: day.AddDate(0, 0, 1)}}
		c := newSegDays(repo)

		var wg sync.WaitGroup
		for seg := range 3 {
			for range 5 {
				wg.Go(func() {
					at, found, err := c.receivedAt(ctx, 1, "r1", seg)
					assert.NoError(t, err)
					assert.True(t, found)
					assert.Equal(t, repo.days[seg], at)
				})
			}
		}
		wg.Wait()

		// одновременные первые запросы могли не слиться в один, но дальше — только из памяти
		before := repo.all.Load()
		assert.LessOrEqual(t, before, int64(15))
		_, found, err := c.receivedAt(ctx, 1, "r1", 2)
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, before, repo.all.Load())
		assert.Zero(t, repo.single.Load())
	})

	t.Run("сегмент, которого нет в памяти, спрашивается точечно", func(t *testing.T) {
		repo := &segRepoFake{days: map[int]time.Time{0: day}}
		c := newSegDays(repo)

		_, found, err := c.receivedAt(ctx, 1, "r1", 0)
		require.NoError(t, err)
		require.True(t, found)

		_, found, err = c.receivedAt(ctx, 1, "r1", 7)
		require.NoError(t, err)
		assert.False(t, found, "дырка в номерах")

		// запись ещё шла: новый сегмент появился после того, как её запомнили
		repo.days[8] = day
		at, found, err := c.receivedAt(ctx, 1, "r1", 8)
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, day, at)

		assert.Equal(t, int64(1), repo.all.Load())
		assert.Equal(t, int64(2), repo.single.Load())
	})

	t.Run("несуществующая запись и чужой проект не запоминаются", func(t *testing.T) {
		repo := &segRepoFake{days: map[int]time.Time{}}
		c := newSegDays(repo)

		for range 2 {
			_, found, err := c.receivedAt(ctx, 1, "none", 0)
			require.NoError(t, err)
			assert.False(t, found)
		}
		assert.Equal(t, int64(2), repo.all.Load())
		assert.Empty(t, c.items)
	})

	t.Run("устаревшая память перечитывается, переполненная — чистится", func(t *testing.T) {
		repo := &segRepoFake{days: map[int]time.Time{0: day}}
		c := newSegDays(repo)

		_, _, err := c.receivedAt(ctx, 1, "r1", 0)
		require.NoError(t, err)
		c.items[segDaysKey{projectId: 1, replayId: "r1"}] = segDaysItem{loadedAt: time.Now().Add(-segDaysTtl), days: repo.days}
		_, _, err = c.receivedAt(ctx, 1, "r1", 0)
		require.NoError(t, err)
		assert.Equal(t, int64(2), repo.all.Load())

		for i := range segDaysMax + 10 {
			c.put(segDaysKey{projectId: int64(i), replayId: "x"}, repo.days)
		}
		assert.LessOrEqual(t, len(c.items), segDaysMax)
	})

	t.Run("сбой базы не запоминается", func(t *testing.T) {
		repo := &segRepoFake{err: errors.New("boom")}
		c := newSegDays(repo)

		_, _, err := c.receivedAt(ctx, 1, "r1", 0)
		require.ErrorIs(t, err, repo.err)
		assert.Empty(t, c.items)
	})
}
