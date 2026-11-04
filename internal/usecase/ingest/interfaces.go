package ingest

import (
	"context"
	"time"

	projectKeyModel "github.com/rendau/zorko/internal/domain/project_key/model"
	queueModel "github.com/rendau/zorko/internal/service/queue/model"
)

type KeyCacheI interface {
	Find(projectId int64, publicKey string) (*projectKeyModel.Main, bool)
}

type RateLimiterI interface {
	Allow(keyId int64, limitPerSec int32, n int) (bool, time.Duration)
}

type QueueI interface {
	PublishEvents(ctx context.Context, events []*queueModel.Event) error
}
