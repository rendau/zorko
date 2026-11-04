package model

import (
	"time"

	domainModel "github.com/rendau/zorko/internal/domain/project/model"
)

type Select struct {
	Id         int64
	CreatedAt  time.Time
	ModifiedAt time.Time
	OrgId      int64
	Slug       string
	Name       string
	Platform   string

	SiteFetchEnabled     bool
	SiteFetchHost        []string
	SiteFetchHeaderName  string
	SiteFetchHeaderValue string
	SiteFetchIntervalMin int32
}

func (m *Select) ListColumnMap() map[string]any {
	return map[string]any{
		"id":          &m.Id,
		"created_at":  &m.CreatedAt,
		"modified_at": &m.ModifiedAt,
		"org_id":      &m.OrgId,
		"slug":        &m.Slug,
		"name":        &m.Name,
		"platform":    &m.Platform,

		"site_fetch_enabled":      &m.SiteFetchEnabled,
		"site_fetch_host":         &m.SiteFetchHost,
		"site_fetch_header_name":  &m.SiteFetchHeaderName,
		"site_fetch_header_value": &m.SiteFetchHeaderValue,
		"site_fetch_interval_min": &m.SiteFetchIntervalMin,
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
		OrgId:      v.OrgId,
		Slug:       v.Slug,
		Name:       v.Name,
		Platform:   v.Platform,

		SiteFetchEnabled:     v.SiteFetchEnabled,
		SiteFetchHost:        v.SiteFetchHost,
		SiteFetchHeaderName:  v.SiteFetchHeaderName,
		SiteFetchHeaderValue: v.SiteFetchHeaderValue,
		SiteFetchIntervalMin: v.SiteFetchIntervalMin,
	}
}
