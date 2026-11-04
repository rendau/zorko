package assemble

import (
	"context"

	"github.com/rendau/zorko/internal/service/assemble/model"
)

// Assembler — сборка архивов карт из кусков загрузки в фоне (ADR-009)
type Assembler interface {
	Assemble(ctx context.Context, req *model.Request) (*model.Result, error)
	Start(ctx context.Context)
	Wait()
}
