package keycache

import (
	"context"

	projectKeyModel "github.com/rendau/zorko/internal/domain/project_key/model"
)

// KeyCache — активные ключи проектов в памяти: приём не ходит в Postgres на каждый запрос (ADR-001)
type KeyCache interface {
	Find(projectId int64, publicKey string) (*projectKeyModel.Main, bool)
	Refresh(ctx context.Context) error
	Start(ctx context.Context)
	Wait()
}
