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
	"github.com/rendau/zorko/internal/errs"
	"github.com/rendau/zorko/internal/testutil"
)

func TestRead(t *testing.T) {
	conn := testutil.ChConn(t)
	repo := repoChP.New(conn)
	write, read := New(repo), NewRead(repo)
	ctx := context.Background()

	projectId := int64(2_000_000_000 + time.Now().UnixNano()%1_000_000)
	now := time.Now().UTC().Truncate(time.Millisecond)

	ev := func(issueId int64, ago time.Duration, browser, user string, env string) *model.Event {
		return &model.Event{
			EventId: strings.ReplaceAll(uuid.NewString(), "-", ""), ProjectId: projectId, IssueId: issueId,
			Timestamp: now.Add(-ago), ReceivedAt: now, Level: "error", Environment: env, Release: "1.0",
			User: model.User{Id: user}, Browser: browser,
			Tags:     map[string]string{"browser.name": browser, "environment": env, "url": "https://shop/cart/" + user},
			Grouping: model.Grouping{Title: "TypeError: x"}, Data: []byte(`{"n":1}`),
		}
	}
	events := []*model.Event{
		ev(1, time.Hour, "Chrome", "u1", "production"),
		ev(1, 2*time.Hour, "Chrome", "u2", "production"),
		ev(1, 3*time.Hour, "Safari", "u1", "production"),
		ev(2, 30*time.Minute, "Safari", "u3", "staging"),
		ev(3, 40*24*time.Hour, "Chrome", "u9", "production"), // старше периода
	}
	require.NoError(t, write.Store(ctx, events, events))

	f := &model.StatFilter{ProjectIds: []int64{projectId}, From: now.Add(-24 * time.Hour), To: now.Add(time.Minute)}

	aggs, truncated, err := read.IssueAggregates(ctx, f)
	require.NoError(t, err)
	assert.False(t, truncated)
	require.Len(t, aggs, 2, "проблема 3 — вне периода")
	assert.Equal(t, int64(1), aggs[0].IssueId, "больше событий — первой")
	assert.Equal(t, int64(3), aggs[0].Events)
	assert.Equal(t, int64(2), aggs[0].Users)

	// фильтры
	cases := map[string]*model.StatFilter{
		"тег равен":    {Conds: []model.TagCond{{Key: "browser.name", Op: "eq", Value: "Safari"}}},
		"окружение":    {Environment: "staging"},
		"содержит":     {Conds: []model.TagCond{{Key: "url", Op: "like", Value: "%/u3"}}},
		"не равен":     {Conds: []model.TagCond{{Key: "browser.name", Op: "ne", Value: "Chrome"}}},
		"список задач": {IssueIds: new([]int64{2})},
	}
	want := map[string]int{"тег равен": 2, "окружение": 1, "содержит": 1, "не равен": 2, "список задач": 1}
	for name, extra := range cases {
		ff := *f
		ff.Conds, ff.Environment, ff.IssueIds = extra.Conds, extra.Environment, extra.IssueIds
		got, _, err := read.IssueAggregates(ctx, &ff)
		require.NoError(t, err, name)
		assert.Len(t, got, want[name], name)
	}

	// ряд по часам за сутки
	series, err := read.Series(ctx, &model.StatFilter{ProjectIds: f.ProjectIds, IssueIds: new([]int64{1}), From: f.From, To: f.From.Add(24 * time.Hour)}, time.Hour)
	require.NoError(t, err)
	require.Len(t, series[1], 24)
	var sum int64
	for _, v := range series[1] {
		sum += v
	}
	assert.Equal(t, int64(3), sum)

	// топ тегов
	tags, err := read.TagTop(ctx, &model.StatFilter{ProjectIds: f.ProjectIds, IssueIds: new([]int64{1}), From: f.From, To: f.To}, 5)
	require.NoError(t, err)
	byKey := map[string]*model.TagDist{}
	for _, d := range tags {
		byKey[d.Key] = d
	}
	require.Contains(t, byKey, "browser.name")
	assert.Equal(t, int64(3), byKey["browser.name"].Total)
	assert.Equal(t, model.TagValueCount{Value: "Chrome", Count: 2}, byKey["browser.name"].Values[0])
	assert.Equal(t, model.TagValueCount{Value: "Safari", Count: 1}, byKey["browser.name"].Values[1])
	require.Len(t, byKey["url"].Values, 2, "значений у ключа два: u1 дважды и u2")
	assert.Equal(t, model.TagValueCount{Value: "https://shop/cart/u1", Count: 2}, byKey["url"].Values[0])

	// подсказки
	keys, err := read.TagKeys(ctx, f, 100)
	require.NoError(t, err)
	assert.Contains(t, keys, "browser.name")
	values, err := read.TagValues(ctx, f, "browser.name", "saf", 20)
	require.NoError(t, err)
	assert.Equal(t, []string{"Safari"}, values)
	// окружение, релиз и уровень читаются из своих колонок, а не из карты тегов — ответ тот же
	values, err = read.TagValues(ctx, f, "environment", "", 20)
	require.NoError(t, err)
	assert.Equal(t, []string{"production", "staging"}, values, "по частоте")
	values, err = read.TagValues(ctx, f, "environment", "STAG", 20)
	require.NoError(t, err)
	assert.Equal(t, []string{"staging"}, values)
	values, err = read.TagValues(ctx, f, "release", "", 20)
	require.NoError(t, err)
	assert.Equal(t, []string{"1.0"}, values)

	// события и соседи; повторная доставка (те же строки ещё раз) ответов не меняет
	require.NoError(t, repo.InsertEvents(ctx, events[:3]))
	issueFilter := &model.StatFilter{ProjectIds: f.ProjectIds, IssueIds: new([]int64{1}), From: now.Add(-30 * 24 * time.Hour), To: now.Add(time.Minute)}
	list, total, err := read.ListEvents(ctx, issueFilter, 0, 50)
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	require.Len(t, list, 3)
	assert.Equal(t, events[0].EventId, list[0].EventId, "новые первыми, id без дефисов")

	latest, err := read.GetEvent(ctx, projectId, 1, "latest")
	require.NoError(t, err)
	assert.Equal(t, events[0].EventId, latest.EventId)
	assert.Empty(t, latest.NextEventId, "новее нет")
	assert.Equal(t, events[1].EventId, latest.PrevEventId)
	assert.JSONEq(t, `{"n":1}`, latest.Data)

	middle, err := read.GetEvent(ctx, projectId, 1, latest.PrevEventId)
	require.NoError(t, err)
	assert.Equal(t, events[0].EventId, middle.NextEventId)
	assert.Equal(t, events[2].EventId, middle.PrevEventId)

	oldest, err := read.GetEvent(ctx, projectId, 1, "oldest")
	require.NoError(t, err)
	assert.Equal(t, events[2].EventId, oldest.EventId)

	_, err = read.GetEvent(ctx, projectId, 2, events[0].EventId)
	assert.ErrorIs(t, err, errs.ObjectNotFound, "событие другой проблемы")
	_, err = read.GetEvent(ctx, projectId, 1, "garbage")
	assert.ErrorIs(t, err, errs.ObjectNotFound)
}
