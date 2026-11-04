package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha1" //nolint:gosec // протокол sentry-cli сверяет куски и архивы по SHA-1
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/rendau/zorko/internal/domain/artifact/model"
	"github.com/rendau/zorko/internal/errs"
)

const (
	// MaxFileBytes — предел одного файла архива в распакованном виде
	MaxFileBytes = 200 << 20
	// сжатый файл в S3 не больше распакованного предела
	maxStoredFileBytes = MaxFileBytes + 1<<20
)

var (
	sha1Re = regexp.MustCompile(`^[0-9a-f]{40}$`)
	uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

type Service struct {
	repoDb   RepoDbI
	repoBlob RepoBlobI
}

func New(repoDb RepoDbI, repoBlob RepoBlobI) *Service {
	return &Service{repoDb: repoDb, repoBlob: repoBlob}
}

// IsSha1 — 40 hex-символов в нижнем регистре
func IsSha1(v string) bool {
	return sha1Re.MatchString(v)
}

// NormalizeDebugId — метка в виде uuid в нижнем регистре; не uuid → ""
func NormalizeDebugId(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if uuidRe.MatchString(v) {
		return v
	}
	// метки бывают без дефисов и с суффиксом возраста (32 hex + appendix) — берём первые 32
	compact := strings.ReplaceAll(v, "-", "")
	if len(compact) >= 32 {
		c := compact[:32]
		if _, err := hex.DecodeString(c); err == nil {
			return c[0:8] + "-" + c[8:12] + "-" + c[12:16] + "-" + c[16:20] + "-" + c[20:32]
		}
	}
	return ""
}

func Sha1Hex(data []byte) string {
	sum := sha1.Sum(data) //nolint:gosec
	return hex.EncodeToString(sum[:])
}

// chunks

// PutChunk — кусок загрузки; имя обязано совпадать с SHA-1 содержимого
func (s *Service) PutChunk(ctx context.Context, checksum string, data []byte) error {
	if !IsSha1(checksum) || Sha1Hex(data) != checksum {
		return errs.ErrFull{Err: errs.ChunkInvalid, Desc: "chunk checksum mismatch: " + checksum}
	}
	exists, err := s.repoBlob.ChunkExists(ctx, checksum)
	if err != nil {
		return fmt.Errorf("repoBlob.ChunkExists: %w", err)
	}
	if exists {
		return nil
	}
	if err = s.repoBlob.PutChunk(ctx, checksum, data); err != nil {
		return fmt.Errorf("repoBlob.PutChunk: %w", err)
	}
	return nil
}

// MissingChunks — каких кусков нет в хранилище (порядок как в запросе, без повторов)
func (s *Service) MissingChunks(ctx context.Context, checksums []string) ([]string, error) {
	uniq := make([]string, 0, len(checksums))
	seen := make(map[string]bool, len(checksums))
	for _, c := range checksums {
		if !seen[c] {
			seen[c] = true
			uniq = append(uniq, c)
		}
	}
	exists := make([]bool, len(uniq))
	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(16)
	for i, c := range uniq {
		g.Go(func() error {
			ok, err := s.repoBlob.ChunkExists(gCtx, c)
			exists[i] = ok
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("repoBlob.ChunkExists: %w", err)
	}
	missing := make([]string, 0)
	for i, c := range uniq {
		if !exists[i] {
			missing = append(missing, c)
		}
	}
	return missing, nil
}

func (s *Service) OpenChunk(ctx context.Context, checksum string) (io.ReadCloser, error) {
	r, err := s.repoBlob.OpenChunk(ctx, checksum)
	if err != nil {
		return nil, fmt.Errorf("repoBlob.OpenChunk: %w", err)
	}
	return r, nil
}

func (s *Service) DeleteChunks(ctx context.Context, checksums []string) error {
	if err := s.repoBlob.DeleteChunks(ctx, checksums); err != nil {
		return fmt.Errorf("repoBlob.DeleteChunks: %w", err)
	}
	return nil
}

// files

// StoreFile — файл в S3 под SHA-1 содержимого (gzip); одинаковые файлы хранятся один раз
func (s *Service) StoreFile(ctx context.Context, data []byte) (string, error) {
	checksum := Sha1Hex(data)
	exists, err := s.repoBlob.FileExists(ctx, checksum)
	if err != nil {
		return "", fmt.Errorf("repoBlob.FileExists: %w", err)
	}
	if exists {
		return checksum, nil
	}
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	if _, err = zw.Write(data); err != nil {
		return "", fmt.Errorf("gzip: %w", err)
	}
	if err = zw.Close(); err != nil {
		return "", fmt.Errorf("gzip: %w", err)
	}
	if err = s.repoBlob.PutFile(ctx, checksum, buf.Bytes()); err != nil {
		return "", fmt.Errorf("repoBlob.PutFile: %w", err)
	}
	return checksum, nil
}

// ReadFile — распакованное содержимое файла; нет файла → found=false
func (s *Service) ReadFile(ctx context.Context, checksum string) ([]byte, bool, error) {
	gz, found, err := s.repoBlob.GetFile(ctx, checksum, maxStoredFileBytes)
	if err != nil || !found {
		return nil, false, err
	}
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return nil, false, fmt.Errorf("gzip.NewReader: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(zr, MaxFileBytes+1))
	if err != nil {
		return nil, false, fmt.Errorf("gunzip: %w", err)
	}
	if len(data) > MaxFileBytes {
		return nil, false, fmt.Errorf("file %s is larger than %d bytes", checksum, MaxFileBytes)
	}
	return data, true, nil
}

// bundles

func (s *Service) ClaimBundle(ctx context.Context, nb *model.NewBundle, staleAfter, errorRetryAfter time.Duration) (*model.Bundle, bool, error) {
	b, claimed, err := s.repoDb.ClaimBundle(ctx, nb, staleAfter, errorRetryAfter)
	if err != nil {
		return nil, false, fmt.Errorf("repoDb.ClaimBundle: %w", err)
	}
	return b, claimed, nil
}

func (s *Service) CompleteBundle(ctx context.Context, bundleId int64, release, dist string, files []*model.File) error {
	if err := s.repoDb.CompleteBundle(ctx, bundleId, release, dist, files); err != nil {
		return fmt.Errorf("repoDb.CompleteBundle: %w", err)
	}
	return nil
}

func (s *Service) FailBundle(ctx context.Context, bundleId int64, detail string) error {
	if err := s.repoDb.FailBundle(ctx, bundleId, detail); err != nil {
		return fmt.Errorf("repoDb.FailBundle: %w", err)
	}
	return nil
}

func (s *Service) GetBundleByChecksum(ctx context.Context, orgId int64, checksum string) (*model.Bundle, bool, error) {
	b, found, err := s.repoDb.GetBundleByChecksum(ctx, orgId, checksum)
	if err != nil {
		return nil, false, fmt.Errorf("repoDb.GetBundleByChecksum: %w", err)
	}
	return b, found, nil
}

func (s *Service) AddBundleProjects(ctx context.Context, bundleId int64, projectIds []int64) error {
	if err := s.repoDb.AddBundleProjects(ctx, bundleId, projectIds); err != nil {
		return fmt.Errorf("repoDb.AddBundleProjects: %w", err)
	}
	return nil
}

func (s *Service) GetBundle(ctx context.Context, id int64) (*model.Bundle, bool, error) {
	b, found, err := s.repoDb.GetBundle(ctx, id)
	if err != nil {
		return nil, false, fmt.Errorf("repoDb.GetBundle: %w", err)
	}
	return b, found, nil
}

// lookup

func (s *Service) FilesByDebugIds(ctx context.Context, projectId int64, debugIds []string) ([]*model.File, error) {
	ids := make([]string, 0, len(debugIds))
	for _, id := range debugIds {
		if n := NormalizeDebugId(id); n != "" {
			ids = append(ids, n)
		}
	}
	files, err := s.repoDb.FilesByDebugIds(ctx, projectId, ids)
	if err != nil {
		return nil, fmt.Errorf("repoDb.FilesByDebugIds: %w", err)
	}
	return files, nil
}

func (s *Service) FilesByReleaseUrls(ctx context.Context, projectId int64, release, dist string, urls []string) ([]*model.File, error) {
	files, err := s.repoDb.FilesByReleaseUrls(ctx, projectId, release, dist, urls)
	if err != nil {
		return nil, fmt.Errorf("repoDb.FilesByReleaseUrls: %w", err)
	}
	return files, nil
}

func (s *Service) FilesOfBundle(ctx context.Context, bundleId int64, urls []string) ([]*model.File, error) {
	files, err := s.repoDb.FilesOfBundle(ctx, bundleId, urls)
	if err != nil {
		return nil, fmt.Errorf("repoDb.FilesOfBundle: %w", err)
	}
	return files, nil
}

func (s *Service) MarkUsed(ctx context.Context, bundleIds []int64) error {
	if err := s.repoDb.MarkUsed(ctx, bundleIds); err != nil {
		return fmt.Errorf("repoDb.MarkUsed: %w", err)
	}
	return nil
}
