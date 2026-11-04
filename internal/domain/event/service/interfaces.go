package service

import (
	"context"
	"time"

	"github.com/rendau/zorko/internal/domain/event/model"
)

type RepoChI interface {
	InsertStats(ctx context.Context, events []*model.Event) error
	InsertEvents(ctx context.Context, events []*model.Event) error
}

type RepoChReadI interface {
	IssueAggregates(ctx context.Context, f *model.StatFilter) ([]*model.IssueAgg, bool, error)
	Series(ctx context.Context, f *model.StatFilter, step time.Duration) (map[int64][]int64, error)
	TagTop(ctx context.Context, f *model.StatFilter, top int) ([]*model.TagDist, error)
	TagKeys(ctx context.Context, f *model.StatFilter, limit int) ([]string, error)
	TagValues(ctx context.Context, f *model.StatFilter, key, prefix string, limit int) ([]string, error)
	ListEvents(ctx context.Context, f *model.StatFilter, offset, limit int64) ([]*model.EventSummary, int64, error)
	GetEvent(ctx context.Context, projectId, issueId int64, eventId string) (*model.EventDetail, bool, error)
}
