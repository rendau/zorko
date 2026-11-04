package org

import (
	"context"
	"fmt"

	"github.com/rendau/zorko/internal/domain/org/model"
	"github.com/rendau/zorko/internal/errs"
	"github.com/rendau/zorko/internal/util"
)

type Usecase struct {
	svc ServiceI
}

func New(svc ServiceI) *Usecase {
	return &Usecase{svc: svc}
}

func (u *Usecase) validateEdit(obj *model.Edit, forCreate bool) error {
	return util.ValidateSlug(obj.Slug, forCreate)
}

func (u *Usecase) List(ctx context.Context, pars *model.ListReq) ([]*model.Main, int64, error) {
	if err := util.RequirePageSize(pars.ListParams, 0); err != nil {
		return nil, 0, err
	}
	items, tCount, err := u.svc.List(ctx, pars)
	if err != nil {
		return nil, 0, fmt.Errorf("svc.List: %w", err)
	}
	return items, tCount, nil
}

func (u *Usecase) Get(ctx context.Context, id int64) (*model.Main, error) {
	result, _, err := u.svc.Get(ctx, id, true)
	if err != nil {
		return nil, fmt.Errorf("svc.Get: %w", err)
	}
	return result, nil
}

func (u *Usecase) Create(ctx context.Context, obj *model.Edit) (int64, error) {
	// TODO: check auth/permissions (шаг 5)
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
	// TODO: check auth/permissions (шаг 5)
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
	// TODO: check auth/permissions (шаг 5)
	if err := u.svc.Delete(ctx, id); err != nil {
		return fmt.Errorf("svc.Delete: %w", err)
	}
	return nil
}
