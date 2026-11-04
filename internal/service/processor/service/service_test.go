package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventModel "github.com/rendau/zorko/internal/domain/event/model"
	issueModel "github.com/rendau/zorko/internal/domain/issue/model"
	groupingService "github.com/rendau/zorko/internal/service/grouping/service"
	normalizeService "github.com/rendau/zorko/internal/service/normalize/service"
	queueModel "github.com/rendau/zorko/internal/service/queue/model"
)

type queueFake struct {
	mu       sync.Mutex
	batches  [][]*queueModel.Event
	commits  int
	polled   chan struct{}
	afterAll func()
}

func (q *queueFake) PollEvents(ctx context.Context, _ int) ([]*queueModel.Event, error) {
	q.mu.Lock()
	if len(q.batches) > 0 {
		b := q.batches[0]
		q.batches = q.batches[1:]
		q.mu.Unlock()
		return b, nil
	}
	q.mu.Unlock()
	if q.afterAll != nil {
		q.afterAll()
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func (q *queueFake) CommitPolled(context.Context) error {
	q.mu.Lock()
	q.commits++
	q.mu.Unlock()
	return nil
}

type issueFake struct {
	calls   [][]*issueModel.Hit
	allowed int64
	deleted map[int64]bool
	err     error
}

func (f *issueFake) Record(_ context.Context, hits []*issueModel.Hit, storeCap int64, _ time.Time) ([]*issueModel.HitResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.calls = append(f.calls, hits)
	res := make([]*issueModel.HitResult, len(hits))
	for i, h := range hits {
		if f.deleted[h.ProjectId] {
			res[i] = &issueModel.HitResult{}
			continue
		}
		res[i] = &issueModel.HitResult{IssueId: int64(100 + i), StoreAllowed: min(f.allowed, storeCap)}
	}
	return res, nil
}

type eventFake struct {
	calls     int
	failTimes int
	alwaysErr bool
	hang      bool
	all, full []*eventModel.Event
}

func (f *eventFake) Store(ctx context.Context, all, full []*eventModel.Event) error {
	f.calls++
	if f.hang && f.calls == 1 {
		<-ctx.Done() // «тихо» пропавшая база: ни ответа, ни ошибки
		return ctx.Err()
	}
	if f.alwaysErr || f.calls <= f.failTimes {
		return errors.New("clickhouse down")
	}
	f.all, f.full = all, full
	return nil
}

func ev(projectId int64, payload string) *queueModel.Event {
	return &queueModel.Event{EventId: "ffffffffffffffffffffffffffffffff", ProjectId: projectId, ReceivedAt: time.Now(), Payload: []byte(payload)}
}

const typeErrorPayload = `{"exception":{"values":[{"type":"TypeError","value":"x","stacktrace":{"frames":[{"filename":"/src/cart.js","function":"addItem"}]}}]},"release":"1.0"}`

func run(t *testing.T, q *queueFake, issues *issueFake, events *eventFake) {
	t.Helper()
	s := New(Config{BatchSize: 500, StorePerMinute: 100, RetryMin: time.Millisecond, RetryMax: 5 * time.Millisecond, StageTimeout: time.Second},
		q, normalizeService.New(), nil, groupingService.New(), issues, events)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	q.afterAll = cancel
	s.Start(ctx)
	s.Wait()
}

func TestBatch(t *testing.T) {
	q := &queueFake{batches: [][]*queueModel.Event{{
		ev(1, typeErrorPayload),
		ev(1, typeErrorPayload),
		ev(1, typeErrorPayload),
		ev(1, `{"message":"other"}`),
		ev(1, `not json`),
		{EventId: "x", Payload: []byte(`{}`)}, // без project_id
	}}}
	issues := &issueFake{allowed: 2}
	events := &eventFake{}

	run(t, q, issues, events)

	require.Len(t, issues.calls, 1)
	hits := issues.calls[0]
	require.Len(t, hits, 2, "два отпечатка — две прибавки")
	assert.Equal(t, int64(3), hits[0].Count, "одинаковые события пачки — одна прибавка")
	assert.Equal(t, "TypeError: x", hits[0].Title)
	assert.Equal(t, "1.0", hits[0].LastRelease)

	assert.Len(t, events.all, 4, "битые сообщения пропущены")
	assert.Len(t, events.full, 3, "по разрешению: 2 из TypeError и 1 из сообщения")
	for _, e := range events.all {
		assert.NotZero(t, e.IssueId)
	}
	assert.Equal(t, 1, q.commits)
}

func TestClickhouseRetryDoesNotRecountPostgres(t *testing.T) {
	q := &queueFake{batches: [][]*queueModel.Event{{ev(1, typeErrorPayload)}}}
	issues := &issueFake{allowed: 100}
	events := &eventFake{failTimes: 3}

	run(t, q, issues, events)

	assert.Len(t, issues.calls, 1, "Postgres прибавлен один раз")
	assert.Equal(t, 4, events.calls, "ClickHouse повторён до успеха")
	assert.Len(t, events.all, 1)
	assert.Equal(t, 1, q.commits, "подтверждение после записи")
}

func TestDeletedProject(t *testing.T) {
	q := &queueFake{batches: [][]*queueModel.Event{{ev(1, typeErrorPayload), ev(2, typeErrorPayload)}}}
	issues := &issueFake{allowed: 100, deleted: map[int64]bool{1: true}}
	events := &eventFake{}

	run(t, q, issues, events)

	require.Len(t, events.all, 1)
	assert.Equal(t, int64(2), events.all[0].ProjectId)
	assert.Equal(t, 1, q.commits)
}

func TestShutdownDuringOutageAbandonsBatch(t *testing.T) {
	q := &queueFake{batches: [][]*queueModel.Event{{ev(1, typeErrorPayload)}}}
	issues := &issueFake{allowed: 100}
	events := &eventFake{alwaysErr: true}

	s := New(Config{BatchSize: 500, StorePerMinute: 100, RetryMin: time.Millisecond, RetryMax: 5 * time.Millisecond, StageTimeout: time.Second},
		q, normalizeService.New(), nil, groupingService.New(), issues, events)

	ctx, cancel := context.WithCancel(context.Background())
	s.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	cancel()

	done := make(chan struct{})
	go func() { s.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("обработка не остановилась во время сбоя ClickHouse")
	}

	assert.Zero(t, q.commits, "брошенная пачка не подтверждается — её получит другая копия")
	assert.Greater(t, events.calls, 1)
}

func TestHangingDatabaseIsRetried(t *testing.T) {
	q := &queueFake{batches: [][]*queueModel.Event{{ev(1, typeErrorPayload)}}}
	issues := &issueFake{allowed: 100}
	events := &eventFake{hang: true}

	run(t, q, issues, events)

	assert.Equal(t, 2, events.calls, "зависшая попытка прервана по пределу времени и повторена")
	assert.Len(t, events.all, 1)
	assert.Equal(t, 1, q.commits)
}
