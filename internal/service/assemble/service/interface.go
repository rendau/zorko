package service

import (
	"context"
	"io"
	"time"

	artifactModel "github.com/rendau/zorko/internal/domain/artifact/model"
)

type artifactServiceI interface {
	MissingChunks(ctx context.Context, checksums []string) ([]string, error)
	OpenChunk(ctx context.Context, checksum string) (io.ReadCloser, error)
	DeleteChunks(ctx context.Context, checksums []string) error
	StoreFile(ctx context.Context, data []byte) (string, error)
	GetBundleByChecksum(ctx context.Context, orgId int64, checksum string) (*artifactModel.Bundle, bool, error)
	AddBundleProjects(ctx context.Context, bundleId int64, projectIds []int64) error
	ClaimBundle(ctx context.Context, nb *artifactModel.NewBundle, staleAfter, errorRetryAfter time.Duration) (*artifactModel.Bundle, bool, error)
	CompleteBundle(ctx context.Context, bundleId int64, release, dist string, files []*artifactModel.File) error
	FailBundle(ctx context.Context, bundleId int64, detail string) error
}
