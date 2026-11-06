package service

import (
	"context"

	"github.com/rendau/zorko/internal/domain/chdisk/model"
)

type RepoChI interface {
	Disk(ctx context.Context) (*model.Disk, error)
	Partitions(ctx context.Context, tables []string) ([]*model.Partition, error)
	DropPartition(ctx context.Context, table, id string) error
}

type RepoDbI interface {
	TryLock(ctx context.Context, key int64) (func(), bool, error)
}
