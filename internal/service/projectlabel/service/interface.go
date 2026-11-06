package service

import (
	"context"

	projectModel "github.com/rendau/zorko/internal/domain/project/model"
)

type projectServiceI interface {
	List(ctx context.Context, pars *projectModel.ListReq) ([]*projectModel.Main, int64, error)
}
