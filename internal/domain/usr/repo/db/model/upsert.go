package model

import (
	"time"

	domainModel "github.com/rendau/zorko/internal/domain/usr/model"
)

type Upsert struct {
	PKId  int64
	NewId int64 // id новой записи, заполняется в Create через RETURNING

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

func (m *Upsert) CreateColumnMap() map[string]any {
	result := make(map[string]any, 10)
	if m.CreatedAt != nil {
		result["created_at"] = *m.CreatedAt
	}
	if m.ModifiedAt != nil {
		result["modified_at"] = *m.ModifiedAt
	}
	if m.Active != nil {
		result["active"] = *m.Active
	}
	if m.IsAdmin != nil {
		result["is_admin"] = *m.IsAdmin
	}
	if m.Name != nil {
		result["name"] = *m.Name
	}
	if m.Username != nil {
		result["username"] = *m.Username
	}
	if m.Password != nil {
		result["password"] = *m.Password
	}
	if m.ExtSub != nil {
		result["ext_sub"] = *m.ExtSub
	}
	if m.AllProjects != nil {
		result["all_projects"] = *m.AllProjects
	}
	if m.ProjectIds != nil {
		result["project_ids"] = *m.ProjectIds
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
		CreatedAt:   v.CreatedAt,
		ModifiedAt:  v.ModifiedAt,
		Active:      v.Active,
		IsAdmin:     v.IsAdmin,
		Name:        v.Name,
		Username:    v.Username,
		Password:    v.Password,
		ExtSub:      v.ExtSub,
		AllProjects: v.AllProjects,
		ProjectIds:  v.ProjectIds,
	}
}
