// Package mem — oauth-репозиторий в памяти для тестов: с ним гоняется настоящий доменный сервис
package mem

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"sync"
	"time"

	"github.com/rendau/zorko/internal/domain/oauth/model"
)

type Repo struct {
	mu     sync.Mutex
	grants map[string]*model.Grant
	tokens map[string]*model.Token
	seq    int
}

func New() *Repo {
	return &Repo{grants: map[string]*model.Grant{}, tokens: map[string]*model.Token{}}
}

func (r *Repo) ListGrants(_ context.Context, usrId int64) ([]*model.Grant, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]*model.Grant, 0)
	for _, g := range r.grants {
		if g.UsrId == usrId && g.ConnectedAt != nil {
			result = append(result, new(*g))
		}
	}
	return result, nil
}

func (r *Repo) GetGrant(_ context.Context, id string) (*model.Grant, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g, ok := r.grants[id]
	if !ok {
		return nil, false, nil
	}
	return new(*g), true, nil
}

func (r *Repo) CreateGrant(_ context.Context, obj *model.Grant, code *model.Token) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	g := new(*obj)
	g.Id = "grant-" + strconv.Itoa(r.seq)
	g.CreatedAt = time.Now()
	r.grants[g.Id] = g
	code.GrantId = g.Id
	r.tokens[code.Hash] = new(*code)
	return g.Id, nil
}

func (r *Repo) DeleteGrant(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.grants, id)
	for hash, t := range r.tokens {
		if t.GrantId == id {
			delete(r.tokens, hash)
		}
	}
	return nil
}

func (r *Repo) TouchGrantLastUsed(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if g, ok := r.grants[id]; ok {
		g.LastUsedAt = new(time.Now())
	}
	return nil
}

func (r *Repo) GetToken(_ context.Context, hash string) (*model.Token, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tokens[hash]
	if !ok {
		return nil, false, nil
	}
	return new(*t), true, nil
}

func (r *Repo) ExchangeToken(_ context.Context, hash, kind string, tokens []*model.Token) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	old, ok := r.tokens[hash]
	if !ok || old.Kind != kind || !old.ExpiresAt.After(time.Now()) {
		return false, nil
	}
	delete(r.tokens, hash)
	if g, ok := r.grants[old.GrantId]; ok && g.ConnectedAt == nil {
		g.ConnectedAt = new(time.Now())
	}
	for _, t := range tokens {
		t.GrantId = old.GrantId
		r.tokens[t.Hash] = new(*t)
	}
	return true, nil
}

func (r *Repo) DeleteExpiredTokens(context.Context) (int64, error) { return 0, nil }

func (r *Repo) DeleteIdleGrants(context.Context, time.Time) (int64, error) { return 0, nil }

// Expire — сделать токен истёкшим
func (r *Repo) Expire(token string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tokens[hashToken(token)].ExpiresAt = time.Now().Add(-time.Second)
}

func (r *Repo) GrantCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.grants)
}

// hashToken — как service.HashToken (сервис импортировать нельзя: он зависит от репозитория)
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
