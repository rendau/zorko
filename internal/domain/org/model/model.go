package model

import (
	"time"

	commonModel "github.com/rendau/zorko/internal/domain/common/model"
)

// Main — организация: верхний уровень, внутри — проекты
type Main struct {
	Id         int64
	CreatedAt  time.Time
	ModifiedAt time.Time
	Slug       string
	Name       string
}

// Edit — мутация (partial create/update)
type Edit struct {
	CreatedAt  *time.Time
	ModifiedAt *time.Time
	Slug       *string
	Name       *string
}

// ListReq — параметры выборки
type ListReq struct {
	commonModel.ListParams

	Slug *string
	IDs  *[]int64
}
