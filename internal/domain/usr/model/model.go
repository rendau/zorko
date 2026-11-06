package model

import (
	"time"

	commonModel "github.com/rendau/zorko/internal/domain/common/model"
)

// Main — человек, который входит в интерфейс
type Main struct {
	Id         int64
	CreatedAt  time.Time
	ModifiedAt time.Time
	Active     bool
	IsAdmin    bool
	Name       string
	Username   string
	Password   string // bcrypt-хеш; наружу не отдаётся
	// ProjectIds — проекты, которые видит участник; пусто — все. Админу не ограничивает.
	ProjectIds []int64
}

// Edit — мутация (partial create/update); Password — открытый пароль, хешируется в domain service
type Edit struct {
	CreatedAt  *time.Time
	ModifiedAt *time.Time
	Active     *bool
	IsAdmin    *bool
	Name       *string
	Username   *string
	Password   *string
	ProjectIds *[]int64
}

// ListReq — параметры выборки
type ListReq struct {
	commonModel.ListParams

	Active *bool
	IDs    *[]int64
}
