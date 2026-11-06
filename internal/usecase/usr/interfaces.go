package usr

import (
	"context"

	projectModel "github.com/rendau/zorko/internal/domain/project/model"
	sessionModel "github.com/rendau/zorko/internal/domain/session/model"
	"github.com/rendau/zorko/internal/domain/usr/model"
)

type ServiceI interface {
	List(ctx context.Context, pars *model.ListReq) ([]*model.Main, int64, error)
	Get(ctx context.Context, id int64, errNE bool) (*model.Main, bool, error)
	AuthByUsernamePassword(ctx context.Context, username, password string) (*model.Main, bool, error)
	HasAny(ctx context.Context) (bool, error)
	Create(ctx context.Context, obj *model.Edit) (int64, error)
	Update(ctx context.Context, id int64, obj *model.Edit) error
	Delete(ctx context.Context, id int64) error
}

type SessionServiceI interface {
	FromContext(ctx context.Context) *sessionModel.Session
	CtxIsAuthorized(ctx context.Context) bool
	CtxRequireAdmin(ctx context.Context) error
	CreateToken(usrId int64, isAdmin bool) (string, error)
	CreateRefreshToken(usrId int64, passwordHash string) (string, error)
	RefreshTokenUserId(tokenStr string) (int64, error)
	CheckRefreshToken(tokenStr, currentPasswordHash string) error
}

type LoginLimiterI interface {
	Allowed(key string) bool
	Fail(key string)
}

type ProjectServiceI interface {
	List(ctx context.Context, pars *projectModel.ListReq) ([]*projectModel.Main, int64, error)
}
