package service

import (
	"context"

	artifactModel "github.com/rendau/zorko/internal/domain/artifact/model"
	sitefetchModel "github.com/rendau/zorko/internal/service/sitefetch/model"
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
	// Fetch — nil без ошибки: скачивание в проекте выключено
	Fetch(ctx context.Context, projectId int64, absUrl string) (*sitefetchModel.Result, error)
}
