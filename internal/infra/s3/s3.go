// Package s3 — тонкая обёртка над minio-go: файлы карт исходников, куски загрузки (ADR-009), сегменты записей (ADR-012)
package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/rendau/zorko/internal/infra/httpx"
)

type Config struct {
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	Secure    bool
}

type Client struct {
	mc     *minio.Client
	bucket string
	region string
}

func New(conf Config) (*Client, error) {
	mc, err := minio.New(conf.Endpoint, &minio.Options{
		Creds:     credentials.NewStaticV4(conf.AccessKey, conf.SecretKey, ""),
		Secure:    conf.Secure,
		Region:    conf.Region,
		Transport: httpx.NewTransport(httpx.Config{}),
	})
	if err != nil {
		return nil, fmt.Errorf("minio.New: %w", err)
	}
	return &Client{mc: mc, bucket: conf.Bucket, region: conf.Region}, nil
}

// EnsureBucket — создать бакет, если его нет (стенд; в эксплуатации бакет обычно уже есть)
func (c *Client) EnsureBucket(ctx context.Context) error {
	ok, err := c.mc.BucketExists(ctx, c.bucket)
	if err != nil {
		return fmt.Errorf("BucketExists: %w", err)
	}
	if ok {
		return nil
	}
	if err = c.mc.MakeBucket(ctx, c.bucket, minio.MakeBucketOptions{Region: c.region}); err != nil {
		return fmt.Errorf("MakeBucket: %w", err)
	}
	return nil
}

func (c *Client) Put(ctx context.Context, key string, data []byte) error {
	_, err := c.mc.PutObject(ctx, c.bucket, key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
		ContentType: "application/octet-stream",
	})
	if err != nil {
		return fmt.Errorf("PutObject %s: %w", key, err)
	}
	return nil
}

// Get — содержимое объекта; нет объекта → found=false без ошибки
func (c *Client) Get(ctx context.Context, key string, maxBytes int64) ([]byte, bool, error) {
	obj, err := c.mc.GetObject(ctx, c.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, false, fmt.Errorf("GetObject %s: %w", key, err)
	}
	defer obj.Close()

	data, err := io.ReadAll(io.LimitReader(obj, maxBytes+1))
	if err != nil {
		if isNotFound(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read %s: %w", key, err)
	}
	if int64(len(data)) > maxBytes {
		return nil, false, fmt.Errorf("object %s is larger than %d bytes", key, maxBytes)
	}
	return data, true, nil
}

// Open — поток объекта (для склейки кусков без чтения целиком в память)
func (c *Client) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := c.mc.GetObject(ctx, c.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("GetObject %s: %w", key, err)
	}
	return obj, nil
}

func (c *Client) Exists(ctx context.Context, key string) (bool, error) {
	_, err := c.mc.StatObject(ctx, c.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("StatObject %s: %w", key, err)
	}
	return true, nil
}

func (c *Client) Delete(ctx context.Context, keys ...string) error {
	for _, key := range keys {
		if err := c.mc.RemoveObject(ctx, c.bucket, key, minio.RemoveObjectOptions{}); err != nil && !isNotFound(err) {
			return fmt.Errorf("RemoveObject %s: %w", key, err)
		}
	}
	return nil
}

// ListOlder — ключи с префиксом, изменённые раньше before
func (c *Client) ListOlder(ctx context.Context, prefix string, before time.Time) ([]string, error) {
	result := make([]string, 0)
	for obj := range c.mc.ListObjects(ctx, c.bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("ListObjects %s: %w", prefix, obj.Err)
		}
		if obj.LastModified.Before(before) {
			result = append(result, obj.Key)
		}
	}
	return result, nil
}

// ListPrefixes — «папки» первого уровня под prefix (prefix оканчивается на "/"): "replay/20261106/"
func (c *Client) ListPrefixes(ctx context.Context, prefix string) ([]string, error) {
	result := make([]string, 0)
	for obj := range c.mc.ListObjects(ctx, c.bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: false}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("ListObjects %s: %w", prefix, obj.Err)
		}
		if strings.HasSuffix(obj.Key, "/") {
			result = append(result, obj.Key)
		}
	}
	return result, nil
}

// DeletePrefix удаляет все объекты под prefix; возвращает, сколько удалено
func (c *Client) DeletePrefix(ctx context.Context, prefix string) (int, error) {
	keys := make([]string, 0)
	for obj := range c.mc.ListObjects(ctx, c.bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
		if obj.Err != nil {
			return 0, fmt.Errorf("ListObjects %s: %w", prefix, obj.Err)
		}
		keys = append(keys, obj.Key)
	}

	objects := make(chan minio.ObjectInfo, len(keys))
	for _, key := range keys {
		objects <- minio.ObjectInfo{Key: key}
	}
	close(objects)

	var firstErr error
	failed := 0
	for res := range c.mc.RemoveObjectsWithResult(ctx, c.bucket, objects, minio.RemoveObjectsOptions{}) {
		if res.Err != nil && !isNotFound(res.Err) {
			failed++
			if firstErr == nil {
				firstErr = fmt.Errorf("RemoveObjects %s: %w", res.ObjectName, res.Err)
			}
		}
	}

	return len(keys) - failed, firstErr
}

func isNotFound(err error) bool {
	if resp, ok := errors.AsType[minio.ErrorResponse](err); ok {
		return resp.StatusCode == http.StatusNotFound || resp.Code == "NoSuchKey"
	}
	return false
}
