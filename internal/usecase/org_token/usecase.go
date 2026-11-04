package org_token

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/rendau/zorko/internal/domain/org_token/model"
	"github.com/rendau/zorko/internal/errs"
)

const nameMaxLen = 100

// Usecase — токены CI: всё только администратору (ADR-009)
type Usecase struct {
	svc        ServiceI
	orgSvc     OrgServiceI
	sessionSvc SessionServiceI
}

func New(svc ServiceI, orgSvc OrgServiceI, sessionSvc SessionServiceI) *Usecase {
	return &Usecase{svc: svc, orgSvc: orgSvc, sessionSvc: sessionSvc}
}

func (u *Usecase) List(ctx context.Context, orgId int64) ([]*model.Main, error) {
	if !u.sessionSvc.CtxIsAdmin(ctx) {
		return nil, errs.NoPermission
	}
	if orgId == 0 {
		return nil, errs.ErrFull{Err: errs.InvalidRequest, Desc: "org_id is required", Fields: map[string]string{"org_id": "required"}}
	}
	items, err := u.svc.List(ctx, &model.ListReq{OrgId: orgId})
	if err != nil {
		return nil, fmt.Errorf("svc.List: %w", err)
	}
	return items, nil
}

func (u *Usecase) Create(ctx context.Context, orgId int64, name string) (*model.Created, error) {
	if !u.sessionSvc.CtxIsAdmin(ctx) {
		return nil, errs.NoPermission
	}
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > nameMaxLen {
		return nil, errs.ErrFull{Err: errs.InvalidRequest, Desc: "name is required, up to 100 characters", Fields: map[string]string{"name": "invalid"}}
	}
	org, found, err := u.orgSvc.Get(ctx, orgId, false)
	if err != nil {
		return nil, fmt.Errorf("orgSvc.Get: %w", err)
	}
	if !found {
		return nil, errs.OrgNotFound
	}
	result, err := u.svc.Create(ctx, org.Id, org.Slug, new(u.sessionSvc.FromContext(ctx).Id), name)
	if err != nil {
		return nil, fmt.Errorf("svc.Create: %w", err)
	}
	return result, nil
}

func (u *Usecase) Delete(ctx context.Context, id int64) error {
	if id == 0 {
		return errs.IdRequired
	}
	if !u.sessionSvc.CtxIsAdmin(ctx) {
		return errs.NoPermission
	}
	if err := u.svc.Delete(ctx, id); err != nil {
		return fmt.Errorf("svc.Delete: %w", err)
	}
	return nil
}
