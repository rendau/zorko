package ingest

import (
	"context"
	"time"

	projectKeyModel "github.com/rendau/zorko/internal/domain/project_key/model"
	replayModel "github.com/rendau/zorko/internal/domain/replay/model"
	"github.com/rendau/zorko/internal/service/projectlabel"
	queueModel "github.com/rendau/zorko/internal/service/queue/model"
)

type KeyCacheI interface {
	Find(projectId int64, publicKey string) (*projectKeyModel.Main, bool)
}

type RateLimiterI interface {
	Allow(keyId int64, limitPerSec int32, n int) (bool, time.Duration)
}

type ProjectLabelI interface {
	Label(projectId int64) string
	Replay(projectId int64) projectlabel.ReplaySettings
}

// ReplayStoreI — тела сегментов записей в S3 (ADR-012)
type ReplayStoreI interface {
	PutSegmentBody(ctx context.Context, seg *replayModel.Segment, body []byte, compressed bool) (int64, error)
}

type ReplayLimitI interface {
	Allow(projectId int64, limit int32, now time.Time) (bool, time.Duration)
}

type QueueI interface {
	PublishEvents(ctx context.Context, events []*queueModel.Event) error
}
