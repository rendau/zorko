// Package upload — API для sentry-cli и @sentry/vite-plugin: заливка карт и релизы (ADR-009).
// Вход — токен организации (Bearer sntrys_…), а не сессия интерфейса.
package upload

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/samber/lo"

	"github.com/rendau/zorko/internal/config"
	projectModel "github.com/rendau/zorko/internal/domain/project/model"
	"github.com/rendau/zorko/internal/errs"
	assembleModel "github.com/rendau/zorko/internal/service/assemble/model"
)

// Настройки загрузки кусков (ответ chunk-upload)
const (
	ChunkSize        = 8 << 20
	ChunksPerRequest = 64
	MaxRequestSize   = 32 << 20
	MaxFileSize      = 1 << 30
	Concurrency      = 8
)

// Auth — проверенный токен CI
type Auth struct {
	OrgId     int64
	OrgSlug   string
	TokenId   int64
	TokenHint string
}

type ChunkOptions struct {
	Url              string
	ChunkSize        int64
	ChunksPerRequest int64
	MaxRequestSize   int64
	MaxFileSize      int64
	Concurrency      int64
	HashAlgorithm    string
	Compression      []string
	Accept           []string
}

type AssembleReq struct {
	Checksum string
	Chunks   []string
	Projects []string // slug-и проектов организации
	Version  string
	Dist     string
}

type Usecase struct {
	enabled     bool
	tokenSvc    TokenServiceI
	orgSvc      OrgServiceI
	projectSvc  ProjectServiceI
	artifactSvc ArtifactServiceI
	assembler   AssemblerI
	releaseSvc  ReleaseServiceI
}

// New — enabled=false, если S3 не настроен: заливка отвечает «карты выключены», релизы работают
func New(enabled bool, tokenSvc TokenServiceI, orgSvc OrgServiceI, projectSvc ProjectServiceI,
	artifactSvc ArtifactServiceI, assembler AssemblerI, releaseSvc ReleaseServiceI) *Usecase {
	return &Usecase{
		enabled: enabled, tokenSvc: tokenSvc, orgSvc: orgSvc, projectSvc: projectSvc,
		artifactSvc: artifactSvc, assembler: assembler, releaseSvc: releaseSvc,
	}
}

// Authorize — токен и организация из пути запроса (orgSlug пусто — любая организация токена)
func (u *Usecase) Authorize(ctx context.Context, raw, orgSlug string) (*Auth, error) {
	if raw == "" {
		return nil, errs.TokenInvalid
	}
	token, err := u.tokenSvc.Auth(ctx, raw)
	if err != nil {
		return nil, fmt.Errorf("tokenSvc.Auth: %w", err)
	}
	org, found, err := u.orgSvc.Get(ctx, token.OrgId, false)
	if err != nil {
		return nil, fmt.Errorf("orgSvc.Get: %w", err)
	}
	if !found {
		return nil, errs.TokenInvalid
	}
	if orgSlug != "" && orgSlug != org.Slug {
		return nil, errs.ErrFull{Err: errs.NoPermission, Desc: "token belongs to another organization"}
	}
	return &Auth{OrgId: org.Id, OrgSlug: org.Slug, TokenId: token.Id, TokenHint: token.Hint}, nil
}

func (u *Usecase) ChunkOptions(auth *Auth) (*ChunkOptions, error) {
	if !u.enabled {
		return nil, errs.SourcemapsDisabled
	}
	return &ChunkOptions{
		Url:              chunkUploadUrl(auth.OrgSlug),
		ChunkSize:        ChunkSize,
		ChunksPerRequest: ChunksPerRequest,
		MaxRequestSize:   MaxRequestSize,
		MaxFileSize:      MaxFileSize,
		Concurrency:      Concurrency,
		HashAlgorithm:    "sha1",
		Compression:      []string{"gzip"},
		Accept:           []string{"artifact_bundles", "artifact_bundles_v2"},
	}, nil
}

// chunkUploadUrl — абсолютный адрес при INGEST_PUBLIC_URL, иначе путь без /api/0 (его добавит sentry-cli)
func chunkUploadUrl(orgSlug string) string {
	path := "/organizations/" + url.PathEscape(orgSlug) + "/chunk-upload/"
	if config.Conf.IngestPublicUrl == "" {
		return path
	}
	return strings.TrimSuffix(config.Conf.IngestPublicUrl, "/") + "/api/0" + path
}

func (u *Usecase) PutChunk(ctx context.Context, _ *Auth, checksum string, data []byte) error {
	if !u.enabled {
		return errs.SourcemapsDisabled
	}
	if len(data) > ChunkSize {
		return errs.ErrFull{Err: errs.ChunkInvalid, Desc: fmt.Sprintf("chunk is larger than %d bytes", ChunkSize)}
	}
	if err := u.artifactSvc.PutChunk(ctx, checksum, data); err != nil {
		return fmt.Errorf("artifactSvc.PutChunk: %w", err)
	}
	return nil
}

func (u *Usecase) Assemble(ctx context.Context, auth *Auth, req *AssembleReq) (*assembleModel.Result, error) {
	if !u.enabled {
		return nil, errs.SourcemapsDisabled
	}
	projectIds, err := u.resolveProjects(ctx, auth.OrgId, req.Projects)
	if err != nil {
		return nil, err
	}
	res, err := u.assembler.Assemble(ctx, &assembleModel.Request{
		OrgId:      auth.OrgId,
		Checksum:   strings.ToLower(req.Checksum),
		Chunks:     lo.Map(req.Chunks, func(c string, _ int) string { return strings.ToLower(c) }),
		ProjectIds: projectIds,
		Release:    req.Version,
		Dist:       req.Dist,
	})
	if err != nil {
		return nil, fmt.Errorf("assembler.Assemble: %w", err)
	}
	return res, nil
}

// resolveProjects — проекты организации по slug или id (sentry-cli шлёт числа как id); нужен хотя бы
// один, все должны существовать
func (u *Usecase) resolveProjects(ctx context.Context, orgId int64, refs []string) ([]int64, error) {
	refs = lo.Uniq(lo.Filter(refs, func(s string, _ int) bool { return s != "" }))
	if len(refs) == 0 {
		return nil, errs.ErrFull{Err: errs.InvalidRequest, Desc: "projects are required"}
	}
	projects, _, err := u.projectSvc.List(ctx, &projectModel.ListReq{OrgId: &orgId})
	if err != nil {
		return nil, fmt.Errorf("projectSvc.List: %w", err)
	}
	bySlug := lo.SliceToMap(projects, func(p *projectModel.Main) (string, int64) { return p.Slug, p.Id })
	byId := lo.SliceToMap(projects, func(p *projectModel.Main) (string, int64) { return strconv.FormatInt(p.Id, 10), p.Id })
	ids := make([]int64, 0, len(refs))
	for _, ref := range refs {
		id, ok := bySlug[ref]
		if !ok {
			id, ok = byId[ref]
		}
		if !ok {
			return nil, errs.ErrFull{Err: errs.ProjectNotFound, Desc: "project not found: " + ref}
		}
		ids = append(ids, id)
	}
	return lo.Uniq(ids), nil
}
