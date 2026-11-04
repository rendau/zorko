package upload

import (
	"context"

	orgModel "github.com/rendau/zorko/internal/domain/org/model"
	orgTokenModel "github.com/rendau/zorko/internal/domain/org_token/model"
	projectModel "github.com/rendau/zorko/internal/domain/project/model"
	assembleModel "github.com/rendau/zorko/internal/service/assemble/model"
)

type TokenServiceI interface {
	Auth(ctx context.Context, raw string) (*orgTokenModel.Main, error)
}

type OrgServiceI interface {
	Get(ctx context.Context, id int64, errNE bool) (*orgModel.Main, bool, error)
}

type ProjectServiceI interface {
	List(ctx context.Context, pars *projectModel.ListReq) ([]*projectModel.Main, int64, error)
}

type ArtifactServiceI interface {
	PutChunk(ctx context.Context, checksum string, data []byte) error
}

type AssemblerI interface {
	Assemble(ctx context.Context, req *assembleModel.Request) (*assembleModel.Result, error)
}
