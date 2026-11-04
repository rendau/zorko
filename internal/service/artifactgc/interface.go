package artifactgc

import "context"

// Collector — уборка карт раз в сутки (спека, раздел 8): одна на все копии
type Collector interface {
	RunOnce(ctx context.Context) error
	Start(ctx context.Context)
	Wait()
}
