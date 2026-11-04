package service

import (
	"context"
	"io"
	"time"

	"github.com/rendau/zorko/internal/domain/artifact/model"
)

type RepoDbI interface {
	ClaimBundle(ctx context.Context, nb *model.NewBundle, staleAfter, errorRetryAfter time.Duration) (*model.Bundle, bool, error)
	AddBundleProjects(ctx context.Context, bundleId int64, projectIds []int64) error
	CompleteBundle(ctx context.Context, bundleId int64, release, dist string, files []*model.File) error
	FailBundle(ctx context.Context, bundleId int64, detail string) error
	GetBundle(ctx context.Context, id int64) (*model.Bundle, bool, error)
	GetBundleByChecksum(ctx context.Context, orgId int64, checksum string) (*model.Bundle, bool, error)

	FilesByDebugIds(ctx context.Context, projectId int64, debugIds []string) ([]*model.File, error)
	FilesByReleaseUrls(ctx context.Context, projectId int64, release, dist string, urls []string) ([]*model.File, error)
	FilesOfBundle(ctx context.Context, bundleId int64, urls []string) ([]*model.File, error)
	MarkUsed(ctx context.Context, bundleIds []int64) error

	GetSiteFetch(ctx context.Context, projectId int64, url string) (*model.SiteFetch, bool, error)
	SaveSiteFetch(ctx context.Context, m *model.SiteFetch) error
	ListSiteFetch(ctx context.Context, projectId int64, state string, limit int) ([]*model.SiteFetch, error)

	TryLock(ctx context.Context, key int64) (func(), bool, error)
	TouchBlob(ctx context.Context, sha1 string, size int64) error
	DeleteStaleBundles(ctx context.Context, retention time.Duration, limit int) (int64, error)
	OrphanBlobs(ctx context.Context, olderThan time.Duration, limit int) ([]string, error)
	DeleteBlobs(ctx context.Context, sha1s []string, olderThan time.Duration) ([]string, error)
	DeleteOldSiteFetch(ctx context.Context, olderThan time.Duration) (int64, error)
}

type RepoBlobI interface {
	PutChunk(ctx context.Context, sha1 string, data []byte) error
	ChunkExists(ctx context.Context, sha1 string) (bool, error)
	OpenChunk(ctx context.Context, sha1 string) (io.ReadCloser, error)
	DeleteChunks(ctx context.Context, sha1s []string) error
	OldChunks(ctx context.Context, before time.Time) ([]string, error)

	FileExists(ctx context.Context, sha1 string) (bool, error)
	PutFile(ctx context.Context, sha1 string, gz []byte) error
	GetFile(ctx context.Context, sha1 string, maxBytes int64) ([]byte, bool, error)
	DeleteFiles(ctx context.Context, sha1s []string) error
}
