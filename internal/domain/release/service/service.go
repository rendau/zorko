package service

import (
	"context"
	"fmt"
	"time"

	"github.com/rendau/zorko/internal/domain/release/model"
)

type RepoDbI interface {
	Get(ctx context.Context, orgId int64, version string) (*model.Main, bool, error)
	Ensure(ctx context.Context, orgId int64, version string, projectIds []int64, dateReleased *time.Time) (bool, error)
	SetDateReleased(ctx context.Context, id int64, at time.Time) error
}

type Service struct {
	repoDb RepoDbI
}

func New(repoDb RepoDbI) *Service {
	return &Service{repoDb: repoDb}
}

func (s *Service) Get(ctx context.Context, orgId int64, version string) (*model.Main, bool, error) {
	m, found, err := s.repoDb.Get(ctx, orgId, version)
	if err != nil {
		return nil, false, fmt.Errorf("repoDb.Get: %w", err)
	}
	return m, found, nil
}

// Ensure — релиз создан или уже был (created=false); возвращает его после изменений
func (s *Service) Ensure(ctx context.Context, orgId int64, version string, projectIds []int64, dateReleased *time.Time) (*model.Main, bool, error) {
	created, err := s.repoDb.Ensure(ctx, orgId, version, projectIds, dateReleased)
	if err != nil {
		return nil, false, fmt.Errorf("repoDb.Ensure: %w", err)
	}
	m, _, err := s.Get(ctx, orgId, version)
	if err != nil {
		return nil, false, err
	}
	return m, created, nil
}

func (s *Service) SetDateReleased(ctx context.Context, id int64, at time.Time) error {
	if err := s.repoDb.SetDateReleased(ctx, id, at); err != nil {
		return fmt.Errorf("repoDb.SetDateReleased: %w", err)
	}
	return nil
}
