package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/samber/lo"

	"github.com/rendau/zorko/internal/domain/issue/model"
	"github.com/rendau/zorko/internal/errs"
)

type Service struct {
	repoDb RepoDbI
}

func New(repoDb RepoDbI) *Service {
	return &Service{repoDb: repoDb}
}

// Record — счётчики проблем по пачке (ADR-003, ADR-004): находит или заводит проблему для каждого
// отпечатка, прибавляет события и выдаёт, сколько полных записей можно сохранить в минуту minute.
// Результат — в порядке hits. Разрешение выдаётся на проблему: если два отпечатка пачки ведут к
// одной проблеме, оно целиком достаётся первому. Отпечаток удалённого проекта получает IssueId = 0.
func (s *Service) Record(ctx context.Context, hits []*model.Hit, storeCap int64, minute time.Time) ([]*model.HitResult, error) {
	if len(hits) == 0 {
		return nil, nil
	}

	keys := lo.Map(hits, func(h *model.Hit, _ int) model.HashKey {
		return model.HashKey{ProjectId: h.ProjectId, Hash: h.Hash}
	})

	issueIds, err := s.repoDb.ResolveHashes(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("repoDb.ResolveHashes: %w", err)
	}

	for i, hit := range hits {
		if _, ok := issueIds[keys[i]]; ok {
			continue
		}
		issueId, err := s.repoDb.CreateWithHash(ctx, hit)
		if errors.Is(err, errs.ProjectNotFound) {
			// проект удалили, а его события ещё лежали в очереди
			issueIds[keys[i]] = 0
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("repoDb.CreateWithHash: %w", err)
		}
		issueIds[keys[i]] = issueId
	}

	// прибавки по проблеме (несколько отпечатков могут вести к одной)
	bumps := map[int64]*model.Bump{}
	for i, hit := range hits {
		issueId := issueIds[keys[i]]
		if issueId == 0 {
			continue
		}
		b, ok := bumps[issueId]
		if !ok {
			bumps[issueId] = &model.Bump{
				IssueId:     issueId,
				Count:       hit.Count,
				FirstSeen:   hit.FirstSeen,
				LastSeen:    hit.LastSeen,
				LastRelease: hit.LastRelease,
			}
			continue
		}
		b.Count += hit.Count
		b.FirstSeen = lo.Ternary(hit.FirstSeen.Before(b.FirstSeen), hit.FirstSeen, b.FirstSeen)
		if hit.LastSeen.After(b.LastSeen) {
			b.LastSeen, b.LastRelease = hit.LastSeen, lo.CoalesceOrEmpty(hit.LastRelease, b.LastRelease)
		}
	}

	bumpList := lo.Values(bumps)
	slices.SortFunc(bumpList, func(a, b *model.Bump) int { return int(a.IssueId - b.IssueId) })

	allowed, err := s.repoDb.Bump(ctx, bumpList, storeCap, minute)
	if err != nil {
		return nil, fmt.Errorf("repoDb.Bump: %w", err)
	}

	return lo.Map(hits, func(_ *model.Hit, i int) *model.HitResult {
		issueId := issueIds[keys[i]]
		result := &model.HitResult{IssueId: issueId, StoreAllowed: allowed[issueId]}
		allowed[issueId] = 0
		return result
	}), nil
}
