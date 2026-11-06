package org

import (
	"context"
	"fmt"
	"slices"

	"github.com/samber/lo"

	"github.com/rendau/zorko/internal/domain/org/model"
	projectModel "github.com/rendau/zorko/internal/domain/project/model"
	"github.com/rendau/zorko/internal/errs"
	"github.com/rendau/zorko/internal/util"
)

type Usecase struct {
	svc        ServiceI
	sessionSvc SessionServiceI
	projectSvc ProjectServiceI
	accessSvc  AccessServiceI
}

func New(svc ServiceI, sessionSvc SessionServiceI, projectSvc ProjectServiceI, accessSvc AccessServiceI) *Usecase {
	return &Usecase{svc: svc, sessionSvc: sessionSvc, projectSvc: projectSvc, accessSvc: accessSvc}
}

// visibleOrgIds — организации видимых проектов; nil — видны все
func (u *Usecase) visibleOrgIds(ctx context.Context) ([]int64, error) {
	access, err := u.accessSvc.ProjectAccess(ctx)
	if err != nil {
		return nil, err
	}
	if access.All {
		return nil, nil
	}
	projects, _, err := u.projectSvc.List(ctx, &projectModel.ListReq{IDs: &access.Ids})
	if err != nil {
		return nil, fmt.Errorf("projectSvc.List: %w", err)
	}
	return lo.Uniq(lo.Map(projects, func(p *projectModel.Main, _ int) int64 { return p.OrgId })), nil
}

func (u *Usecase) validateEdit(obj *model.Edit, forCreate bool) error {
	return util.ValidateSlug(obj.Slug, forCreate)
}

func (u *Usecase) List(ctx context.Context, pars *model.ListReq) ([]*model.Main, int64, error) {
	if !u.sessionSvc.CtxIsAuthorized(ctx) {
		return nil, 0, errs.NotAuthorized
	}
	if err := util.RequirePageSize(pars.ListParams, 0); err != nil {
		return nil, 0, err
	}
	orgIds, err := u.visibleOrgIds(ctx)
	if err != nil {
		return nil, 0, err
	}
	if orgIds != nil {
		// участник видит только организации своих проектов
		pars.IDs = new(lo.Ternary(pars.IDs == nil, orgIds, lo.Intersect(orgIds, lo.FromPtr(pars.IDs))))
		if len(*pars.IDs) == 0 {
			return []*model.Main{}, 0, nil
		}
	}
	items, tCount, err := u.svc.List(ctx, pars)
	if err != nil {
		return nil, 0, fmt.Errorf("svc.List: %w", err)
	}
	return items, tCount, nil
}

func (u *Usecase) Get(ctx context.Context, id int64) (*model.Main, error) {
	if !u.sessionSvc.CtxIsAuthorized(ctx) {
		return nil, errs.NotAuthorized
	}
	orgIds, err := u.visibleOrgIds(ctx)
	if err != nil {
		return nil, err
	}
	if orgIds != nil && !slices.Contains(orgIds, id) {
		return nil, errs.NoPermission
	}
	result, _, err := u.svc.Get(ctx, id, true)
	if err != nil {
		return nil, fmt.Errorf("svc.Get: %w", err)
	}
	return result, nil
}

func (u *Usecase) Create(ctx context.Context, obj *model.Edit) (int64, error) {
	if err := u.sessionSvc.CtxRequireAdmin(ctx); err != nil {
		return 0, err
	}
	if err := u.validateEdit(obj, true); err != nil {
		return 0, err
	}
	newId, err := u.svc.Create(ctx, obj)
	if err != nil {
		return 0, fmt.Errorf("svc.Create: %w", err)
	}
	return newId, nil
}

func (u *Usecase) Update(ctx context.Context, id int64, obj *model.Edit) error {
	if id == 0 {
		return errs.IdRequired
	}
	if err := u.sessionSvc.CtxRequireAdmin(ctx); err != nil {
		return err
	}
	if err := u.validateEdit(obj, false); err != nil {
		return err
	}
	if err := u.svc.Update(ctx, id, obj); err != nil {
		return fmt.Errorf("svc.Update: %w", err)
	}
	return nil
}

func (u *Usecase) Delete(ctx context.Context, id int64) error {
	if id == 0 {
		return errs.IdRequired
	}
	if err := u.sessionSvc.CtxRequireAdmin(ctx); err != nil {
		return err
	}
	if err := u.svc.Delete(ctx, id); err != nil {
		return fmt.Errorf("svc.Delete: %w", err)
	}
	return nil
}
