package service

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

const (
	// tickInterval — период чистки; первая выполняется сразу на старте
	tickInterval = time.Hour

	// idleGrantTTL: подключение без токенов старше этого — брошенное согласие или истёкшее подключение
	idleGrantTTL = time.Hour
)

type oauthServiceI interface {
	DeleteExpiredTokens(ctx context.Context) (int64, error)
	DeleteIdleGrants(ctx context.Context, createdBefore time.Time) (int64, error)
}

type Service struct {
	oauthSvc oauthServiceI

	wg sync.WaitGroup
}

func New(oauthSvc oauthServiceI) *Service {
	return &Service{oauthSvc: oauthSvc}
}

// Start запускает периодическую чистку; останавливается отменой ctx. Копий сервиса может быть
// несколько: удаление истёкшего безвредно повторять, блокировка не нужна.
func (s *Service) Start(ctx context.Context) {
	s.wg.Go(func() {
		ticker := time.NewTicker(tickInterval)
		defer ticker.Stop()

		for {
			s.Cleanup(ctx)

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	})
}

func (s *Service) Wait() {
	s.wg.Wait()
}

// Cleanup удаляет истёкшие токены, затем подключения без токенов
func (s *Service) Cleanup(ctx context.Context) {
	tokens, err := s.oauthSvc.DeleteExpiredTokens(ctx)
	if err != nil {
		if ctx.Err() == nil {
			slog.Error("oauth: cleanup tokens", "error", err)
		}
		return
	}

	grants, err := s.oauthSvc.DeleteIdleGrants(ctx, time.Now().Add(-idleGrantTTL))
	if err != nil {
		if ctx.Err() == nil {
			slog.Error("oauth: cleanup grants", "error", err)
		}
		return
	}

	if tokens > 0 || grants > 0 {
		slog.Info("oauth: cleanup", "tokens", tokens, "grants", grants)
	}
}
