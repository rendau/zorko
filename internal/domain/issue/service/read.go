package service

import (
	"context"
	"fmt"

	"github.com/rendau/zorko/internal/domain/issue/model"
	"github.com/rendau/zorko/internal/errs"
)

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

func (s *Service) ListByIds(ctx context.Context, ids []int64) ([]*model.Main, error) {
	result, err := s.repoDb.ListByIds(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("repoDb.ListByIds: %w", err)
	}
	return result, nil
}

func (s *Service) IdsByText(ctx context.Context, projectIds []int64, words []string, limit int) ([]int64, error) {
	result, err := s.repoDb.IdsByText(ctx, projectIds, words, limit)
	if err != nil {
		return nil, fmt.Errorf("repoDb.IdsByText: %w", err)
	}
	return result, nil
}
