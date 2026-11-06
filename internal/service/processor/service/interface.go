package service

import (
	"context"
	"time"

	eventModel "github.com/rendau/zorko/internal/domain/event/model"
	issueModel "github.com/rendau/zorko/internal/domain/issue/model"
	replayModel "github.com/rendau/zorko/internal/domain/replay/model"
	queueModel "github.com/rendau/zorko/internal/service/queue/model"
)

type queueI interface {
	PollEvents(ctx context.Context, max int) ([]*queueModel.Event, error)
	CommitPolled(ctx context.Context) error
}

type normalizerI interface {
	Normalize(raw *eventModel.RawEvent) (*eventModel.Event, error)
}

type symbolicatorI interface {
	Symbolicate(ctx context.Context, events []*eventModel.Event)
}

type grouperI interface {
	Group(ev *eventModel.Event) eventModel.Grouping
}

type issueServiceI interface {
	Record(ctx context.Context, hits []*issueModel.Hit, storeCap int64, minute time.Time) ([]*issueModel.HitResult, error)
}

type eventServiceI interface {
	Store(ctx context.Context, all []*eventModel.Event, full []*eventModel.Event) error
}

// ReplayLinker — связь записи сессии с проблемой по тегу replayId (ADR-014); экспортирован для
// композиции: без S3 передаётся nil-интерфейс
type ReplayLinker interface {
	InsertLinks(ctx context.Context, links []*replayModel.Link) error
}

type projectLabelI interface {
	Label(projectId int64) string
}
