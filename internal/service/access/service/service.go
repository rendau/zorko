package service

import (
	"context"
	"fmt"

	"github.com/rendau/zorko/internal/errs"
	"github.com/rendau/zorko/internal/service/access/model"
)

type Service struct {
	sessionService sessionServiceI
	usrService     usrServiceI
}

func New(sessionService sessionServiceI, usrService usrServiceI) *Service {
	return &Service{sessionService: sessionService, usrService: usrService}
}

// ProjectAccess — человек перечитывается из БД на каждый запрос: смена списка проектов и
// отключение действуют сразу, не дожидаясь нового access-токена
func (s *Service) ProjectAccess(ctx context.Context) (*model.ProjectAccess, error) {
	session := s.sessionService.FromContext(ctx)
	if !session.IsAuthorized() {
		return nil, errs.NotAuthorized
	}

	usr, found, err := s.usrService.Get(ctx, session.Id, false)
	if err != nil {
		return nil, fmt.Errorf("usrService.Get: %w", err)
	}
	if !found || !usr.Active {
		return nil, errs.NotAuthorized
	}

	// пустой список без AllProjects — ни одного проекта: так заводится человек при внешнем входе
	if usr.IsAdmin || usr.AllProjects {
		return &model.ProjectAccess{All: true}, nil
	}
	return &model.ProjectAccess{Ids: usr.ProjectIds}, nil
}

// CheckProject — NoPermission, если проект человеку не виден
func (s *Service) CheckProject(ctx context.Context, projectId int64) error {
	access, err := s.ProjectAccess(ctx)
	if err != nil {
		return err
	}
	if !access.Allows(projectId) {
		return errs.NoPermission
	}
	return nil
}
