//go:build integration

package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	queueModel "github.com/rendau/zorko/internal/service/queue/model"
)

// Стенд: scripts/stand.sh up (Redpanda на localhost:19092)
func TestPublishPollCommit(t *testing.T) {
	ctx, ctxCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer ctxCancel()

	topic := "test.events." + uuid.NewString()[:8]
	conf := Config{
		Brokers:         []string{"localhost:19092"},
		Topic:           topic,
		ConsumerGroup:   "test-" + topic,
		TopicPartitions: 3,
		TopicRetention:  time.Hour,
		WithProducer:    true,
		WithConsumer:    true,
		FetchMaxWait:    100 * time.Millisecond,
	}

	s, err := New(conf)
	require.NoError(t, err)
	defer s.Close()

	require.NoError(t, s.EnsureTopic(ctx))
	require.NoError(t, s.EnsureTopic(ctx), "повторное создание темы — не ошибка")

	receivedAt := time.Now().UTC().Truncate(time.Millisecond)
	events := []*queueModel.Event{
		{EventId: "e1", ProjectId: 7, ReceivedAt: receivedAt, ClientIp: "10.0.0.1", Payload: []byte(`{"a":1}`)},
		{EventId: "e2", ProjectId: 7, ReceivedAt: receivedAt, ClientIp: "10.0.0.2", Payload: []byte(`{"a":2}`)},
	}
	require.NoError(t, s.PublishEvents(ctx, events))

	got := map[string]*queueModel.Event{}
	for len(got) < len(events) {
		polled, err := s.PollEvents(ctx, 100)
		require.NoError(t, err)
		for _, e := range polled {
			got[e.EventId] = e
		}
		require.NoError(t, s.CommitPolled(ctx))
	}

	require.Contains(t, got, "e1")
	assert.Equal(t, int64(7), got["e1"].ProjectId)
	assert.Equal(t, "10.0.0.1", got["e1"].ClientIp)
	assert.True(t, receivedAt.Equal(got["e1"].ReceivedAt))
	assert.JSONEq(t, `{"a":1}`, string(got["e1"].Payload))

	// после подтверждения новая копия той же группы не получает старые записи
	s.Close()
	s2, err := New(conf)
	require.NoError(t, err)
	defer s2.Close()

	shortCtx, shortCancel := context.WithTimeout(ctx, 3*time.Second)
	defer shortCancel()
	polled, _ := s2.PollEvents(shortCtx, 100)
	assert.Empty(t, polled)
}
