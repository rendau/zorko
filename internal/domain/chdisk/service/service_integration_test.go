//go:build integration

package service_test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rendau/zorko/internal/domain/chdisk/model"
	"github.com/rendau/zorko/internal/domain/chdisk/repo/ch"
	"github.com/rendau/zorko/internal/domain/chdisk/repo/db"
	"github.com/rendau/zorko/internal/domain/chdisk/service"
	"github.com/rendau/zorko/internal/testutil"
)

// Стенд: scripts/stand.sh up. Своя таблица с партициями по дням — данные zorko на стенде не трогаются.
func TestDaysAndDropDay(t *testing.T) {
	ctx := context.Background()
	con := testutil.ChConn(t)
	table := fmt.Sprintf("chdisk_test_%d", rand.Int64N(1<<40))

	require.NoError(t, con.Exec(ctx, `create table `+table+` (d DateTime64(3, 'UTC'), v String)
		engine = MergeTree partition by toYYYYMMDD(d) order by d`))
	t.Cleanup(func() { _ = con.Exec(context.Background(), `drop table if exists `+table) })

	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	for i := range 3 {
		require.NoError(t, con.Exec(ctx, `insert into `+table+` values (?, ?)`, base.AddDate(0, 0, i), "x"))
	}

	svc := service.New(ch.New(con), db.New(testutil.PgPool(t)), []string{table})

	disk, err := svc.Disk(ctx)
	require.NoError(t, err)
	assert.Positive(t, disk.Total)
	assert.LessOrEqual(t, disk.Used(), disk.Total)

	days, err := svc.Days(ctx)
	require.NoError(t, err)
	require.Len(t, days, 3)
	assert.Equal(t, []string{"2026-10-01", "2026-10-02", "2026-10-03"},
		lo.Map(days, func(d *model.Day, _ int) string { return d.Day.Format(time.DateOnly) }))
	assert.Positive(t, days[0].Bytes)
	assert.Equal(t, table, days[0].Partitions[0].Table)

	require.NoError(t, svc.DropDay(ctx, days[0]))

	days, err = svc.Days(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"2026-10-02", "2026-10-03"},
		lo.Map(days, func(d *model.Day, _ int) string { return d.Day.Format(time.DateOnly) }))

	// чужая таблица и не дата в номере партиции не доходят до DDL
	require.Error(t, svc.DropDay(ctx, &model.Day{Partitions: []*model.Partition{{Table: "usr", Id: "20261001"}}}))
	require.Error(t, svc.DropDay(ctx, &model.Day{Partitions: []*model.Partition{{Table: table, Id: "1' or 1"}}}))

	unlock, ok, err := svc.Lock(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	_, ok2, err := service.New(ch.New(con), db.New(testutil.PgPool(t)), nil).Lock(ctx)
	require.NoError(t, err)
	assert.False(t, ok2, "замок у первой копии")
	unlock()
}
