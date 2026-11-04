package service

import (
	"context"
	"fmt"
	"time"

	"github.com/rendau/zorko/internal/domain/event/model"
	"github.com/rendau/zorko/internal/errs"
)

// ReadService — чтение событий для интерфейса (отдельно от записи обработчиком)
type ReadService struct {
	repoCh RepoChReadI
}

func NewRead(repoCh RepoChReadI) *ReadService {
	return &ReadService{repoCh: repoCh}
}

func (s *ReadService) IssueAggregates(ctx context.Context, f *model.StatFilter) ([]*model.IssueAgg, bool, error) {
	items, truncated, err := s.repoCh.IssueAggregates(ctx, f)
	if err != nil {
		return nil, false, fmt.Errorf("repoCh.IssueAggregates: %w", err)
	}
	return items, truncated, nil
}

func (s *ReadService) Series(ctx context.Context, f *model.StatFilter, step time.Duration) (map[int64][]int64, error) {
	result, err := s.repoCh.Series(ctx, f, step)
	if err != nil {
		return nil, fmt.Errorf("repoCh.Series: %w", err)
	}
	return result, nil
}

func (s *ReadService) TagTop(ctx context.Context, f *model.StatFilter, top int) ([]*model.TagDist, error) {
	result, err := s.repoCh.TagTop(ctx, f, top)
	if err != nil {
		return nil, fmt.Errorf("repoCh.TagTop: %w", err)
	}
	return result, nil
}

func (s *ReadService) TagKeys(ctx context.Context, f *model.StatFilter, limit int) ([]string, error) {
	result, err := s.repoCh.TagKeys(ctx, f, limit)
	if err != nil {
		return nil, fmt.Errorf("repoCh.TagKeys: %w", err)
	}
	return result, nil
}

func (s *ReadService) TagValues(ctx context.Context, f *model.StatFilter, key, prefix string, limit int) ([]string, error) {
	result, err := s.repoCh.TagValues(ctx, f, key, prefix, limit)
	if err != nil {
		return nil, fmt.Errorf("repoCh.TagValues: %w", err)
	}
	return result, nil
}

func (s *ReadService) ListEvents(ctx context.Context, f *model.StatFilter, offset, limit int64) ([]*model.EventSummary, int64, error) {
	items, total, err := s.repoCh.ListEvents(ctx, f, offset, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("repoCh.ListEvents: %w", err)
	}
	return items, total, nil
}

// GetEvent — errNE: ненайденное событие — errs.ObjectNotFound
func (s *ReadService) GetEvent(ctx context.Context, projectId, issueId int64, eventId string) (*model.EventDetail, error) {
	result, found, err := s.repoCh.GetEvent(ctx, projectId, issueId, eventId)
	if err != nil {
		return nil, fmt.Errorf("repoCh.GetEvent: %w", err)
	}
	if !found {
		return nil, errs.ObjectNotFound
	}
	return result, nil
}
