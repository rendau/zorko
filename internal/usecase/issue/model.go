package issue

import (
	"time"

	commonModel "github.com/rendau/zorko/internal/domain/common/model"
	issueModel "github.com/rendau/zorko/internal/domain/issue/model"
)

// Scope — где искать: организация, проекты (пусто — все проекты организации), окружение, период
type Scope struct {
	OrgId       int64
	ProjectIds  []int64
	Environment string
	Period      string
}

type ListReq struct {
	commonModel.ListParams
	Scope

	Query string
	Sort  string // last_seen, first_seen, events, users
}

type ListItem struct {
	Issue     *issueModel.Main
	Events    int64
	Users     int64
	Sparkline []int64
}

type ListRep struct {
	Items     []*ListItem
	Total     int64
	Truncated bool
}

type GetRep struct {
	Issue  *issueModel.Main
	Events int64
	Users  int64
}

type StatPoint struct {
	Ts    time.Time
	Count int64
}
