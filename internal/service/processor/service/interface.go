package service

import (
	"context"
	"time"

	eventModel "github.com/rendau/zorko/internal/domain/event/model"
	issueModel "github.com/rendau/zorko/internal/domain/issue/model"
	queueModel "github.com/rendau/zorko/internal/service/queue/model"
)

type queueI interface {
	PollEvents(ctx context.Context, max int) ([]*queueModel.Event, error)
	CommitPolled(ctx context.Context) error
}

type normalizerI interface {
	Normalize(raw *eventModel.RawEvent) (*eventModel.Event, error)
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
