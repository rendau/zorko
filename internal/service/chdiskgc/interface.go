package chdiskgc

import "context"

// Collector — предохранитель диска ClickHouse (ADR-017): одна проверка на все копии
type Collector interface {
	RunOnce(ctx context.Context) error
	Start(ctx context.Context)
	Wait()
}
