package service

import (
	"context"
	"fmt"
	"time"

	"github.com/rendau/zorko/internal/domain/oauth/model"
	"github.com/rendau/zorko/internal/errs"
)

type Service struct {
	repoDb RepoDbI
}

func New(repoDb RepoDbI) *Service {
	return &Service{repoDb: repoDb}
}

// ListGrants — состоявшиеся подключения человека, старые первыми
func (s *Service) ListGrants(ctx context.Context, usrId int64) ([]*model.Grant, error) {
	items, err := s.repoDb.ListGrants(ctx, usrId)
	if err != nil {
		return nil, fmt.Errorf("repoDb.ListGrants: %w", err)
	}
	return items, nil
}

func (s *Service) GetGrant(ctx context.Context, id string, errNE bool) (*model.Grant, bool, error) {
	result, found, err := s.repoDb.GetGrant(ctx, id)
	if err != nil {
		return nil, false, fmt.Errorf("repoDb.GetGrant: %w", err)
	}
	if !found {
		if errNE {
			return nil, false, errs.ObjectNotFound
		}
		return nil, false, nil
	}
	return result, true, nil
}

// CreateGrant заводит подключение и его одноразовый код авторизации; возвращает id и сам код —
// в БД попадает только его хеш
func (s *Service) CreateGrant(ctx context.Context, obj *model.Grant, codeTTL time.Duration) (string, string, error) {
	code, hash, err := GenerateToken(model.TokenKindCode)
	if err != nil {
		return "", "", fmt.Errorf("GenerateToken: %w", err)
	}

	id, err := s.repoDb.CreateGrant(ctx, obj, &model.Token{
		Hash:      hash,
		Kind:      model.TokenKindCode,
		ExpiresAt: time.Now().Add(codeTTL),
	})
	if err != nil {
		return "", "", fmt.Errorf("repoDb.CreateGrant: %w", err)
	}
	return id, code, nil
}

func (s *Service) DeleteGrant(ctx context.Context, id string) error {
	if err := s.repoDb.DeleteGrant(ctx, id); err != nil {
		return fmt.Errorf("repoDb.DeleteGrant: %w", err)
	}
	return nil
}

func (s *Service) TouchGrantLastUsed(ctx context.Context, id string) error {
	if err := s.repoDb.TouchGrantLastUsed(ctx, id); err != nil {
		return fmt.Errorf("repoDb.TouchGrantLastUsed: %w", err)
	}
	return nil
}

// FindToken возвращает живой (не истёкший) токен вида kind
func (s *Service) FindToken(ctx context.Context, token, kind string) (*model.Token, bool, error) {
	item, found, err := s.repoDb.GetToken(ctx, HashToken(token))
	if err != nil {
		return nil, false, fmt.Errorf("repoDb.GetToken: %w", err)
	}
	if !found || item.Kind != kind || !item.ExpiresAt.After(time.Now()) {
		return nil, false, nil
	}
	return item, true, nil
}

// FindAnyToken возвращает токен любого вида, в том числе истёкший
func (s *Service) FindAnyToken(ctx context.Context, token string) (*model.Token, bool, error) {
	item, found, err := s.repoDb.GetToken(ctx, HashToken(token))
	if err != nil {
		return nil, false, fmt.Errorf("repoDb.GetToken: %w", err)
	}
	return item, found, nil
}

// Exchange меняет одноразовый токен (код или refresh) на новые — по одному на каждый вид из ttls.
// false — токен уже использован: новых нет.
func (s *Service) Exchange(ctx context.Context, token, kind string, ttls map[string]time.Duration) (map[string]string, bool, error) {
	now := time.Now()

	result := make(map[string]string, len(ttls))
	records := make([]*model.Token, 0, len(ttls))
	for newKind, ttl := range ttls {
		value, hash, err := GenerateToken(newKind)
		if err != nil {
			return nil, false, fmt.Errorf("GenerateToken: %w", err)
		}
		result[newKind] = value
		records = append(records, &model.Token{Hash: hash, Kind: newKind, ExpiresAt: now.Add(ttl)})
	}

	ok, err := s.repoDb.ExchangeToken(ctx, HashToken(token), kind, records)
	if err != nil {
		return nil, false, fmt.Errorf("repoDb.ExchangeToken: %w", err)
	}
	if !ok {
		return nil, false, nil
	}
	return result, true, nil
}

// DeleteExpiredTokens удаляет истёкшие токены
func (s *Service) DeleteExpiredTokens(ctx context.Context) (int64, error) {
	count, err := s.repoDb.DeleteExpiredTokens(ctx)
	if err != nil {
		return 0, fmt.Errorf("repoDb.DeleteExpiredTokens: %w", err)
	}
	return count, nil
}

// DeleteIdleGrants удаляет подключения без токенов, созданные раньше createdBefore
func (s *Service) DeleteIdleGrants(ctx context.Context, createdBefore time.Time) (int64, error) {
	count, err := s.repoDb.DeleteIdleGrants(ctx, createdBefore)
	if err != nil {
		return 0, fmt.Errorf("repoDb.DeleteIdleGrants: %w", err)
	}
	return count, nil
}
