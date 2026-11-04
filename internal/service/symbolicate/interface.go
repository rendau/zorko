package symbolicate

import (
	"context"

	eventModel "github.com/rendau/zorko/internal/domain/event/model"
)

// Symbolicator — перевод сжатых кадров стека в исходные по картам (ADR-008): меняет кадры модели
// (их видит группировка) и JSON события (его видит интерфейс). Не обязателен для приёма: при сбоях
// кадры остаются как есть с причиной.
type Symbolicator interface {
	Symbolicate(ctx context.Context, events []*eventModel.Event)
	Start(ctx context.Context)
	Wait()
}
