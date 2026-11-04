package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/rendau/zorko/internal/config"
	"github.com/rendau/zorko/internal/domain/project_key/model"
	"github.com/rendau/zorko/internal/errs"
)

type Service struct {
	repoDb RepoDbI
}

func New(repoDb RepoDbI) *Service {
	return &Service{repoDb: repoDb}
}

func (s *Service) List(ctx context.Context, pars *model.ListReq) ([]*model.Main, int64, error) {
	items, tCount, err := s.repoDb.List(ctx, pars)
	if err != nil {
		return nil, 0, fmt.Errorf("repoDb.List: %w", err)
	}
	for _, item := range items {
		item.Dsn = buildDsn(item.PublicKey, item.ProjectId)
	}
	return items, tCount, nil
}

func (s *Service) Get(ctx context.Context, id int64, errNE bool) (*model.Main, bool, error) {
	result, found, err := s.repoDb.Get(ctx, id)
	if err != nil {
		return nil, false, fmt.Errorf("repoDb.Get: %w", err)
	}
	if !found {
		if errNE {
			return nil, false, errs.ObjectNotFound
		}
		return nil, false, nil
	}
	result.Dsn = buildDsn(result.PublicKey, result.ProjectId)
	return result, true, nil
}

func (s *Service) Create(ctx context.Context, obj *model.Edit) (int64, error) {
	obj.ModifiedAt = nil // DB-default устанавливает значение

	publicKey, err := generatePublicKey()
	if err != nil {
		return 0, fmt.Errorf("generatePublicKey: %w", err)
	}
	obj.PublicKey = &publicKey

	newId, err := s.repoDb.Create(ctx, obj)
	if err != nil {
		return 0, fmt.Errorf("repoDb.Create: %w", err)
	}
	return newId, nil
}

func (s *Service) Update(ctx context.Context, id int64, obj *model.Edit) error {
	obj.ModifiedAt = new(time.Now())
	obj.ProjectId = nil // ключ не переезжает в другой проект
	obj.PublicKey = nil // и не меняет значение: иначе у фронтендеров молча сломается DSN

	if err := s.repoDb.Update(ctx, id, obj); err != nil {
		return fmt.Errorf("repoDb.Update: %w", err)
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	if err := s.repoDb.Delete(ctx, id); err != nil {
		return fmt.Errorf("repoDb.Delete: %w", err)
	}
	return nil
}

// generatePublicKey — 32 hex-символа, как у Sentry
func generatePublicKey() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// buildDsn — <схема>://<public_key>@<хост>[/<путь>]/<project_id>; без INGEST_PUBLIC_URL — пусто
func buildDsn(publicKey string, projectId int64) string {
	if config.Conf.IngestPublicUrl == "" {
		return ""
	}
	u, err := url.Parse(config.Conf.IngestPublicUrl)
	if err != nil || u.Host == "" {
		return ""
	}
	u.User = url.User(publicKey)
	u.Path = strings.TrimSuffix(u.Path, "/") + "/" + strconv.FormatInt(projectId, 10)
	u.RawQuery, u.Fragment = "", ""
	return u.String()
}
