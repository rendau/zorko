// Package blob — тела сегментов записей в S3 (ADR-012)
package blob

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rendau/zorko/internal/domain/replay/model"
	"github.com/rendau/zorko/internal/infra/s3"
)

type Repo struct {
	s3 *s3.Client
}

func New(client *s3.Client) *Repo {
	return &Repo{s3: client}
}

func (r *Repo) Put(ctx context.Context, key string, data []byte) error {
	return r.s3.Put(ctx, key, data)
}

// Get — тело сегмента; нет объекта → found=false
func (r *Repo) Get(ctx context.Context, key string, maxBytes int64) ([]byte, bool, error) {
	data, found, err := r.s3.Get(ctx, key, maxBytes)
	if err != nil {
		return nil, false, fmt.Errorf("s3.Get: %w", err)
	}
	return data, found, nil
}

// Days — дни, за которые в S3 есть записи (по папкам replay/YYYYMMDD/); чужие имена пропускаются
func (r *Repo) Days(ctx context.Context) ([]time.Time, error) {
	prefixes, err := r.s3.ListPrefixes(ctx, model.KeyPrefix)
	if err != nil {
		return nil, fmt.Errorf("s3.ListPrefixes: %w", err)
	}
	result := make([]time.Time, 0, len(prefixes))
	for _, p := range prefixes {
		name := strings.TrimSuffix(strings.TrimPrefix(p, model.KeyPrefix), "/")
		day, err := time.Parse("20060102", name)
		if err != nil {
			continue
		}
		result = append(result, day)
	}
	return result, nil
}

// DeleteDay удаляет папку дня целиком; возвращает число удалённых сегментов
func (r *Repo) DeleteDay(ctx context.Context, day time.Time) (int, error) {
	n, err := r.s3.DeletePrefix(ctx, model.DayPrefix(day))
	if err != nil {
		return n, fmt.Errorf("s3.DeletePrefix: %w", err)
	}
	return n, nil
}
