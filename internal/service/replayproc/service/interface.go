package service

import (
	"context"

	eventModel "github.com/rendau/zorko/internal/domain/event/model"
	replayModel "github.com/rendau/zorko/internal/domain/replay/model"
	queueModel "github.com/rendau/zorko/internal/service/queue/model"
)

type queueI interface {
	PollEvents(ctx context.Context, max int) ([]*queueModel.Event, error)
	CommitPolled(ctx context.Context) error
}

// normalizerI — описание сегмента устроено как событие: пользователь, браузер, релиз разбираются так же
type normalizerI interface {
	Normalize(raw *eventModel.RawEvent) (*eventModel.Event, error)
}

type replayServiceI interface {
	InsertSegments(ctx context.Context, segments []*replayModel.Segment) error
}

type projectLabelI interface {
	Label(projectId int64) string
}
