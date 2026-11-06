// Package replay — записи сессий для интерфейса (ADR-012..015): список, запись, записи проблемы, тела сегментов
package replay

import (
	"context"
	"fmt"

	"github.com/samber/lo"

	commonModel "github.com/rendau/zorko/internal/domain/common/model"
	issueModel "github.com/rendau/zorko/internal/domain/issue/model"
	projectModel "github.com/rendau/zorko/internal/domain/project/model"
	replayModel "github.com/rendau/zorko/internal/domain/replay/model"
	"github.com/rendau/zorko/internal/errs"
	"github.com/rendau/zorko/internal/util"
)

const maxPageSize = 100

type Usecase struct {
	sessionSvc SessionServiceI
	projectSvc ProjectServiceI
	issueSvc   IssueServiceI
	replaySvc  ReplayServiceI
	accessSvc  AccessServiceI

	maxSegmentBytes int64 // тело сегмента из S3 — не больше этого
}

func New(sessionSvc SessionServiceI, projectSvc ProjectServiceI, issueSvc IssueServiceI, replaySvc ReplayServiceI,
	accessSvc AccessServiceI, maxSegmentBytes int64) *Usecase {
	return &Usecase{sessionSvc: sessionSvc, projectSvc: projectSvc, issueSvc: issueSvc, replaySvc: replaySvc,
		accessSvc: accessSvc, maxSegmentBytes: maxSegmentBytes}
}

// List — записи видимых проектов организации, новые первыми
func (u *Usecase) List(ctx context.Context, req *ListReq) ([]*replayModel.Summary, int64, error) {
	if !u.sessionSvc.CtxIsAuthorized(ctx) {
		return nil, 0, errs.NotAuthorized
	}
	if err := util.RequirePageSize(req.ListParams, maxPageSize); err != nil {
		return nil, 0, err
	}
	if req.OrgId == 0 {
		return nil, 0, errs.ErrFull{Err: errs.InvalidRequest, Desc: "org_id is required", Fields: map[string]string{"org_id": "required"}}
	}
	if req.Type != "" && req.Type != replayModel.TypeBuffer && req.Type != replayModel.TypeSession {
		return nil, 0, errs.ErrFull{Err: errs.InvalidRequest, Desc: "replay_type: buffer or session", Fields: map[string]string{"replay_type": "invalid"}}
	}

	projects, _, err := u.projectSvc.List(ctx, &projectModel.ListReq{OrgId: &req.OrgId})
	if err != nil {
		return nil, 0, fmt.Errorf("projectSvc.List: %w", err)
	}
	projectIds := lo.Map(projects, func(p *projectModel.Main, _ int) int64 { return p.Id })
	if len(req.ProjectIds) > 0 {
		// только проекты этой организации
		projectIds = lo.Intersect(projectIds, req.ProjectIds)
	}
	access, err := u.accessSvc.ProjectAccess(ctx)
	if err != nil {
		return nil, 0, err
	}
	projectIds = access.Filter(projectIds)
	if len(projectIds) == 0 {
		return []*replayModel.Summary{}, 0, nil
	}

	items, total, err := u.replaySvc.List(ctx, &replayModel.ListReq{
		ListParams:  req.ListParams,
		ProjectIds:  projectIds,
		Environment: lo.EmptyableToPtr(req.Environment),
		Release:     lo.EmptyableToPtr(req.Release),
		Query:       lo.EmptyableToPtr(req.Query),
		HasErrors:   req.HasErrors,
		Type:        lo.EmptyableToPtr(req.Type),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("replaySvc.List: %w", err)
	}
	return items, total, nil
}

// Get — запись проекта и её ошибки (с заголовками проблем)
func (u *Usecase) Get(ctx context.Context, projectId int64, replayId string) (*GetRep, error) {
	if err := u.checkProject(ctx, projectId); err != nil {
		return nil, err
	}

	summary, err := u.replaySvc.Get(ctx, projectId, replayId)
	if err != nil {
		return nil, err
	}

	links, err := u.replaySvc.Links(ctx, projectId, summary.ReplayId)
	if err != nil {
		return nil, fmt.Errorf("replaySvc.Links: %w", err)
	}
	issues, err := u.issueSvc.ListByIds(ctx, lo.Uniq(lo.Map(links, func(l *replayModel.Link, _ int) int64 { return l.IssueId })))
	if err != nil {
		return nil, fmt.Errorf("issueSvc.ListByIds: %w", err)
	}
	// проблему могли удалить вместе с проектом; чужой проект в связях не встречается — ключ (project, replay)
	byId := lo.KeyBy(issues, func(v *issueModel.Main) int64 { return v.Id })

	return &GetRep{
		Summary: summary,
		Errors: lo.FilterMap(links, func(l *replayModel.Link, _ int) (*ErrorMark, bool) {
			issue, ok := byId[l.IssueId]
			if !ok || issue.ProjectId != projectId {
				return nil, false
			}
			return &ErrorMark{IssueId: l.IssueId, Title: issue.Title, EventId: l.EventId, Timestamp: l.Timestamp}, true
		}),
	}, nil
}

// IssueReplays — записи с ошибками проблемы, по времени ошибки, новые первыми
func (u *Usecase) IssueReplays(ctx context.Context, issueId int64, pars commonModel.ListParams) ([]*replayModel.Summary, int64, error) {
	if !u.sessionSvc.CtxIsAuthorized(ctx) {
		return nil, 0, errs.NotAuthorized
	}
	if issueId == 0 {
		return nil, 0, errs.IdRequired
	}
	if err := util.RequirePageSize(pars, maxPageSize); err != nil {
		return nil, 0, err
	}
	issue, _, err := u.issueSvc.Get(ctx, issueId, true)
	if err != nil {
		return nil, 0, fmt.Errorf("issueSvc.Get: %w", err)
	}
	if err = u.accessSvc.CheckProject(ctx, issue.ProjectId); err != nil {
		return nil, 0, err
	}

	items, total, err := u.replaySvc.IssueReplays(ctx, issue.ProjectId, issue.Id, pars.Page, pars.PageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("replaySvc.IssueReplays: %w", err)
	}
	return items, total, nil
}

// SegmentBody — тело сегмента (zlib) для проигрывателя; распаковывает браузер (ADR-015)
func (u *Usecase) SegmentBody(ctx context.Context, projectId int64, replayId string, segmentId int) ([]byte, error) {
	if err := u.checkProject(ctx, projectId); err != nil {
		return nil, err
	}
	body, err := u.replaySvc.SegmentBody(ctx, projectId, replayId, segmentId, u.maxSegmentBytes)
	if err != nil {
		return nil, fmt.Errorf("replaySvc.SegmentBody: %w", err)
	}
	return body, nil
}

func (u *Usecase) checkProject(ctx context.Context, projectId int64) error {
	if !u.sessionSvc.CtxIsAuthorized(ctx) {
		return errs.NotAuthorized
	}
	if projectId == 0 {
		return errs.ErrFull{Err: errs.InvalidRequest, Desc: "project_id is required", Fields: map[string]string{"project_id": "required"}}
	}
	return u.accessSvc.CheckProject(ctx, projectId)
}
