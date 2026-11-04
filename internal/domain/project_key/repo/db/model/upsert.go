package model

import (
	"time"

	domainModel "github.com/rendau/zorko/internal/domain/project_key/model"
)

type Upsert struct {
	PKId  int64
	NewId int64 // id новой записи, заполняется в Create через RETURNING

	CreatedAt       *time.Time
	ModifiedAt      *time.Time
	ProjectId       *int64
	PublicKey       *string
	Label           *string
	Active          *bool
	RateLimitPerSec *int32
	AllowedOrigin   *[]string
}

func (m *Upsert) CreateColumnMap() map[string]any {
	result := make(map[string]any, 8)
	if m.CreatedAt != nil {
		result["created_at"] = *m.CreatedAt
	}
	if m.ModifiedAt != nil {
		result["modified_at"] = *m.ModifiedAt
	}
	if m.ProjectId != nil {
		result["project_id"] = *m.ProjectId
	}
	if m.PublicKey != nil {
		result["public_key"] = *m.PublicKey
	}
	if m.Label != nil {
		result["label"] = *m.Label
	}
	if m.Active != nil {
		result["active"] = *m.Active
	}
	if m.RateLimitPerSec != nil {
		result["rate_limit_per_sec"] = *m.RateLimitPerSec
	}
	if m.AllowedOrigin != nil {
		result["allowed_origin"] = *m.AllowedOrigin
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
		CreatedAt:       v.CreatedAt,
		ModifiedAt:      v.ModifiedAt,
		ProjectId:       v.ProjectId,
		PublicKey:       v.PublicKey,
		Label:           v.Label,
		Active:          v.Active,
		RateLimitPerSec: v.RateLimitPerSec,
		AllowedOrigin:   v.AllowedOrigin,
	}
}
