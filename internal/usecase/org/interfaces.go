package org

import (
	"context"

	"github.com/rendau/zorko/internal/domain/org/model"
	projectModel "github.com/rendau/zorko/internal/domain/project/model"
	accessModel "github.com/rendau/zorko/internal/service/access/model"
)

type SessionServiceI interface {
	CtxIsAuthorized(ctx context.Context) bool
	CtxIsAdmin(ctx context.Context) bool
}

type ServiceI interface {
	List(ctx context.Context, pars *model.ListReq) ([]*model.Main, int64, error)
	Get(ctx context.Context, id int64, errNE bool) (*model.Main, bool, error)
	Create(ctx context.Context, obj *model.Edit) (int64, error)
	Update(ctx context.Context, id int64, obj *model.Edit) error
	Delete(ctx context.Context, id int64) error
}

type ProjectServiceI interface {
	List(ctx context.Context, pars *projectModel.ListReq) ([]*projectModel.Main, int64, error)
}

type AccessServiceI interface {
	ProjectAccess(ctx context.Context) (*accessModel.ProjectAccess, error)
}
