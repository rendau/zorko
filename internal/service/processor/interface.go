package processor

import "context"

// Processor — фоновая обработка событий из очереди (ADR-002, ADR-003)
type Processor interface {
	Start(ctx context.Context)
	Wait()
}
