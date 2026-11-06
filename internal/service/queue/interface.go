package queue

import (
	"context"

	queueModel "github.com/rendau/zorko/internal/service/queue/model"
)

// Queue — очередь событий между приёмом и обработкой (Redpanda, ADR-002)
type Queue interface {
	EnsureTopic(ctx context.Context) error
	PublishEvents(ctx context.Context, events []*queueModel.Event) error
	PollEvents(ctx context.Context, max int) ([]*queueModel.Event, error)
	CommitPolled(ctx context.Context) error
	Start(ctx context.Context)
	Wait()
	Close()
}
