//go:build integration

package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kadm"

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

// Существующей теме (созданной до предела объёма) срок и предел выставляются при старте
func TestEnsureTopicUpdatesRetention(t *testing.T) {
	ctx, ctxCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer ctxCancel()

	topic := "test.retention." + uuid.NewString()[:8]
	conf := Config{
		Brokers:         []string{"localhost:19092"},
		Topic:           topic,
		TopicPartitions: 1,
		TopicRetention:  time.Hour,
		WithProducer:    true,
	}

	old, err := New(conf)
	require.NoError(t, err)
	defer old.Close()
	require.NoError(t, old.EnsureTopic(ctx))
	assert.Equal(t, map[string]string{"retention.ms": "3600000", "retention.bytes": "-1"}, topicRetention(ctx, t, old, topic))

	conf.TopicRetention = 2 * time.Hour
	conf.TopicRetentionBytes = 512 << 20
	s, err := New(conf)
	require.NoError(t, err)
	defer s.Close()
	require.NoError(t, s.EnsureTopic(ctx))
	assert.Equal(t, map[string]string{"retention.ms": "7200000", "retention.bytes": "536870912"}, topicRetention(ctx, t, s, topic))
}

func topicRetention(ctx context.Context, t *testing.T, s *Service, topic string) map[string]string {
	t.Helper()
	admin := kadm.NewClient(s.producer)
	rcs, err := admin.DescribeTopicConfigs(ctx, topic)
	require.NoError(t, err)
	rc, err := rcs.On(topic, nil)
	require.NoError(t, err)
	got := map[string]string{}
	for _, c := range rc.Configs {
		if c.Key == "retention.ms" || c.Key == "retention.bytes" {
			got[c.Key] = c.MaybeValue()
		}
	}
	_, _ = admin.DeleteTopic(ctx, topic)
	return got
}
