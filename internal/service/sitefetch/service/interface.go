package service

import (
	"context"
	"time"

	artifactModel "github.com/rendau/zorko/internal/domain/artifact/model"
	projectModel "github.com/rendau/zorko/internal/domain/project/model"
	"github.com/rendau/zorko/internal/util"
)

type projectServiceI interface {
	Get(ctx context.Context, id int64, errNE bool) (*projectModel.Main, bool, error)
}

type artifactServiceI interface {
	GetSiteFetch(ctx context.Context, projectId int64, url string) (*artifactModel.SiteFetch, bool, error)
	SaveSiteFetch(ctx context.Context, m *artifactModel.SiteFetch) error
	FilesOfBundle(ctx context.Context, bundleId int64, urls []string) ([]*artifactModel.File, error)
	StoreFile(ctx context.Context, data []byte) (string, error)
	ClaimBundle(ctx context.Context, nb *artifactModel.NewBundle, staleAfter, errorRetryAfter time.Duration) (*artifactModel.Bundle, bool, error)
	CompleteBundle(ctx context.Context, bundleId int64, release, dist string, files []*artifactModel.File) error
}

var decodeDataUrl = util.DecodeDataUrl
