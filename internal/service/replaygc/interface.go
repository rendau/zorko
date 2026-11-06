package replaygc

import "context"

// Collector — уборка записей сессий раз в сутки (ADR-012): одна на все копии
type Collector interface {
	RunOnce(ctx context.Context) error
	Start(ctx context.Context)
	Wait()
}
