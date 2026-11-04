package issue

import (
	"context"
	"time"

	eventModel "github.com/rendau/zorko/internal/domain/event/model"
	issueModel "github.com/rendau/zorko/internal/domain/issue/model"
	projectModel "github.com/rendau/zorko/internal/domain/project/model"
	searchqueryModel "github.com/rendau/zorko/internal/service/searchquery/model"
)

type SessionServiceI interface {
	CtxIsAuthorized(ctx context.Context) bool
}

type ProjectServiceI interface {
	List(ctx context.Context, pars *projectModel.ListReq) ([]*projectModel.Main, int64, error)
	Get(ctx context.Context, id int64, errNE bool) (*projectModel.Main, bool, error)
}

type IssueServiceI interface {
	Get(ctx context.Context, id int64, errNE bool) (*issueModel.Main, bool, error)
	ListByIds(ctx context.Context, ids []int64) ([]*issueModel.Main, error)
	IdsByText(ctx context.Context, projectIds []int64, words []string, limit int) ([]int64, error)
}

type EventReadServiceI interface {
	IssueAggregates(ctx context.Context, f *eventModel.StatFilter) ([]*eventModel.IssueAgg, bool, error)
	Series(ctx context.Context, f *eventModel.StatFilter, step time.Duration) (map[int64][]int64, error)
	TagTop(ctx context.Context, f *eventModel.StatFilter, top int) ([]*eventModel.TagDist, error)
	TagKeys(ctx context.Context, f *eventModel.StatFilter, limit int) ([]string, error)
	TagValues(ctx context.Context, f *eventModel.StatFilter, key, prefix string, limit int) ([]string, error)
	ListEvents(ctx context.Context, f *eventModel.StatFilter, offset, limit int64) ([]*eventModel.EventSummary, int64, error)
	GetEvent(ctx context.Context, projectId, issueId int64, eventId string) (*eventModel.EventDetail, error)
}

type QueryParserI interface {
	Parse(q string) (*searchqueryModel.Query, error)
}
