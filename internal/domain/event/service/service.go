package service

import (
	"context"
	"fmt"

	"github.com/rendau/zorko/internal/domain/event/model"
)

type Service struct {
	repoCh RepoChI
}

func New(repoCh RepoChI) *Service {
	return &Service{repoCh: repoCh}
}

// Store пишет все события коротко и полные записи только для full (ADR-003)
func (s *Service) Store(ctx context.Context, all []*model.Event, full []*model.Event) error {
	if err := s.repoCh.InsertStats(ctx, all); err != nil {
		return fmt.Errorf("repoCh.InsertStats: %w", err)
	}
	if err := s.repoCh.InsertEvents(ctx, full); err != nil {
		return fmt.Errorf("repoCh.InsertEvents: %w", err)
	}
	return nil
}
