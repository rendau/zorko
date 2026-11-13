package service

import (
	"context"
	"time"

	"github.com/rendau/zorko/internal/domain/issue/model"
)

type RepoDbI interface {
	ResolveHashes(ctx context.Context, keys []model.HashKey) (map[model.HashKey]int64, error)
	CreateWithHash(ctx context.Context, hit *model.Hit) (int64, error)
	Bump(ctx context.Context, bumps []*model.Bump, storeCap int64, minute time.Time) (map[int64]int64, error)

	Get(ctx context.Context, id int64) (*model.Main, bool, error)
	ListByIds(ctx context.Context, ids []int64) ([]*model.Main, error)
	SeenByIds(ctx context.Context, ids []int64) ([]*model.Seen, error)
	IdsByText(ctx context.Context, projectIds []int64, words []string, limit int) ([]int64, error)
}
