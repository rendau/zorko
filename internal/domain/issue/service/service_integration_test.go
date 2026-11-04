//go:build integration

package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rendau/zorko/internal/domain/issue/model"
	repoDbP "github.com/rendau/zorko/internal/domain/issue/repo/db"
	"github.com/rendau/zorko/internal/testutil"
)

func hit(projectId int64, hash string, count int64, at time.Time, release string) *model.Hit {
	return &model.Hit{
		ProjectId: projectId, Hash: hash, HashVersion: "v1",
		Title: "TypeError: x", Culprit: "cart.js в addItem", Type: "TypeError", Value: "x", Level: "error", Platform: "javascript",
		Count: count, FirstSeen: at, LastSeen: at, FirstRelease: release, LastRelease: release,
	}
}

func TestRecordCountsAndAllowance(t *testing.T) {
	pool := testutil.PgPool(t)
	projectId := testutil.NewProject(t, pool)
	s := New(repoDbP.New(pool))
	ctx := context.Background()

	minute := time.Now().UTC().Truncate(time.Minute)
	at := time.Now().UTC().Truncate(time.Millisecond)

	res, err := s.Record(ctx, []*model.Hit{hit(projectId, "h1", 70, at, "1.0")}, 100, minute)
	require.NoError(t, err)
	require.Len(t, res, 1)
	issueId := res[0].IssueId
	assert.Equal(t, int64(70), res[0].StoreAllowed)

	res, err = s.Record(ctx, []*model.Hit{hit(projectId, "h1", 70, at.Add(time.Second), "1.1")}, 100, minute)
	require.NoError(t, err)
	assert.Equal(t, issueId, res[0].IssueId, "тот же отпечаток — та же проблема")
	assert.Equal(t, int64(30), res[0].StoreAllowed, "до предела 100 в минуту осталось 30")

	res, err = s.Record(ctx, []*model.Hit{hit(projectId, "h1", 5, at, "1.0")}, 100, minute)
	require.NoError(t, err)
	assert.Zero(t, res[0].StoreAllowed, "предел минуты исчерпан")

	res, err = s.Record(ctx, []*model.Hit{hit(projectId, "h1", 5, at, "")}, 100, minute.Add(time.Minute))
	require.NoError(t, err)
	assert.Equal(t, int64(5), res[0].StoreAllowed, "новая минута — новый предел")

	var timesSeen int64
	var lastRelease string
	var lastSeen time.Time
	require.NoError(t, pool.QueryRow(ctx, `select times_seen, last_release, last_seen from issue where id = $1`, issueId).
		Scan(&timesSeen, &lastRelease, &lastSeen))
	assert.Equal(t, int64(150), timesSeen, "счётчик — все события, а не только сохранённые")
	assert.Equal(t, "1.1", lastRelease, "релиз самого позднего события")
	assert.True(t, lastSeen.Equal(at.Add(time.Second)))
}

func TestRecordConcurrentNewHash(t *testing.T) {
	pool := testutil.PgPool(t)
	projectId := testutil.NewProject(t, pool)
	s := New(repoDbP.New(pool))
	ctx := context.Background()
	minute := time.Now().UTC().Truncate(time.Minute)
	at := time.Now().UTC()

	// две копии обработки одновременно видят новый отпечаток
	const copies = 8
	ids := make([]int64, copies)
	var wg sync.WaitGroup
	for i := range copies {
		wg.Go(func() {
			res, err := s.Record(ctx, []*model.Hit{hit(projectId, "race", 10, at, "")}, 100, minute)
			assert.NoError(t, err)
			if err == nil {
				ids[i] = res[0].IssueId
			}
		})
	}
	wg.Wait()

	for _, id := range ids {
		assert.Equal(t, ids[0], id, "все копии получили одну проблему")
	}

	var issues, timesSeen int64
	require.NoError(t, pool.QueryRow(ctx, `select count(*), coalesce(sum(times_seen), 0) from issue where project_id = $1`, projectId).
		Scan(&issues, &timesSeen))
	assert.Equal(t, int64(1), issues, "лишние проблемы откатились")
	assert.Equal(t, int64(copies*10), timesSeen)
}

func TestRecordBatchWithSeveralHashes(t *testing.T) {
	pool := testutil.PgPool(t)
	projectId := testutil.NewProject(t, pool)
	s := New(repoDbP.New(pool))
	minute := time.Now().UTC().Truncate(time.Minute)
	at := time.Now().UTC()

	res, err := s.Record(context.Background(), []*model.Hit{
		hit(projectId, "a", 3, at, ""),
		hit(projectId, "b", 200, at, ""),
	}, 100, minute)
	require.NoError(t, err)
	require.Len(t, res, 2)
	assert.NotEqual(t, res[0].IssueId, res[1].IssueId)
	assert.Equal(t, int64(3), res[0].StoreAllowed)
	assert.Equal(t, int64(100), res[1].StoreAllowed)
}

func TestRecordDeletedProject(t *testing.T) {
	pool := testutil.PgPool(t)
	projectId := testutil.NewProject(t, pool)
	s := New(repoDbP.New(pool))

	res, err := s.Record(context.Background(), []*model.Hit{
		hit(999999999, "x", 1, time.Now(), ""),
		hit(projectId, "y", 1, time.Now(), ""),
	}, 100, time.Now().Truncate(time.Minute))
	require.NoError(t, err, "удалённый проект не роняет пачку")
	assert.Zero(t, res[0].IssueId)
	assert.Zero(t, res[0].StoreAllowed)
	assert.NotZero(t, res[1].IssueId, "события живого проекта той же пачки обрабатываются")
}
