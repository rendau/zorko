package service

import (
	"context"
	"fmt"
	"time"

	"github.com/rendau/zorko/internal/domain/usr/model"
	"github.com/rendau/zorko/internal/errs"
)

type Service struct {
	repoDb RepoDbI
}

func New(repoDb RepoDbI) *Service {
	return &Service{repoDb: repoDb}
}

func (s *Service) List(ctx context.Context, pars *model.ListReq) ([]*model.Main, int64, error) {
	items, tCount, err := s.repoDb.List(ctx, pars)
	if err != nil {
		return nil, 0, fmt.Errorf("repoDb.List: %w", err)
	}
	return items, tCount, nil
}

func (s *Service) Get(ctx context.Context, id int64, errNE bool) (*model.Main, bool, error) {
	result, found, err := s.repoDb.Get(ctx, id)
	if err != nil {
		return nil, false, fmt.Errorf("repoDb.Get: %w", err)
	}
	if !found {
		if errNE {
			return nil, false, errs.ObjectNotFound
		}
		return nil, false, nil
	}
	return result, true, nil
}

// GetByUsername — человек по логину (без проверки пароля)
func (s *Service) GetByUsername(ctx context.Context, username string) (*model.Main, bool, error) {
	result, found, err := s.repoDb.GetByUsername(ctx, username)
	if err != nil {
		return nil, false, fmt.Errorf("repoDb.GetByUsername: %w", err)
	}
	return result, found, nil
}

// GetByExtSub — человек по id у внешнего провайдера входа (ADR-019)
func (s *Service) GetByExtSub(ctx context.Context, extSub string) (*model.Main, bool, error) {
	result, found, err := s.repoDb.GetByExtSub(ctx, extSub)
	if err != nil {
		return nil, false, fmt.Errorf("repoDb.GetByExtSub: %w", err)
	}
	return result, found, nil
}

// AuthByUsernamePassword — человек по логину и паролю; неверная пара — found = false
func (s *Service) AuthByUsernamePassword(ctx context.Context, username, password string) (*model.Main, bool, error) {
	result, found, err := s.repoDb.GetByUsername(ctx, username)
	if err != nil {
		return nil, false, fmt.Errorf("repoDb.GetByUsername: %w", err)
	}
	// у внешней учётки пароля нет: по паролю она не входит
	if !found || result.Password == "" {
		return nil, false, nil
	}

	ok, err := comparePassword(result.Password, password)
	if err != nil {
		return nil, false, fmt.Errorf("comparePassword: %w", err)
	}
	if !ok {
		return nil, false, nil
	}
	return result, true, nil
}

func (s *Service) HasAny(ctx context.Context) (bool, error) {
	result, err := s.repoDb.HasAny(ctx)
	if err != nil {
		return false, fmt.Errorf("repoDb.HasAny: %w", err)
	}
	return result, nil
}

func (s *Service) Create(ctx context.Context, obj *model.Edit) (int64, error) {
	obj.ModifiedAt = nil // DB-default устанавливает значение

	if err := hashEditPassword(obj); err != nil {
		return 0, err
	}

	newId, err := s.repoDb.Create(ctx, obj)
	if err != nil {
		return 0, fmt.Errorf("repoDb.Create: %w", err)
	}
	return newId, nil
}

func (s *Service) Update(ctx context.Context, id int64, obj *model.Edit) error {
	obj.ModifiedAt = new(time.Now())

	if err := hashEditPassword(obj); err != nil {
		return err
	}

	if err := s.repoDb.Update(ctx, id, obj); err != nil {
		return fmt.Errorf("repoDb.Update: %w", err)
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	if err := s.repoDb.Delete(ctx, id); err != nil {
		return fmt.Errorf("repoDb.Delete: %w", err)
	}
	return nil
}

func hashEditPassword(obj *model.Edit) error {
	if obj.Password == nil {
		return nil
	}
	hash, err := hashPassword(*obj.Password)
	if err != nil {
		return fmt.Errorf("hashPassword: %w", err)
	}
	obj.Password = &hash
	return nil
}
