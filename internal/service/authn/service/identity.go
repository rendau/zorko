package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	sessionModel "github.com/rendau/zorko/internal/domain/session/model"
	usrModel "github.com/rendau/zorko/internal/domain/usr/model"
	"github.com/rendau/zorko/internal/errs"
)

// identity — кто пришёл по claims внешнего токена
type identity struct {
	sub      string
	username string
	name     string
	admin    bool
}

// identityFromClaims — без sub человека не узнать: такой токен не годится
func identityFromClaims(claims map[string]any, usernameClaim string, admin adminRule) (identity, bool) {
	sub := strings.TrimSpace(stringClaim(claims, "sub"))
	if sub == "" {
		return identity{}, false
	}

	var username string
	if usernameClaim != "" {
		username = stringClaim(claims, usernameClaim)
	} else {
		username = firstNonEmpty(stringClaim(claims, "preferred_username"), stringClaim(claims, "email"))
	}
	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" {
		username = sub
	}

	name := strings.TrimSpace(stringClaim(claims, "name"))
	if name == "" {
		name = strings.TrimSpace(stringClaim(claims, "given_name") + " " + stringClaim(claims, "family_name"))
	}

	return identity{sub: sub, username: username, name: name, admin: admin.matches(claims)}, true
}

func stringClaim(claims map[string]any, path string) string {
	v, _ := claimByPath(claims, path).(string)
	return v
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// resolve — человек в «Людях» для identity:
//   - найден по sub — логин, имя и флаг админа подтягиваются из токена, если изменились;
//   - не найден, но есть локальная учётка с тем же логином — привязывается к sub (админ заранее
//     завёл человека и выдал проекты; так же переезжают учётки, заведённые до внешнего входа);
//   - иначе заводится: активный, без проектов — пока админ их не выдаст, не видит ничего.
func (s *Service) resolve(ctx context.Context, id identity) (*sessionModel.Session, error) {
	if entry, ok := s.cache.get(id); ok {
		return entry.session()
	}

	usr, found, err := s.usrSvc.GetByExtSub(ctx, id.sub)
	if err != nil {
		return nil, fmt.Errorf("usrSvc.GetByExtSub: %w", err)
	}

	if !found {
		usr, err = s.linkOrCreate(ctx, id)
		if err != nil {
			return nil, err
		}
	} else if err = s.sync(ctx, usr, id, false); err != nil {
		return nil, err
	}

	entry := identityEntry{id: id, usrId: usr.Id, active: usr.Active}
	s.cache.put(entry)
	return entry.session()
}

func (s *Service) linkOrCreate(ctx context.Context, id identity) (*usrModel.Main, error) {
	local, found, err := s.usrSvc.GetByUsername(ctx, id.username)
	if err != nil {
		return nil, fmt.Errorf("usrSvc.GetByUsername: %w", err)
	}
	if found && local.ExtSub == "" {
		if err = s.sync(ctx, local, id, true); err != nil {
			return nil, err
		}
		return local, nil
	}

	username := id.username
	if found {
		// логин занят человеком другого sub (провайдер выдал тот же логин другому) — не путаем их
		username = id.username + "-" + shortHash(id.sub)
	}

	obj := &usrModel.Edit{
		Active:      new(true),
		IsAdmin:     new(id.admin),
		Name:        new(id.name),
		Username:    new(username),
		ExtSub:      new(id.sub),
		AllProjects: new(false),
		ProjectIds:  new([]int64{}),
	}
	newId, err := s.usrSvc.Create(ctx, obj)
	if err != nil {
		// параллельный первый запрос того же человека успел завести его раньше
		if errors.Is(err, errs.UsrExtSubExists) || errors.Is(err, errs.UsrUsernameExists) {
			usr, ok, getErr := s.usrSvc.GetByExtSub(ctx, id.sub)
			if getErr != nil {
				return nil, fmt.Errorf("usrSvc.GetByExtSub: %w", getErr)
			}
			if ok {
				return usr, nil
			}
		}
		return nil, fmt.Errorf("usrSvc.Create: %w", err)
	}

	return &usrModel.Main{Id: newId, Active: true, IsAdmin: id.admin, Name: id.name, Username: username, ExtSub: id.sub}, nil
}

// sync — подтягивает из токена флаг админа, имя и логин; link — ещё и привязывает к sub. Логин,
// занятый другим человеком, не меняется (и логин с суффиксом от shortHash не трогается).
func (s *Service) sync(ctx context.Context, usr *usrModel.Main, id identity, link bool) error {
	obj := &usrModel.Edit{}
	changed := false
	if link {
		obj.ExtSub, changed = new(id.sub), true
	}
	if usr.IsAdmin != id.admin {
		obj.IsAdmin, changed = new(id.admin), true
	}
	if id.name != "" && usr.Name != id.name {
		obj.Name, changed = new(id.name), true
	}
	if usr.Username != id.username && !strings.HasPrefix(usr.Username, id.username+"-") {
		obj.Username = new(id.username)
	}
	if !changed && obj.Username == nil {
		return nil
	}

	err := s.usrSvc.Update(ctx, usr.Id, obj)
	if obj.Username != nil && errors.Is(err, errs.UsrUsernameExists) {
		obj.Username = nil
		err = nil
		if changed {
			err = s.usrSvc.Update(ctx, usr.Id, obj)
		}
	}
	if err != nil {
		return fmt.Errorf("usrSvc.Update: %w", err)
	}
	return nil
}

func shortHash(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:3])
}

// identityCache — «sub → человек» на identityCacheTTL. Запись годна, пока логин, имя и флаг админа
// в токене те же: сменились — человек перечитывается и обновляется.
type identityCache struct {
	ttl     time.Duration
	mu      sync.Mutex
	entries map[string]identityEntry
}

type identityEntry struct {
	id      identity
	usrId   int64
	active  bool
	expires time.Time
}

func (e identityEntry) session() (*sessionModel.Session, error) {
	if !e.active {
		return nil, errs.UsrDisabled
	}
	return &sessionModel.Session{Id: e.usrId, Admin: e.id.admin}, nil
}

func newIdentityCache(ttl time.Duration) *identityCache {
	return &identityCache{ttl: ttl, entries: make(map[string]identityEntry)}
}

func (c *identityCache) get(id identity) (identityEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[id.sub]
	if !ok || time.Now().After(entry.expires) || entry.id != id {
		return identityEntry{}, false
	}
	return entry, true
}

func (c *identityCache) put(entry identityEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	entry.expires = now.Add(c.ttl)
	c.entries[entry.id.sub] = entry
	// людей немного, но просроченные записи не копим
	for sub, e := range c.entries {
		if now.After(e.expires) {
			delete(c.entries, sub)
		}
	}
}
