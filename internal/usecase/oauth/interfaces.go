package oauth

import (
	"context"
	"time"

	"github.com/rendau/zorko/internal/domain/oauth/model"
	sessionModel "github.com/rendau/zorko/internal/domain/session/model"
	usrModel "github.com/rendau/zorko/internal/domain/usr/model"
)

type ServiceI interface {
	ListGrants(ctx context.Context, usrId int64) ([]*model.Grant, error)
	GetGrant(ctx context.Context, id string, errNE bool) (*model.Grant, bool, error)
	CreateGrant(ctx context.Context, obj *model.Grant, codeTTL time.Duration) (string, string, error)
	DeleteGrant(ctx context.Context, id string) error
	TouchGrantLastUsed(ctx context.Context, id string) error

	FindToken(ctx context.Context, token, kind string) (*model.Token, bool, error)
	FindAnyToken(ctx context.Context, token string) (*model.Token, bool, error)
	Exchange(ctx context.Context, token, kind string, ttls map[string]time.Duration) (map[string]string, bool, error)
}

type ClientCodecI interface {
	Encode(client *model.Client) (string, error)
	Decode(clientId string) (*model.Client, bool)
	Secret(clientId string) string
	VerifySecret(clientId, secret string) bool
}

type UsrServiceI interface {
	Get(ctx context.Context, id int64, errNE bool) (*usrModel.Main, bool, error)
}

type SessionServiceI interface {
	FromContext(ctx context.Context) *sessionModel.Session
}
