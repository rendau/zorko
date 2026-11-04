//go:build integration

package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rendau/zorko/internal/domain/event/model"
	repoChP "github.com/rendau/zorko/internal/domain/event/repo/ch"
	"github.com/rendau/zorko/internal/testutil"
)

func TestStore(t *testing.T) {
	conn := testutil.ChConn(t)
	s := New(repoChP.New(conn))
	ctx := context.Background()

	projectId := int64(time.Now().UnixNano() % 1_000_000_000)
	now := time.Now().UTC().Truncate(time.Millisecond)

	newEvent := func() *model.Event {
		return &model.Event{
			EventId: strings.ReplaceAll(uuid.NewString(), "-", ""), ProjectId: projectId, IssueId: 7,
			Timestamp: now, ReceivedAt: now, Level: "error", Environment: "production", Release: "1.0",
			User: model.User{Id: "u-1"}, Tags: map[string]string{"browser": "Chrome 130"},
			Grouping: model.Grouping{Title: "TypeError: x", Culprit: "cart.js в addItem", Type: "TypeError", Value: "x"},
			Data:     []byte(`{"message":"x"}`),
		}
	}

	all := []*model.Event{newEvent(), newEvent(), newEvent()}
	require.NoError(t, s.Store(ctx, all, all[:1]))

	// повторная обработка той же пачки (at-least-once) после слияния не задваивает строки
	require.NoError(t, s.Store(ctx, all, all[:1]))
	require.NoError(t, conn.Exec(ctx, `optimize table event_stat final`))
	require.NoError(t, conn.Exec(ctx, `optimize table event final`))

	var stats, full uint64
	require.NoError(t, conn.QueryRow(ctx, `select count() from event_stat where project_id = ?`, uint64(projectId)).Scan(&stats))
	require.NoError(t, conn.QueryRow(ctx, `select count() from event where project_id = ?`, uint64(projectId)).Scan(&full))
	assert.Equal(t, uint64(3), stats)
	assert.Equal(t, uint64(1), full)

	var title, browserTag, data string
	require.NoError(t, conn.QueryRow(ctx, `select title, tags['browser'], data from event where project_id = ?`, uint64(projectId)).
		Scan(&title, &browserTag, &data))
	assert.Equal(t, "TypeError: x", title)
	assert.Equal(t, "Chrome 130", browserTag)
	assert.JSONEq(t, `{"message":"x"}`, data)
}
