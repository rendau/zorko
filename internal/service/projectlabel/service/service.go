package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/samber/lo"

	projectModel "github.com/rendau/zorko/internal/domain/project/model"
)

// LabelUnknown — проект не найден (нет такого или создан после последней перечитки)
const LabelUnknown = "unknown"

type Service struct {
	projectService  projectServiceI
	refreshInterval time.Duration

	// project_id → метка; карта целиком заменяется при перечитке
	labels atomic.Pointer[map[int64]string]

	wg sync.WaitGroup
}

func New(projectService projectServiceI, refreshInterval time.Duration) *Service {
	s := &Service{
		projectService:  projectService,
		refreshInterval: refreshInterval,
	}
	s.labels.Store(new(map[int64]string{}))
	return s
}

// Label — slug проекта; slug, повторяющийся в разных организациях, дополняется номером проекта
func (s *Service) Label(projectId int64) string {
	if label, ok := (*s.labels.Load())[projectId]; ok {
		return label
	}
	return LabelUnknown
}

// Refresh перечитывает все проекты
func (s *Service) Refresh(ctx context.Context) error {
	items, _, err := s.projectService.List(ctx, &projectModel.ListReq{})
	if err != nil {
		return fmt.Errorf("projectService.List: %w", err)
	}

	s.labels.Store(new(buildLabels(items)))

	return nil
}

func buildLabels(items []*projectModel.Main) map[int64]string {
	slugCount := lo.CountValuesBy(items, func(v *projectModel.Main) string { return v.Slug })

	return lo.SliceToMap(items, func(v *projectModel.Main) (int64, string) {
		if slugCount[v.Slug] > 1 {
			return v.Id, v.Slug + "-" + strconv.FormatInt(v.Id, 10)
		}
		return v.Id, v.Slug
	})
}

// Start загружает проекты сразу и затем перечитывает их каждые refreshInterval до отмены ctx
func (s *Service) Start(ctx context.Context) {
	if err := s.Refresh(ctx); err != nil {
		slog.Error("projectlabel: initial refresh", "error", err)
	}

	s.wg.Go(func() {
		ticker := time.NewTicker(s.refreshInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.Refresh(ctx); err != nil && ctx.Err() == nil {
					slog.Error("projectlabel: refresh", "error", err)
				}
			}
		}
	})
}

func (s *Service) Wait() {
	s.wg.Wait()
}
