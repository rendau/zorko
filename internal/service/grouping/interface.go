package grouping

import (
	eventModel "github.com/rendau/zorko/internal/domain/event/model"
)

// Grouper — отпечаток события (по нему одинаковые ошибки склеиваются в проблему), заголовок и место падения
type Grouper interface {
	Group(ev *eventModel.Event) eventModel.Grouping
}
