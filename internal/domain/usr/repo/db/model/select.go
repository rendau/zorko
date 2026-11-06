package model

import (
	"time"

	domainModel "github.com/rendau/zorko/internal/domain/usr/model"
)

type Select struct {
	Id         int64
	CreatedAt  time.Time
	ModifiedAt time.Time
	Active     bool
	IsAdmin    bool
	Name       string
	Username   string
	Password   string
	ProjectIds []int64
}

func (m *Select) ListColumnMap() map[string]any {
	return map[string]any{
		"id":          &m.Id,
		"created_at":  &m.CreatedAt,
		"modified_at": &m.ModifiedAt,
		"active":      &m.Active,
		"is_admin":    &m.IsAdmin,
		"name":        &m.Name,
		"username":    &m.Username,
		"password":    &m.Password,
		"project_ids": &m.ProjectIds,
	}
}

func (m *Select) PKColumnMap() map[string]any {
	return map[string]any{"id": m.Id}
}

func (m *Select) DefaultSortColumns() []string {
	return []string{"username"}
}

// DTO

func EncodeSelect(v *Select, _ int) *domainModel.Main {
	return &domainModel.Main{
		Id:         v.Id,
		CreatedAt:  v.CreatedAt,
		ModifiedAt: v.ModifiedAt,
		Active:     v.Active,
		IsAdmin:    v.IsAdmin,
		Name:       v.Name,
		Username:   v.Username,
		Password:   v.Password,
		ProjectIds: v.ProjectIds,
	}
}
