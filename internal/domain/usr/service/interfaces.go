package service

import (
	"context"

	"github.com/rendau/zorko/internal/domain/usr/model"
)

type RepoDbI interface {
	List(ctx context.Context, pars *model.ListReq) ([]*model.Main, int64, error)
	Get(ctx context.Context, id int64) (*model.Main, bool, error)
	GetByUsername(ctx context.Context, username string) (*model.Main, bool, error)
	HasAny(ctx context.Context) (bool, error)
	Create(ctx context.Context, obj *model.Edit) (int64, error)
	Update(ctx context.Context, id int64, obj *model.Edit) error
	Delete(ctx context.Context, id int64) error
}
