package projectlabel

import "context"

// ReplaySettings — настройки записи сессий проекта; проекта нет в кэше — режим "off"
type ReplaySettings struct {
	Mode      string
	HourLimit int32
}

// ProjectLabel — имя проекта для меток метрик: набор значений конечен (только существующие
// проекты), поэтому номер проекта из чужого запроса не раздувает число рядов
// Здесь же — настройки записи сессий для приёма (ADR-013): тот же список проектов, та же перечитка.
type ProjectLabel interface {
	Label(projectId int64) string
	Replay(projectId int64) ReplaySettings
	Refresh(ctx context.Context) error
	Start(ctx context.Context)
	Wait()
}
