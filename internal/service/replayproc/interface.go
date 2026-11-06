package replayproc

import "context"

// ReplayProc — обработка описаний сегментов записей: тема zorko.replays → ClickHouse (ADR-012)
type ReplayProc interface {
	Start(ctx context.Context)
	Wait()
}
