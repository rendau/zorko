package model

import (
	"time"

	commonModel "github.com/rendau/zorko/internal/domain/common/model"
)

// Main — ключ проекта: по нему SDK Sentry шлют ошибки в zorko
type Main struct {
	Id              int64
	CreatedAt       time.Time
	ModifiedAt      time.Time
	ProjectId       int64
	PublicKey       string
	Label           string
	Active          bool
	RateLimitPerSec int32    // защитный предел событий в секунду на копию приёма; 0 — без предела
	AllowedOrigin   []string // хосты или маски вида *.mdev.kz; пусто — любые
	Dsn             string   // вычисляется из INGEST_PUBLIC_URL, не хранится
}

// Edit — мутация (partial create/update)
type Edit struct {
	CreatedAt       *time.Time
	ModifiedAt      *time.Time
	ProjectId       *int64
	PublicKey       *string
	Label           *string
	Active          *bool
	RateLimitPerSec *int32
	AllowedOrigin   *[]string
}

// ListReq — параметры выборки
type ListReq struct {
	commonModel.ListParams

	ProjectId *int64
	Active    *bool
	IDs       *[]int64
}
