package service

import (
	"context"

	"github.com/rendau/zorko/internal/domain/event/model"
)

type RepoChI interface {
	InsertStats(ctx context.Context, events []*model.Event) error
	InsertEvents(ctx context.Context, events []*model.Event) error
}
