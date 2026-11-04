package service

import (
	"context"

	"github.com/rendau/zorko/internal/domain/org_token/model"
)

type RepoDbI interface {
	List(ctx context.Context, pars *model.ListReq) ([]*model.Main, error)
	Get(ctx context.Context, id int64) (*model.Main, bool, error)
	GetByHash(ctx context.Context, hash []byte) (*model.Main, bool, error)
	Create(ctx context.Context, orgId int64, createdBy *int64, name, hint string, hash []byte) (int64, error)
	Delete(ctx context.Context, id int64) error
	TouchUsed(ctx context.Context, id int64) error
}
