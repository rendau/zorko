package normalize

import (
	eventModel "github.com/rendau/zorko/internal/domain/event/model"
)

// Normalizer — разбор события SDK Sentry, вычистка чувствительных данных и приведение к единому виду
type Normalizer interface {
	Normalize(raw *eventModel.RawEvent) (*eventModel.Event, error)
}
