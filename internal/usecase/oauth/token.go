package oauth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/rendau/zorko/internal/domain/oauth/model"
	oauthService "github.com/rendau/zorko/internal/domain/oauth/service"
	"github.com/rendau/zorko/internal/errs"
)

// коды ошибок OAuth (RFC 6749, 5.2; RFC 7591, 3.2.2)
const (
	ErrInvalidRequest        = "invalid_request"
	ErrInvalidClient         = "invalid_client"
	ErrInvalidGrant          = "invalid_grant"
	ErrUnsupportedGrantType  = "unsupported_grant_type"
	ErrInvalidTarget         = "invalid_target"
	ErrInvalidRedirectUri    = "invalid_redirect_uri"
	ErrInvalidClientMetadata = "invalid_client_metadata"
	ErrTemporarilyUnavail    = "temporarily_unavailable"
)

// Error — ошибка протокола OAuth для адресов на MCP-порту: отдаётся клиенту как
// {"error", "error_description"}. Отказ по делу, не сбой.
type Error struct {
	Code        string
	Description string
}

func (e *Error) Error() string {
	return e.Code + ": " + e.Description
}

// HTTPStatus — статус ответа с ошибкой
func (e *Error) HTTPStatus() int {
	switch e.Code {
	case ErrInvalidClient:
		return http.StatusUnauthorized
	case ErrTemporarilyUnavail:
		return http.StatusServiceUnavailable
	}
	return http.StatusBadRequest
}

func newError(code, format string, args ...any) *Error {
	return &Error{Code: code, Description: fmt.Sprintf(format, args...)}
}

var errNotEnabled = &Error{Code: ErrTemporarilyUnavail, Description: "OAuth is not enabled on this server"}

type TokenReq struct {
	GrantType    string
	Code         string
	RedirectUri  string
	CodeVerifier string
	RefreshToken string
	Resource     string
	ClientId     string
	ClientSecret string
}

type TokenRep struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
	Scope        string
}

// Token меняет код (с его PKCE verifier) или refresh-токен на новую пару access + refresh. Коды и
// refresh-токены одноразовые: гасятся в одной транзакции с выдачей новых.
func (u *Usecase) Token(ctx context.Context, req *TokenReq) (*TokenRep, error) {
	if !u.conf.Enabled {
		return nil, errNotEnabled
	}

	if err := u.authenticateClient(req.ClientId, req.ClientSecret); err != nil {
		return nil, err
	}
	if !u.resourceMatches(req.Resource) {
		return nil, newError(ErrInvalidTarget, "resource is not this server (%s)", u.conf.Issuer)
	}

	var (
		grant    *model.Grant
		oldToken string
		oldKind  string
		err      error
	)
	switch req.GrantType {
	case grantTypeAuthorizationCode:
		oldToken, oldKind = req.Code, model.TokenKindCode
		grant, err = u.checkCode(ctx, req)
	case grantTypeRefreshToken:
		oldToken, oldKind = req.RefreshToken, model.TokenKindRefresh
		grant, err = u.checkRefresh(ctx, req)
	default:
		return nil, newError(ErrUnsupportedGrantType, "grant_type must be authorization_code or refresh_token")
	}
	if err != nil {
		return nil, err
	}

	if _, err = u.grantSession(ctx, grant); err != nil {
		if errors.Is(err, errs.NotAuthorized) {
			return nil, newError(ErrInvalidGrant, "the user of this connection is not active")
		}
		return nil, err
	}

	tokens, ok, err := u.svc.Exchange(ctx, oldToken, oldKind, map[string]time.Duration{
		model.TokenKindAccess:  u.conf.AccessTTL,
		model.TokenKindRefresh: u.conf.RefreshTTL,
	})
	if err != nil {
		return nil, fmt.Errorf("svc.Exchange: %w", err)
	}
	if !ok {
		// такой же запрос успел раньше
		return nil, newError(ErrInvalidGrant, "the %s is already used", oldKind)
	}

	slog.Info("oauth: tokens issued",
		"grant_type", req.GrantType,
		"grant_id", grant.Id,
		"client_name", grant.ClientName,
		"usr_id", grant.UsrId,
		"access_ttl", u.conf.AccessTTL.String(),
	)

	return &TokenRep{
		AccessToken:  tokens[model.TokenKindAccess],
		RefreshToken: tokens[model.TokenKindRefresh],
		ExpiresIn:    int64(u.conf.AccessTTL.Seconds()),
		Scope:        Scope,
	}, nil
}

func (u *Usecase) checkCode(ctx context.Context, req *TokenReq) (*model.Grant, error) {
	if req.Code == "" || req.CodeVerifier == "" {
		return nil, newError(ErrInvalidRequest, "code and code_verifier are required")
	}

	item, found, err := u.svc.FindToken(ctx, req.Code, model.TokenKindCode)
	if err != nil {
		return nil, fmt.Errorf("svc.FindToken: %w", err)
	}
	if !found {
		return nil, newError(ErrInvalidGrant, "the code is invalid, expired or already used")
	}

	grant, err := u.clientGrant(ctx, item.GrantId, req.ClientId)
	if err != nil {
		return nil, err
	}
	if req.RedirectUri != grant.RedirectUri {
		return nil, newError(ErrInvalidGrant, "redirect_uri differs from the authorization request")
	}
	if !oauthService.VerifyPKCE(grant.CodeChallenge, req.CodeVerifier) {
		return nil, newError(ErrInvalidGrant, "code_verifier does not match the code_challenge")
	}

	return grant, nil
}

func (u *Usecase) checkRefresh(ctx context.Context, req *TokenReq) (*model.Grant, error) {
	if req.RefreshToken == "" {
		return nil, newError(ErrInvalidRequest, "refresh_token is required")
	}

	item, found, err := u.svc.FindToken(ctx, req.RefreshToken, model.TokenKindRefresh)
	if err != nil {
		return nil, fmt.Errorf("svc.FindToken: %w", err)
	}
	if !found {
		return nil, newError(ErrInvalidGrant, "the refresh token is invalid, expired or already used")
	}

	return u.clientGrant(ctx, item.GrantId, req.ClientId)
}

// clientGrant загружает подключение токена и проверяет, что токен выдан тому клиенту, который
// его предъявил
func (u *Usecase) clientGrant(ctx context.Context, grantId, clientId string) (*model.Grant, error) {
	grant, found, err := u.svc.GetGrant(ctx, grantId, false)
	if err != nil {
		return nil, fmt.Errorf("svc.GetGrant: %w", err)
	}
	if !found {
		return nil, newError(ErrInvalidGrant, "the connection was removed")
	}
	if grant.ClientId != clientId {
		return nil, newError(ErrInvalidGrant, "the token was issued to another client")
	}
	return grant, nil
}

// Revoke — отзыв токена (RFC 7009): отзыв любого токена подключения удаляет всё подключение.
// Неизвестный токен — не ошибка.
func (u *Usecase) Revoke(ctx context.Context, token, clientId, clientSecret string) error {
	if !u.conf.Enabled {
		return errNotEnabled
	}

	if err := u.authenticateClient(clientId, clientSecret); err != nil {
		return err
	}
	if token == "" {
		return newError(ErrInvalidRequest, "token is required")
	}

	item, found, err := u.svc.FindAnyToken(ctx, token)
	if err != nil {
		return fmt.Errorf("svc.FindAnyToken: %w", err)
	}
	if !found {
		return nil
	}

	grant, found, err := u.svc.GetGrant(ctx, item.GrantId, false)
	if err != nil {
		return fmt.Errorf("svc.GetGrant: %w", err)
	}
	if !found || grant.ClientId != clientId {
		return nil
	}

	if err = u.svc.DeleteGrant(ctx, grant.Id); err != nil {
		return fmt.Errorf("svc.DeleteGrant: %w", err)
	}

	slog.Info("oauth: connection revoked by the client", "grant_id", grant.Id, "client_name", grant.ClientName, "kind", item.Kind)

	return nil
}

// authenticateClient проверяет client_id и у клиента с секретом — секрет. Секрет, предъявленный
// клиентом без секрета, тоже должен быть верным.
func (u *Usecase) authenticateClient(clientId, clientSecret string) error {
	client, ok := u.clientCodec.Decode(clientId)
	if !ok {
		return newError(ErrInvalidClient, "unknown client_id")
	}
	if clientSecret == "" && client.AuthMethod == model.ClientAuthNone {
		return nil
	}
	if !u.clientCodec.VerifySecret(clientId, clientSecret) {
		return newError(ErrInvalidClient, "client authentication failed")
	}
	return nil
}
