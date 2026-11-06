package service

import (
	"context"

	sessionModel "github.com/rendau/zorko/internal/domain/session/model"
	usrModel "github.com/rendau/zorko/internal/domain/usr/model"
)

type sessionServiceI interface {
	FromContext(ctx context.Context) *sessionModel.Session
}

type usrServiceI interface {
	Get(ctx context.Context, id int64, errNE bool) (*usrModel.Main, bool, error)
}
