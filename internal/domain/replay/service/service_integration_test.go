//go:build integration

package service_test

import (
	"bytes"
	"compress/zlib"
	"context"
	"io"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rendau/zorko/internal/domain/replay/model"
	"github.com/rendau/zorko/internal/domain/replay/repo/blob"
	"github.com/rendau/zorko/internal/domain/replay/repo/ch"
	"github.com/rendau/zorko/internal/domain/replay/repo/db"
	"github.com/rendau/zorko/internal/domain/replay/service"
	"github.com/rendau/zorko/internal/errs"
	"github.com/rendau/zorko/internal/testutil"
)

func randomId() string {
	const hex = "0123456789abcdef"
	b := make([]byte, 32)
	for i := range b {
		b[i] = hex[rand.IntN(16)]
	}
	return string(b)
}

func TestReplayStorage(t *testing.T) {
	ctx := context.Background()
	pool := testutil.PgPool(t)
	s3c := testutil.S3(t)
	svc := service.New(ch.New(testutil.ChConn(t)), blob.New(s3c), db.New(pool))

	// проект — случайный номер: в ClickHouse нет внешних ключей, тесты не мешают друг другу
	projectId := rand.Int64N(1<<40) + 1_000_000
	now := time.Now().UTC().Truncate(time.Millisecond)
	start := now.Add(-2 * time.Minute)

	withErrors, noErrors, noZero := randomId(), randomId(), randomId()
	errorId := randomId()

	seg := func(replayId string, n int, urls []string, errorIds []string, user string) *model.Segment {
		return &model.Segment{
			ProjectId: projectId, ReplayId: replayId, SegmentId: n, Type: model.TypeBuffer,
			Start: start, Timestamp: start.Add(time.Duration(n*5) * time.Second), ReceivedAt: now,
			Urls: urls, ErrorIds: errorIds, Environment: "production", Release: "seller@1.0",
			UserEmail: user, Browser: "Chrome 130.0", Os: "macOS 15.1", SdkName: "sentry.javascript.vue", SdkVersion: "11.11.0",
		}
	}

	// тело сегмента 0: голый JSON — хранилище сожмёт
	raw := []byte(`[{"type":4,"data":{"href":"https://seller.mechta.kz/orders"},"timestamp":1}]`)
	s0 := seg(withErrors, 0, []string{"https://seller.mechta.kz/orders"}, nil, "")
	size, err := svc.PutSegmentBody(ctx, s0, raw, false)
	require.NoError(t, err)
	s0.Size = size

	require.NoError(t, svc.InsertSegments(ctx, []*model.Segment{
		s0,
		seg(withErrors, 1, []string{"https://seller.mechta.kz/orders/42"}, []string{errorId}, "oleg@mechta.kz"),
		seg(withErrors, 1, []string{"https://seller.mechta.kz/orders/42"}, []string{errorId}, "oleg@mechta.kz"), // повтор SDK
		seg(noErrors, 0, []string{"https://seller.mechta.kz/"}, nil, ""),
		seg(noZero, 3, []string{"https://seller.mechta.kz/x"}, nil, ""), // без нулевого — не показывается
	}))
	require.NoError(t, svc.InsertLinks(ctx, []*model.Link{
		{ProjectId: projectId, IssueId: 77, ReplayId: withErrors, EventId: errorId, Timestamp: start.Add(7 * time.Second)},
	}))

	t.Run("список", func(t *testing.T) {
		items, total, err := svc.List(ctx, &model.ListReq{ProjectIds: []int64{projectId}})
		require.NoError(t, err)
		assert.EqualValues(t, 2, total, "запись без нулевого сегмента не показывается")
		require.Len(t, items, 2)

		byId := map[string]*model.Summary{items[0].ReplayId: items[0], items[1].ReplayId: items[1]}
		s := byId[withErrors]
		require.NotNil(t, s)
		assert.Equal(t, 2, s.Segments, "повтор сегмента схлопнут")
		assert.Equal(t, 1, s.MaxSegment)
		assert.Equal(t, 1, s.ErrorCount)
		assert.Equal(t, "https://seller.mechta.kz/orders", s.Url, "первый адрес — из нулевого сегмента")
		assert.Equal(t, "oleg@mechta.kz", s.UserEmail, "пользователь вошёл посреди записи")
		assert.Equal(t, start, s.Start.UTC())
		assert.Equal(t, start.Add(5*time.Second), s.End.UTC())
		assert.Equal(t, "Chrome 130.0", s.Browser)
	})

	t.Run("фильтры", func(t *testing.T) {
		yes, no := true, false
		items, _, err := svc.List(ctx, &model.ListReq{ProjectIds: []int64{projectId}, HasErrors: &yes})
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, withErrors, items[0].ReplayId)

		items, _, err = svc.List(ctx, &model.ListReq{ProjectIds: []int64{projectId}, HasErrors: &no})
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, noErrors, items[0].ReplayId)

		q := "orders/42"
		items, _, err = svc.List(ctx, &model.ListReq{ProjectIds: []int64{projectId}, Query: &q})
		require.NoError(t, err)
		require.Len(t, items, 1, "адрес из любого сегмента")
		assert.Equal(t, withErrors, items[0].ReplayId)
		assert.Equal(t, 2, items[0].Segments, "сводка — по всем сегментам, а не только совпавшим")

		items, _, err = svc.List(ctx, &model.ListReq{ProjectIds: []int64{projectId + 1}})
		require.NoError(t, err)
		assert.Empty(t, items, "чужой проект")
	})

	t.Run("сводка и связи", func(t *testing.T) {
		s, err := svc.Get(ctx, projectId, withErrors)
		require.NoError(t, err)
		assert.Equal(t, withErrors, s.ReplayId)

		_, err = svc.Get(ctx, projectId, noZero)
		assert.ErrorIs(t, err, errs.ReplayNotFound)
		_, err = svc.Get(ctx, projectId+1, withErrors)
		assert.ErrorIs(t, err, errs.ReplayNotFound, "номер записи без проекта доступа не даёт")

		links, err := svc.Links(ctx, projectId, withErrors)
		require.NoError(t, err)
		require.Len(t, links, 1)
		assert.EqualValues(t, 77, links[0].IssueId)
		assert.Equal(t, errorId, links[0].EventId)
	})

	t.Run("записи проблемы", func(t *testing.T) {
		items, total, err := svc.IssueReplays(ctx, projectId, 77, 0, 20)
		require.NoError(t, err)
		assert.EqualValues(t, 1, total)
		require.Len(t, items, 1)
		assert.Equal(t, withErrors, items[0].ReplayId)
		require.NotNil(t, items[0].ErrorAt)
		assert.Equal(t, start.Add(7*time.Second), items[0].ErrorAt.UTC())

		items, total, err = svc.IssueReplays(ctx, projectId, 78, 0, 20)
		require.NoError(t, err)
		assert.Zero(t, total)
		assert.Empty(t, items)
	})

	t.Run("тело сегмента", func(t *testing.T) {
		body, err := svc.SegmentBody(ctx, projectId, withErrors, 0, 1<<20)
		require.NoError(t, err)
		assert.Equal(t, byte(0x78), body[0], "хранится zlib")
		zr, err := zlib.NewReader(bytes.NewReader(body))
		require.NoError(t, err)
		plain, err := io.ReadAll(zr)
		require.NoError(t, err)
		assert.Equal(t, raw, plain)

		_, err = svc.SegmentBody(ctx, projectId, withErrors, 1, 1<<20)
		assert.ErrorIs(t, err, errs.ReplayNotFound, "описание есть, тела в S3 нет")
		_, err = svc.SegmentBody(ctx, projectId+1, withErrors, 0, 1<<20)
		assert.ErrorIs(t, err, errs.ReplayNotFound, "чужой проект")
	})
}

func TestReplayGcDays(t *testing.T) {
	ctx := context.Background()
	pool := testutil.PgPool(t)
	s3c := testutil.S3(t)
	svc := service.New(ch.New(testutil.ChConn(t)), blob.New(s3c), db.New(pool))

	old := time.Date(2001, 1, 2, 23, 0, 0, 0, time.UTC)
	fresh := time.Date(2001, 1, 3, 1, 0, 0, 0, time.UTC)
	for _, at := range []time.Time{old, fresh} {
		_, err := svc.PutSegmentBody(ctx, &model.Segment{ProjectId: 1, ReplayId: randomId(), ReceivedAt: at}, []byte(`[]`), false)
		require.NoError(t, err)
	}

	unlock, ok, err := svc.GcLock(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	_, ok2, err := svc.GcLock(ctx)
	require.NoError(t, err)
	assert.False(t, ok2, "вторая копия ждёт")
	unlock()

	// граница внутри 3 января: 2 января закончился целиком, 3-е — нет
	n, err := svc.DeleteOldDays(ctx, time.Date(2001, 1, 3, 12, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n, 1)

	left, err := s3c.ListPrefixes(ctx, model.KeyPrefix)
	require.NoError(t, err)
	assert.NotContains(t, left, "replay/20010102/")
	assert.Contains(t, left, "replay/20010103/")

	_, err = svc.DeleteOldDays(ctx, time.Date(2001, 1, 4, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
}
