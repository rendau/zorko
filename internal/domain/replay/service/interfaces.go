package service

import (
	"context"
	"time"

	"github.com/rendau/zorko/internal/domain/replay/model"
)

type RepoChI interface {
	InsertSegments(ctx context.Context, segments []*model.Segment) error
	InsertLinks(ctx context.Context, links []*model.Link) error
	List(ctx context.Context, pars *model.ListReq) ([]*model.Summary, int64, error)
	Get(ctx context.Context, projectId int64, replayId string) (*model.Summary, bool, error)
	IssueReplays(ctx context.Context, projectId, issueId int64, page, pageSize int64) ([]*model.Summary, int64, error)
	Links(ctx context.Context, projectId int64, replayId string, limit int) ([]*model.Link, error)
	SegmentReceivedAt(ctx context.Context, projectId int64, replayId string, segmentId int) (time.Time, bool, error)
	SegmentsReceivedAt(ctx context.Context, projectId int64, replayId string) (map[int]time.Time, error)
}

type RepoBlobI interface {
	Put(ctx context.Context, key string, data []byte) error
	Get(ctx context.Context, key string, maxBytes int64) ([]byte, bool, error)
	Days(ctx context.Context) ([]time.Time, error)
	DeleteDay(ctx context.Context, day time.Time) (int, error)
}

type RepoDbI interface {
	TryLock(ctx context.Context, key int64) (func(), bool, error)
}
