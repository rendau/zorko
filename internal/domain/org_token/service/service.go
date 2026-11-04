package service

import (
	"context"
	"fmt"
	"time"

	"github.com/rendau/zorko/internal/config"
	"github.com/rendau/zorko/internal/domain/org_token/model"
	"github.com/rendau/zorko/internal/errs"
)

type Service struct {
	repoDb RepoDbI
}

func New(repoDb RepoDbI) *Service {
	return &Service{repoDb: repoDb}
}

func (s *Service) List(ctx context.Context, pars *model.ListReq) ([]*model.Main, error) {
	items, err := s.repoDb.List(ctx, pars)
	if err != nil {
		return nil, fmt.Errorf("repoDb.List: %w", err)
	}
	return items, nil
}

func (s *Service) Get(ctx context.Context, id int64, errNE bool) (*model.Main, bool, error) {
	item, found, err := s.repoDb.Get(ctx, id)
	if err != nil {
		return nil, false, fmt.Errorf("repoDb.Get: %w", err)
	}
	if !found && errNE {
		return nil, false, errs.ObjectNotFound
	}
	return item, found, nil
}

// Create — новый токен; значение возвращается один раз и не хранится
func (s *Service) Create(ctx context.Context, orgId int64, orgSlug string, createdBy *int64, name string) (*model.Created, error) {
	raw, err := generateToken(config.Conf.IngestPublicUrl, orgSlug, time.Now())
	if err != nil {
		return nil, fmt.Errorf("generateToken: %w", err)
	}
	id, err := s.repoDb.Create(ctx, orgId, createdBy, name, tokenHint(raw), hashToken(raw))
	if err != nil {
		return nil, fmt.Errorf("repoDb.Create: %w", err)
	}
	item, _, err := s.Get(ctx, id, true)
	if err != nil {
		return nil, err
	}
	return &model.Created{Main: *item, Token: raw}, nil
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	if err := s.repoDb.Delete(ctx, id); err != nil {
		return fmt.Errorf("repoDb.Delete: %w", err)
	}
	return nil
}

// Auth — токен CI по его значению; неверный формат, неизвестный или удалённый → errs.TokenInvalid.
// Проверка всегда по базе (без кэша): удалённый токен перестаёт работать сразу.
func (s *Service) Auth(ctx context.Context, raw string) (*model.Main, error) {
	if _, ok := parseToken(raw); !ok {
		return nil, errs.TokenInvalid
	}
	item, found, err := s.repoDb.GetByHash(ctx, hashToken(raw))
	if err != nil {
		return nil, fmt.Errorf("repoDb.GetByHash: %w", err)
	}
	if !found {
		return nil, errs.TokenInvalid
	}
	if err = s.repoDb.TouchUsed(ctx, item.Id); err != nil {
		return nil, fmt.Errorf("repoDb.TouchUsed: %w", err)
	}
	return item, nil
}
