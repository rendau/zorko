package service

import (
	"context"

	artifactModel "github.com/rendau/zorko/internal/domain/artifact/model"
)

type artifactServiceI interface {
	FilesByDebugIds(ctx context.Context, projectId int64, debugIds []string) ([]*artifactModel.File, error)
	FilesByReleaseUrls(ctx context.Context, projectId int64, release, dist string, urls []string) ([]*artifactModel.File, error)
	FilesOfBundle(ctx context.Context, bundleId int64, urls []string) ([]*artifactModel.File, error)
	ReadFile(ctx context.Context, checksum string) ([]byte, bool, error)
	MarkUsed(ctx context.Context, bundleIds []int64) error
}

// SiteFetcherI — карты с сайта (ADR-010); nil — не используется
type SiteFetcherI interface {
	// Fetch — сжатый JS и его карта для адреса absUrl; nil без ошибки — нечего брать (код причины в Code)
	Fetch(ctx context.Context, projectId int64, absUrl string) (*SiteResult, error)
}

// SiteResult — итог похода на сайт: файлы в архиве bundleId или код причины неудачи
type SiteResult struct {
	BundleId int64
	JsBlob   string
	MapBlob  string
	MapUrl   string
	Code     string // пусто — удача
}
