package model

import (
	"time"

	domainModel "github.com/rendau/zorko/internal/domain/org/model"
)

type Select struct {
	Id         int64
	CreatedAt  time.Time
	ModifiedAt time.Time
	Slug       string
	Name       string
}

func (m *Select) ListColumnMap() map[string]any {
	return map[string]any{
		"id":          &m.Id,
		"created_at":  &m.CreatedAt,
		"modified_at": &m.ModifiedAt,
		"slug":        &m.Slug,
		"name":        &m.Name,
	}
}

func (m *Select) PKColumnMap() map[string]any {
	return map[string]any{"id": m.Id}
}

func (m *Select) DefaultSortColumns() []string {
	return []string{"slug"}
}

// DTO

func EncodeSelect(v *Select, _ int) *domainModel.Main {
	return &domainModel.Main{
		Id:         v.Id,
		CreatedAt:  v.CreatedAt,
		ModifiedAt: v.ModifiedAt,
		Slug:       v.Slug,
		Name:       v.Name,
	}
}
