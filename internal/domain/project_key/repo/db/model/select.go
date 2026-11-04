package model

import (
	"time"

	domainModel "github.com/rendau/zorko/internal/domain/project_key/model"
)

type Select struct {
	Id              int64
	CreatedAt       time.Time
	ModifiedAt      time.Time
	ProjectId       int64
	PublicKey       string
	Label           string
	Active          bool
	RateLimitPerSec int32
	AllowedOrigin   []string
}

func (m *Select) ListColumnMap() map[string]any {
	return map[string]any{
		"id":                 &m.Id,
		"created_at":         &m.CreatedAt,
		"modified_at":        &m.ModifiedAt,
		"project_id":         &m.ProjectId,
		"public_key":         &m.PublicKey,
		"label":              &m.Label,
		"active":             &m.Active,
		"rate_limit_per_sec": &m.RateLimitPerSec,
		"allowed_origin":     &m.AllowedOrigin,
	}
}

func (m *Select) PKColumnMap() map[string]any {
	return map[string]any{"id": m.Id}
}

func (m *Select) DefaultSortColumns() []string {
	return []string{"created_at"}
}

// DTO

func EncodeSelect(v *Select, _ int) *domainModel.Main {
	return &domainModel.Main{
		Id:              v.Id,
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
