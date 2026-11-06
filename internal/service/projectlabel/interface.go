package projectlabel

import "context"

// ProjectLabel — имя проекта для меток метрик: набор значений конечен (только существующие
// проекты), поэтому номер проекта из чужого запроса не раздувает число рядов
type ProjectLabel interface {
	Label(projectId int64) string
	Refresh(ctx context.Context) error
	Start(ctx context.Context)
	Wait()
}
