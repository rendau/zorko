package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rendau/zorko/internal/constant"
	sessionModel "github.com/rendau/zorko/internal/domain/session/model"
	"github.com/rendau/zorko/internal/infra/httpx"
)

// identityCacheTTL — сколько помнить «sub → человек», не заглядывая в БД. Отключение человека
// в «Людях» действует не позже этого срока (видимость проектов перечитывается на каждый запрос).
const identityCacheTTL = 30 * time.Second

type Config struct {
	Mode          string // constant.AuthMode*
	AdminRole     string
	RolesClaim    string
	UsernameClaim string
	OidcIssuer    string
	OidcAudience  string
}

type Service struct {
	mode          string
	sessionSvc    sessionServiceI
	usrSvc        usrServiceI
	claims        claimsReaderI
	admin         adminRule
	usernameClaim string
	cache         *identityCache
}

func New(cfg Config, sessionSvc sessionServiceI, usrSvc usrServiceI) (*Service, error) {
	s := &Service{
		mode:          cfg.Mode,
		sessionSvc:    sessionSvc,
		usrSvc:        usrSvc,
		admin:         newAdminRule(cfg.AdminRole, cfg.RolesClaim),
		usernameClaim: strings.TrimSpace(cfg.UsernameClaim),
		cache:         newIdentityCache(identityCacheTTL),
	}

	switch cfg.Mode {
	case constant.AuthModeLocal:
	case constant.AuthModeProxy:
		s.claims = proxyClaims{}
	case constant.AuthModeOidc:
		if strings.TrimSpace(cfg.OidcIssuer) == "" {
			return nil, errors.New("OIDC_ISSUER is empty")
		}
		s.claims = newOidcClaims(cfg.OidcIssuer, cfg.OidcAudience, httpx.New(httpx.Config{Timeout: 10 * time.Second}))
	default:
		return nil, fmt.Errorf("unknown AUTH_MODE %q: local, oidc or proxy expected", cfg.Mode)
	}

	return s, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (*sessionModel.Session, error) {
	if s.claims == nil {
		session, err := s.sessionSvc.FromToken(token)
		if err != nil || !session.IsAuthorized() {
			return nil, nil //nolint:nilerr // негодный токен — пустая сессия, не сбой
		}
		return session, nil
	}

	claims, err := s.claims.Claims(ctx, token)
	if err != nil {
		if errors.Is(err, errProviderUnavailable) {
			return nil, fmt.Errorf("claims: %w", err)
		}
		return nil, nil //nolint:nilerr // негодный токен — пустая сессия, не сбой
	}

	id, ok := identityFromClaims(claims, s.usernameClaim, s.admin)
	if !ok {
		return nil, nil
	}

	return s.resolve(ctx, id)
}
