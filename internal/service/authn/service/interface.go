package service

import (
	"context"

	sessionModel "github.com/rendau/zorko/internal/domain/session/model"
	usrModel "github.com/rendau/zorko/internal/domain/usr/model"
)

type sessionServiceI interface {
	FromToken(tokenStr string) (*sessionModel.Session, error)
}

type usrServiceI interface {
	GetByExtSub(ctx context.Context, extSub string) (*usrModel.Main, bool, error)
	GetByUsername(ctx context.Context, username string) (*usrModel.Main, bool, error)
	Create(ctx context.Context, obj *usrModel.Edit) (int64, error)
	Update(ctx context.Context, id int64, obj *usrModel.Edit) error
}

// claimsReaderI — claims внешнего токена: oidc проверяет подпись, proxy — нет
type claimsReaderI interface {
	Claims(ctx context.Context, token string) (map[string]any, error)
}
