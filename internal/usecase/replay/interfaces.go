package replay

import (
	"context"

	issueModel "github.com/rendau/zorko/internal/domain/issue/model"
	projectModel "github.com/rendau/zorko/internal/domain/project/model"
	replayModel "github.com/rendau/zorko/internal/domain/replay/model"
	accessModel "github.com/rendau/zorko/internal/service/access/model"
)

type SessionServiceI interface {
	CtxIsAuthorized(ctx context.Context) bool
}

type ProjectServiceI interface {
	List(ctx context.Context, pars *projectModel.ListReq) ([]*projectModel.Main, int64, error)
}

type IssueServiceI interface {
	Get(ctx context.Context, id int64, errNE bool) (*issueModel.Main, bool, error)
	ListByIds(ctx context.Context, ids []int64) ([]*issueModel.Main, error)
}

type ReplayServiceI interface {
	List(ctx context.Context, pars *replayModel.ListReq) ([]*replayModel.Summary, int64, error)
	Get(ctx context.Context, projectId int64, replayId string) (*replayModel.Summary, error)
	IssueReplays(ctx context.Context, projectId, issueId int64, page, pageSize int64) ([]*replayModel.Summary, int64, error)
	Links(ctx context.Context, projectId int64, replayId string) ([]*replayModel.Link, error)
	SegmentBody(ctx context.Context, projectId int64, replayId string, segmentId int, maxBytes int64) ([]byte, error)
}

type AccessServiceI interface {
	ProjectAccess(ctx context.Context) (*accessModel.ProjectAccess, error)
	CheckProject(ctx context.Context, projectId int64) error
}
