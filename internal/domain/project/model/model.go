package model

import (
	"time"

	commonModel "github.com/rendau/zorko/internal/domain/common/model"
)

// Main — проект: одно приложение внутри организации, у него свои ключи и проблемы
type Main struct {
	Id         int64
	CreatedAt  time.Time
	ModifiedAt time.Time
	OrgId      int64
	Slug       string
	Name       string
	Platform   string

	// карты с сайта (ADR-010)
	SiteFetchEnabled     bool
	SiteFetchHost        []string
	SiteFetchHeaderName  string
	SiteFetchHeaderValue string // секрет: наружу (в ответы ручек) не отдаётся
	SiteFetchIntervalMin int32
}

// Edit — мутация (partial create/update)
type Edit struct {
	CreatedAt  *time.Time
	ModifiedAt *time.Time
	OrgId      *int64
	Slug       *string
	Name       *string
	Platform   *string

	SiteFetchEnabled     *bool
	SiteFetchHost        *[]string
	SiteFetchHeaderName  *string
	SiteFetchHeaderValue *string
	SiteFetchIntervalMin *int32
}

// ListReq — параметры выборки
type ListReq struct {
	commonModel.ListParams

	OrgId *int64
	Slug  *string
	IDs   *[]int64
}
