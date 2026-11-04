// Package blob — файлы архивов и куски загрузки в S3 (ADR-009)
package blob

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/rendau/zorko/internal/infra/s3"
)

const (
	chunkPrefix = "chunk/"
	filePrefix  = "file/"
)

type Repo struct {
	s3 *s3.Client
}

func New(client *s3.Client) *Repo {
	return &Repo{s3: client}
}

func (r *Repo) PutChunk(ctx context.Context, sha1 string, data []byte) error {
	return r.s3.Put(ctx, chunkPrefix+sha1, data)
}

func (r *Repo) ChunkExists(ctx context.Context, sha1 string) (bool, error) {
	return r.s3.Exists(ctx, chunkPrefix+sha1)
}

func (r *Repo) OpenChunk(ctx context.Context, sha1 string) (io.ReadCloser, error) {
	return r.s3.Open(ctx, chunkPrefix+sha1)
}

func (r *Repo) DeleteChunks(ctx context.Context, sha1s []string) error {
	keys := make([]string, 0, len(sha1s))
	for _, s := range sha1s {
		keys = append(keys, chunkPrefix+s)
	}
	return r.s3.Delete(ctx, keys...)
}

// OldChunks — куски старше before (брошенные загрузки)
func (r *Repo) OldChunks(ctx context.Context, before time.Time) ([]string, error) {
	keys, err := r.s3.ListOlder(ctx, chunkPrefix, before)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(keys))
	for _, k := range keys {
		result = append(result, k[len(chunkPrefix):])
	}
	return result, nil
}

func (r *Repo) FileExists(ctx context.Context, sha1 string) (bool, error) {
	return r.s3.Exists(ctx, filePrefix+sha1)
}

// PutFile — сжатое (gzip) содержимое файла
func (r *Repo) PutFile(ctx context.Context, sha1 string, gz []byte) error {
	return r.s3.Put(ctx, filePrefix+sha1, gz)
}

// GetFile — сжатое содержимое; нет файла → found=false
func (r *Repo) GetFile(ctx context.Context, sha1 string, maxBytes int64) ([]byte, bool, error) {
	data, found, err := r.s3.Get(ctx, filePrefix+sha1, maxBytes)
	if err != nil {
		return nil, false, fmt.Errorf("s3.Get: %w", err)
	}
	return data, found, nil
}

func (r *Repo) DeleteFiles(ctx context.Context, sha1s []string) error {
	keys := make([]string, 0, len(sha1s))
	for _, s := range sha1s {
		keys = append(keys, filePrefix+s)
	}
	return r.s3.Delete(ctx, keys...)
}
