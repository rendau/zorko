// Package oauth — сервер авторизации OAuth 2.1 встроенного MCP-сервера (ADR-020): MCP-клиент
// (приложения Claude, Claude Code) подключается по одному адресу — человек входит в интерфейс и
// разрешает клиента, а клиент получает токены с правами этого человека.
package oauth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/rendau/zorko/internal/domain/oauth/model"
	sessionModel "github.com/rendau/zorko/internal/domain/session/model"
	"github.com/rendau/zorko/internal/errs"
)

const (
	// Scope — единственный scope: доступ к MCP-серверу
	Scope = "mcp"

	codeTTL = 2 * time.Minute

	// touchInterval ограничивает запись last_used_at: клиент ходит на каждый вызов инструмента
	touchInterval = time.Minute
)

type Config struct {
	Enabled bool
	// Issuer — адрес MCP-сервера, как его видят клиенты: одновременно OAuth issuer и
	// защищаемый ресурс
	Issuer string
	// AdminURL — адрес интерфейса, там страница согласия
	AdminURL     string
	RedirectUris []string
	AccessTTL    time.Duration
	RefreshTTL   time.Duration
}

type Usecase struct {
	conf        Config
	svc         ServiceI
	clientCodec ClientCodecI
	usrSvc      UsrServiceI
	sessionSvc  SessionServiceI

	touchMu   sync.Mutex
	lastTouch map[string]time.Time
}

func New(conf Config, svc ServiceI, clientCodec ClientCodecI, usrSvc UsrServiceI, sessionSvc SessionServiceI) *Usecase {
	return &Usecase{
		conf:        conf,
		svc:         svc,
		clientCodec: clientCodec,
		usrSvc:      usrSvc,
		sessionSvc:  sessionSvc,
		lastTouch:   map[string]time.Time{},
	}
}

func (u *Usecase) Config() Config {
	return u.conf
}

// McpSession — кто пришёл на MCP-порт: человек подключения и само подключение (для логов)
type McpSession struct {
	Session    *sessionModel.Session
	GrantId    string
	ClientName string
	Username   string
}

// ListGrants — подключённые клиенты вошедшего человека
func (u *Usecase) ListGrants(ctx context.Context) ([]*model.Grant, error) {
	session := u.sessionSvc.FromContext(ctx)
	if !session.IsAuthorized() {
		return nil, errs.NotAuthorized
	}

	items, err := u.svc.ListGrants(ctx, session.Id)
	if err != nil {
		return nil, fmt.Errorf("svc.ListGrants: %w", err)
	}
	return items, nil
}

// DeleteGrant отключает клиента: его токены перестают работать сразу
func (u *Usecase) DeleteGrant(ctx context.Context, id string) error {
	session := u.sessionSvc.FromContext(ctx)
	if !session.IsAuthorized() {
		return errs.NotAuthorized
	}
	if id == "" {
		return errs.IdRequired
	}

	item, _, err := u.svc.GetGrant(ctx, id, true)
	if err != nil {
		return fmt.Errorf("svc.GetGrant: %w", err)
	}
	// чужое подключение — как несуществующее: id не подтверждаем
	if item.UsrId != session.Id {
		return errs.ObjectNotFound
	}

	if err = u.svc.DeleteGrant(ctx, item.Id); err != nil {
		return fmt.Errorf("svc.DeleteGrant: %w", err)
	}
	slog.Info("oauth: connection removed by the user", "grant_id", item.Id, "client_name", item.ClientName, "usr_id", item.UsrId)
	return nil
}

// McpSessionFromToken — кто пришёл с access-токеном. Сессия строится по живой записи человека:
// отключение в «Людях» и смена роли действуют сразу. Негодный токен — errs.NotAuthorized.
func (u *Usecase) McpSessionFromToken(ctx context.Context, token string) (*McpSession, error) {
	if !u.conf.Enabled {
		return nil, errs.NotAuthorized
	}

	item, found, err := u.svc.FindToken(ctx, token, model.TokenKindAccess)
	if err != nil {
		return nil, fmt.Errorf("svc.FindToken: %w", err)
	}
	if !found {
		u.logRejectedToken(ctx, token)
		return nil, errs.NotAuthorized
	}

	grant, found, err := u.svc.GetGrant(ctx, item.GrantId, false)
	if err != nil {
		return nil, fmt.Errorf("svc.GetGrant: %w", err)
	}
	if !found {
		slog.Warn("oauth: access token rejected", "reason", "connection removed", "grant_id", item.GrantId)
		return nil, errs.NotAuthorized
	}

	result, err := u.grantSession(ctx, grant)
	if err != nil {
		if errors.Is(err, errs.NotAuthorized) {
			slog.Warn("oauth: access token rejected", "reason", "user is not active", "grant_id", grant.Id, "usr_id", grant.UsrId)
		}
		return nil, err
	}

	u.touchLastUsed(ctx, grant.Id)

	return result, nil
}

// logRejectedToken объясняет, почему токен не живой: по журналу запросов не отличить устаревший
// токен от чужого
func (u *Usecase) logRejectedToken(ctx context.Context, token string) {
	item, found, err := u.svc.FindAnyToken(ctx, token)
	switch {
	case err != nil:
		slog.Warn("oauth: access token rejected", "reason", "lookup failed", "error", err)
	case !found:
		slog.Warn("oauth: access token rejected", "reason", "unknown token")
	case item.Kind != model.TokenKindAccess:
		slog.Warn("oauth: access token rejected", "reason", "not an access token", "kind", item.Kind, "grant_id", item.GrantId)
	default:
		slog.Warn("oauth: access token rejected", "reason", "expired", "expires_at", item.ExpiresAt, "grant_id", item.GrantId)
	}
}

// grantSession — сессия владельца подключения; отключённый или удалённый человек — NotAuthorized
func (u *Usecase) grantSession(ctx context.Context, grant *model.Grant) (*McpSession, error) {
	usr, found, err := u.usrSvc.Get(ctx, grant.UsrId, false)
	if err != nil {
		return nil, fmt.Errorf("usrSvc.Get: %w", err)
	}
	if !found || !usr.Active {
		return nil, errs.NotAuthorized
	}

	return &McpSession{
		Session:    &sessionModel.Session{Id: usr.Id, Admin: usr.IsAdmin},
		GrantId:    grant.Id,
		ClientName: grant.ClientName,
		Username:   usr.Username,
	}, nil
}

func (u *Usecase) touchLastUsed(ctx context.Context, id string) {
	u.touchMu.Lock()
	if last, ok := u.lastTouch[id]; ok && time.Since(last) < touchInterval {
		u.touchMu.Unlock()
		return
	}
	u.lastTouch[id] = time.Now()
	u.touchMu.Unlock()

	if err := u.svc.TouchGrantLastUsed(ctx, id); err != nil {
		slog.Warn("oauth: touch last_used_at", "error", err, "id", id)
	}
}
