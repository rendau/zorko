package org_token

import (
	"context"

	orgModel "github.com/rendau/zorko/internal/domain/org/model"
	"github.com/rendau/zorko/internal/domain/org_token/model"
	sessionModel "github.com/rendau/zorko/internal/domain/session/model"
)

type SessionServiceI interface {
	FromContext(ctx context.Context) *sessionModel.Session
	CtxRequireAdmin(ctx context.Context) error
}

type ServiceI interface {
	List(ctx context.Context, pars *model.ListReq) ([]*model.Main, error)
	Create(ctx context.Context, orgId int64, orgSlug string, createdBy *int64, name string) (*model.Created, error)
	Delete(ctx context.Context, id int64) error
}

type OrgServiceI interface {
	Get(ctx context.Context, id int64, errNE bool) (*orgModel.Main, bool, error)
}
