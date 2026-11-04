package model

import (
	"time"

	domainModel "github.com/rendau/zorko/internal/domain/project/model"
)

type Upsert struct {
	PKId  int64
	NewId int64 // id новой записи, заполняется в Create через RETURNING

	CreatedAt  *time.Time
	ModifiedAt *time.Time
	OrgId      *int64
	Slug       *string
	Name       *string
	Platform   *string
}

func (m *Upsert) CreateColumnMap() map[string]any {
	result := make(map[string]any, 6)
	if m.CreatedAt != nil {
		result["created_at"] = *m.CreatedAt
	}
	if m.ModifiedAt != nil {
		result["modified_at"] = *m.ModifiedAt
	}
	if m.OrgId != nil {
		result["org_id"] = *m.OrgId
	}
	if m.Slug != nil {
		result["slug"] = *m.Slug
	}
	if m.Name != nil {
		result["name"] = *m.Name
	}
	if m.Platform != nil {
		result["platform"] = *m.Platform
	}
	return result
}

func (m *Upsert) UpdateColumnMap() map[string]any {
	return m.CreateColumnMap()
}

func (m *Upsert) PKColumnMap() map[string]any {
	return map[string]any{"id": m.PKId}
}

// ReturningColumnMap — Create использует RETURNING id, чтобы вернуть id новой записи
func (m *Upsert) ReturningColumnMap() map[string]any {
	return map[string]any{"id": &m.NewId}
}

// DTO

func DecodeUpsert(v *domainModel.Edit) *Upsert {
	return &Upsert{
		CreatedAt:  v.CreatedAt,
		ModifiedAt: v.ModifiedAt,
		OrgId:      v.OrgId,
		Slug:       v.Slug,
		Name:       v.Name,
		Platform:   v.Platform,
	}
}
