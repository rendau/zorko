package service

import (
	"context"
	"time"

	"github.com/rendau/zorko/internal/domain/oauth/model"
)

type RepoDbI interface {
	ListGrants(ctx context.Context, usrId int64) ([]*model.Grant, error)
	GetGrant(ctx context.Context, id string) (*model.Grant, bool, error)
	CreateGrant(ctx context.Context, obj *model.Grant, code *model.Token) (string, error)
	DeleteGrant(ctx context.Context, id string) error
	TouchGrantLastUsed(ctx context.Context, id string) error

	GetToken(ctx context.Context, hash string) (*model.Token, bool, error)
	ExchangeToken(ctx context.Context, hash, kind string, tokens []*model.Token) (bool, error)
	DeleteExpiredTokens(ctx context.Context) (int64, error)
	DeleteIdleGrants(ctx context.Context, createdBefore time.Time) (int64, error)
}
