package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/samber/lo"

	projectKeyModel "github.com/rendau/zorko/internal/domain/project_key/model"
)

type Service struct {
	projectKeyService projectKeyServiceI
	refreshInterval   time.Duration

	// public_key → ключ; карта целиком заменяется при перечитке
	keys atomic.Pointer[map[string]*projectKeyModel.Main]

	wg sync.WaitGroup
}

func New(projectKeyService projectKeyServiceI, refreshInterval time.Duration) *Service {
	s := &Service{
		projectKeyService: projectKeyService,
		refreshInterval:   refreshInterval,
	}
	s.keys.Store(new(map[string]*projectKeyModel.Main{}))
	return s
}

// Find — активный ключ проекта; ключ чужого проекта не находится
func (s *Service) Find(projectId int64, publicKey string) (*projectKeyModel.Main, bool) {
	key, ok := (*s.keys.Load())[publicKey]
	if !ok || key.ProjectId != projectId {
		return nil, false
	}
	return key, true
}

// Refresh перечитывает все активные ключи
func (s *Service) Refresh(ctx context.Context) error {
	items, _, err := s.projectKeyService.List(ctx, &projectKeyModel.ListReq{Active: new(true)})
	if err != nil {
		return fmt.Errorf("projectKeyService.List: %w", err)
	}

	s.keys.Store(new(lo.KeyBy(items, func(v *projectKeyModel.Main) string { return v.PublicKey })))

	return nil
}

// Start загружает ключи сразу и затем перечитывает их каждые refreshInterval до отмены ctx
func (s *Service) Start(ctx context.Context) {
	if err := s.Refresh(ctx); err != nil {
		slog.Error("keycache: initial refresh", "error", err)
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
					slog.Error("keycache: refresh", "error", err)
				}
			}
		}
	})
}

func (s *Service) Wait() {
	s.wg.Wait()
}
