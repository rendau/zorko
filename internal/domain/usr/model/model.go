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
	Password   string // bcrypt-хеш; наружу не отдаётся; у внешних учёток пусто
	// ExtSub — постоянный id человека у внешнего провайдера входа (claim sub, ADR-019);
	// пусто — локальная учётка
	ExtSub string
	// AllProjects — видит все проекты; иначе только ProjectIds (пусто — ни одного).
	// Админу не ограничивает.
	AllProjects bool
	ProjectIds  []int64
}

// Edit — мутация (partial create/update); Password — открытый пароль, хешируется в domain service
type Edit struct {
	CreatedAt   *time.Time
	ModifiedAt  *time.Time
	Active      *bool
	IsAdmin     *bool
	Name        *string
	Username    *string
	Password    *string
	ExtSub      *string
	AllProjects *bool
	ProjectIds  *[]int64
}

// ListReq — параметры выборки
type ListReq struct {
	commonModel.ListParams

	Active *bool
	IDs    *[]int64
}
