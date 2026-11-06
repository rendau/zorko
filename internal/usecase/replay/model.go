package replay

import (
	"time"

	commonModel "github.com/rendau/zorko/internal/domain/common/model"
	replayModel "github.com/rendau/zorko/internal/domain/replay/model"
)

// ListReq — записи организации (или её отдельных проектов)
type ListReq struct {
	commonModel.ListParams

	OrgId       int64
	ProjectIds  []int64
	Environment string
	Release     string
	Query       string
	HasErrors   *bool
	Type        string
}

// ErrorMark — ошибка в записи: отметка на шкале и ссылка на проблему
type ErrorMark struct {
	IssueId   int64
	Title     string
	EventId   string
	Timestamp time.Time
}

type GetRep struct {
	Summary *replayModel.Summary
	Errors  []*ErrorMark
}
