// Package artifact — карты исходников для интерфейса: архивы проекта, их файлы, история скачивания с сайта
package artifact

import (
	"context"
	"fmt"

	artifactModel "github.com/rendau/zorko/internal/domain/artifact/model"
	"github.com/rendau/zorko/internal/errs"
	accessModel "github.com/rendau/zorko/internal/service/access/model"
	"github.com/rendau/zorko/internal/util"
)

const siteFetchLimit = 50

type SessionServiceI interface {
	CtxIsAuthorized(ctx context.Context) bool
	CtxIsAdmin(ctx context.Context) bool
}

type ServiceI interface {
	ListBundles(ctx context.Context, pars *artifactModel.BundleListReq) ([]*artifactModel.Bundle, int64, error)
	FilesOfBundle(ctx context.Context, bundleId int64, urls []string) ([]*artifactModel.File, error)
	DeleteBundle(ctx context.Context, id int64) error
	ListSiteFetch(ctx context.Context, projectId int64, state string, limit int) ([]*artifactModel.SiteFetch, error)
	BundleProjectIds(ctx context.Context, bundleId int64) ([]int64, error)
}

type AccessServiceI interface {
	ProjectAccess(ctx context.Context) (*accessModel.ProjectAccess, error)
	CheckProject(ctx context.Context, projectId int64) error
}

type Usecase struct {
	svc        ServiceI
	sessionSvc SessionServiceI
	accessSvc  AccessServiceI
}

func New(svc ServiceI, sessionSvc SessionServiceI, accessSvc AccessServiceI) *Usecase {
	return &Usecase{svc: svc, sessionSvc: sessionSvc, accessSvc: accessSvc}
}

func (u *Usecase) ListBundles(ctx context.Context, pars *artifactModel.BundleListReq) ([]*artifactModel.Bundle, int64, error) {
	if !u.sessionSvc.CtxIsAuthorized(ctx) {
		return nil, 0, errs.NotAuthorized
	}
	if pars.ProjectId == 0 {
		return nil, 0, errs.ErrFull{Err: errs.InvalidRequest, Desc: "project_id is required", Fields: map[string]string{"project_id": "required"}}
	}
	if err := util.RequirePageSize(pars.ListParams, 0); err != nil {
		return nil, 0, err
	}
	if err := u.accessSvc.CheckProject(ctx, pars.ProjectId); err != nil {
		return nil, 0, err
	}
	items, total, err := u.svc.ListBundles(ctx, pars)
	if err != nil {
		return nil, 0, fmt.Errorf("svc.ListBundles: %w", err)
	}
	return items, total, nil
}

func (u *Usecase) ListFiles(ctx context.Context, bundleId int64) ([]*artifactModel.File, error) {
	if !u.sessionSvc.CtxIsAuthorized(ctx) {
		return nil, errs.NotAuthorized
	}
	if bundleId == 0 {
		return nil, errs.IdRequired
	}
	// архив принадлежит организации и привязан к проектам: участнику — если виден хоть один
	access, err := u.accessSvc.ProjectAccess(ctx)
	if err != nil {
		return nil, err
	}
	if !access.All {
		projectIds, err := u.svc.BundleProjectIds(ctx, bundleId)
		if err != nil {
			return nil, fmt.Errorf("svc.BundleProjectIds: %w", err)
		}
		if !access.AllowsAny(projectIds) {
			return nil, errs.NoPermission
		}
	}
	files, err := u.svc.FilesOfBundle(ctx, bundleId, nil)
	if err != nil {
		return nil, fmt.Errorf("svc.FilesOfBundle: %w", err)
	}
	return files, nil
}

// DeleteBundle — архив перестаёт находиться; в обработчике может ещё минуту жить в памяти поиска
func (u *Usecase) DeleteBundle(ctx context.Context, id int64) error {
	if id == 0 {
		return errs.IdRequired
	}
	if !u.sessionSvc.CtxIsAdmin(ctx) {
		return errs.NoPermission
	}
	if err := u.svc.DeleteBundle(ctx, id); err != nil {
		return fmt.Errorf("svc.DeleteBundle: %w", err)
	}
	return nil
}

func (u *Usecase) ListSiteFetch(ctx context.Context, projectId int64, state string) ([]*artifactModel.SiteFetch, error) {
	if !u.sessionSvc.CtxIsAuthorized(ctx) {
		return nil, errs.NotAuthorized
	}
	if projectId == 0 {
		return nil, errs.ErrFull{Err: errs.InvalidRequest, Desc: "project_id is required", Fields: map[string]string{"project_id": "required"}}
	}
	if state != "" && state != "ok" && state != "error" {
		return nil, errs.ErrFull{Err: errs.InvalidRequest, Desc: "state: ok or error", Fields: map[string]string{"state": "invalid"}}
	}
	if err := u.accessSvc.CheckProject(ctx, projectId); err != nil {
		return nil, err
	}
	items, err := u.svc.ListSiteFetch(ctx, projectId, state, siteFetchLimit)
	if err != nil {
		return nil, fmt.Errorf("svc.ListSiteFetch: %w", err)
	}
	return items, nil
}
