package service

import (
	"context"

	projectKeyModel "github.com/rendau/zorko/internal/domain/project_key/model"
)

type projectKeyServiceI interface {
	List(ctx context.Context, pars *projectKeyModel.ListReq) ([]*projectKeyModel.Main, int64, error)
}
