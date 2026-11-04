package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"

	sessionModel "github.com/rendau/zorko/internal/domain/session/model"
	"github.com/rendau/zorko/internal/errs"
)

// Вход как в kusec (ADR-005): короткий access без похода в БД и долгий refresh,
// при обмене которого человек перечитывается из БД.
const (
	accessTokenTTL  = 15 * time.Minute
	refreshTokenTTL = 30 * 24 * time.Hour

	tokenTypeAccess  = "access"
	tokenTypeRefresh = "refresh"
)

type contextKey string

const sessionContextKey = contextKey("session")

type Service struct {
	secret string
}

func New(secret string) *Service {
	return &Service{secret: strings.TrimSpace(secret)}
}

// WithContext кладёт сессию в контекст (gRPC-перехватчик)
func (s *Service) WithContext(ctx context.Context, session *sessionModel.Session) context.Context {
	if session == nil {
		session = sessionModel.New(0)
	}
	return context.WithValue(ctx, sessionContextKey, session)
}

// FromContext — сессия из контекста; всегда не nil
func (s *Service) FromContext(ctx context.Context) *sessionModel.Session {
	if ctx != nil {
		if session, ok := ctx.Value(sessionContextKey).(*sessionModel.Session); ok && session != nil {
			return session
		}
	}
	return sessionModel.New(0)
}

func (s *Service) CtxIsAuthorized(ctx context.Context) bool {
	return s.FromContext(ctx).IsAuthorized()
}

func (s *Service) CtxIsAdmin(ctx context.Context) bool {
	return s.FromContext(ctx).IsAdmin()
}

// CreateToken — access-токен на accessTokenTTL
func (s *Service) CreateToken(usrId int64, isAdmin bool) (string, error) {
	now := time.Now().UTC()
	return s.signClaims(jwtv5.MapClaims{
		"typ":      tokenTypeAccess,
		"id":       usrId,
		"is_admin": isAdmin,
		"iat":      now.Unix(),
		"exp":      now.Add(accessTokenTTL).Unix(),
	})
}

// CreateRefreshToken — refresh-токен на refreshTokenTTL с отпечатком хеша пароля:
// смена пароля отзывает все выданные ранее refresh-токены
func (s *Service) CreateRefreshToken(usrId int64, passwordHash string) (string, error) {
	now := time.Now().UTC()
	return s.signClaims(jwtv5.MapClaims{
		"typ": tokenTypeRefresh,
		"id":  usrId,
		"pwd": passwordFingerprint(passwordHash),
		"iat": now.Unix(),
		"exp": now.Add(refreshTokenTTL).Unix(),
	})
}

// FromToken — сессия из access-токена (подпись и срок, без БД); refresh-токен не годится
func (s *Service) FromToken(tokenStr string) (*sessionModel.Session, error) {
	claims, err := s.parseClaims(tokenStr)
	if err != nil {
		return nil, err
	}
	if tokenType(claims) != tokenTypeAccess {
		return nil, fmt.Errorf("not an access token")
	}

	usrId, err := usrIdFromClaims(claims)
	if err != nil {
		return nil, err
	}
	isAdmin, ok := claims["is_admin"].(bool)
	if !ok {
		return nil, fmt.Errorf("missing is_admin claim in token")
	}

	return &sessionModel.Session{Id: usrId, Admin: isAdmin}, nil
}

// RefreshTokenUserId — id человека из refresh-токена без сверки отпечатка: нужен, чтобы
// сначала загрузить человека и его текущий хеш пароля
func (s *Service) RefreshTokenUserId(tokenStr string) (int64, error) {
	claims, err := s.parseClaims(tokenStr)
	if err != nil {
		return 0, err
	}
	if tokenType(claims) != tokenTypeRefresh {
		return 0, fmt.Errorf("not a refresh token")
	}
	return usrIdFromClaims(claims)
}

// CheckRefreshToken сверяет отпечаток пароля в refresh-токене с текущим хешем
func (s *Service) CheckRefreshToken(tokenStr, currentPasswordHash string) error {
	claims, err := s.parseClaims(tokenStr)
	if err != nil {
		return err
	}
	if tokenType(claims) != tokenTypeRefresh {
		return fmt.Errorf("not a refresh token")
	}
	fp, _ := claims["pwd"].(string)
	if fp == "" || fp != passwordFingerprint(currentPasswordHash) {
		return fmt.Errorf("refresh token revoked by password change")
	}
	return nil
}

func (s *Service) parseClaims(tokenStr string) (jwtv5.MapClaims, error) {
	if s.secret == "" {
		return nil, errs.InvalidConfig
	}

	claims := jwtv5.MapClaims{}
	parsed, err := jwtv5.ParseWithClaims(tokenStr, claims,
		func(_ *jwtv5.Token) (any, error) { return []byte(s.secret), nil },
		jwtv5.WithValidMethods([]string{jwtv5.SigningMethodHS256.Alg()}),
		jwtv5.WithExpirationRequired(),
	)
	if err != nil {
		return nil, err
	}
	if !parsed.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	return claims, nil
}

func (s *Service) signClaims(claims jwtv5.MapClaims) (string, error) {
	if s.secret == "" {
		return "", errs.InvalidConfig
	}
	tokenStr, err := jwtv5.NewWithClaims(jwtv5.SigningMethodHS256, claims).SignedString([]byte(s.secret))
	if err != nil {
		return "", fmt.Errorf("token.SignedString: %w", err)
	}
	return tokenStr, nil
}

func tokenType(claims jwtv5.MapClaims) string {
	typ, _ := claims["typ"].(string)
	return typ
}

func usrIdFromClaims(claims jwtv5.MapClaims) (int64, error) {
	id, ok := claims["id"].(float64) // числа JSON декодируются как float64
	if !ok || id <= 0 || id != float64(int64(id)) {
		return 0, fmt.Errorf("bad user id claim in token")
	}
	return int64(id), nil
}

// passwordFingerprint — необратимый отпечаток хеша пароля
func passwordFingerprint(passwordHash string) string {
	sum := sha256.Sum256([]byte(passwordHash))
	return hex.EncodeToString(sum[:8])
}
