package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rendau/zorko/internal/domain/oauth/model"
	oauthRepoMem "github.com/rendau/zorko/internal/domain/oauth/repo/mem"
	oauthService "github.com/rendau/zorko/internal/domain/oauth/service"
	sessionModel "github.com/rendau/zorko/internal/domain/session/model"
	sessionService "github.com/rendau/zorko/internal/domain/session/service"
	usrModel "github.com/rendau/zorko/internal/domain/usr/model"
	"github.com/rendau/zorko/internal/errs"
)

const (
	testIssuer   = "https://mcp.example.com"
	testRedirect = "https://claude.ai/api/mcp/auth_callback"
	testVerifier = "verifier-verifier-verifier-verifier-verifier"
)

type usrFake map[int64]*usrModel.Main

func (f usrFake) Get(_ context.Context, id int64, _ bool) (*usrModel.Main, bool, error) {
	v, ok := f[id]
	return v, ok, nil
}

type fixture struct {
	uc       *Usecase
	repo     *oauthRepoMem.Repo
	usrs     usrFake
	sessions *sessionService.Service
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	repo := oauthRepoMem.New()
	usrs := usrFake{
		1: {Id: 1, Active: true, IsAdmin: true, Username: "admin"},
		2: {Id: 2, Active: true, Username: "oleg", ProjectIds: []int64{5}},
	}
	sessions := sessionService.New("test-secret")

	return &fixture{
		uc: New(Config{
			Enabled:      true,
			Issuer:       testIssuer,
			AdminURL:     "https://zorko.example.com",
			RedirectUris: []string{testRedirect, "http://localhost"},
			AccessTTL:    time.Hour,
			RefreshTTL:   24 * time.Hour,
		}, oauthService.New(repo), oauthService.NewClientCodec("test-secret"), usrs, sessions),
		repo:     repo,
		usrs:     usrs,
		sessions: sessions,
	}
}

func (f *fixture) as(usrId int64) context.Context {
	return f.sessions.WithContext(context.Background(), &sessionModel.Session{Id: usrId})
}

func (f *fixture) register(t *testing.T) *RegisterRep {
	t.Helper()
	rep, err := f.uc.Register(context.Background(), &RegisterReq{
		RedirectUris:            []string{testRedirect},
		ClientName:              "Claude",
		TokenEndpointAuthMethod: model.ClientAuthNone,
	})
	require.NoError(t, err)
	return rep
}

func challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func authorizeReq(clientId string) *AuthorizeReq {
	return &AuthorizeReq{
		ResponseType:        "code",
		ClientId:            clientId,
		RedirectUri:         testRedirect,
		State:               "state-1",
		CodeChallenge:       challenge(testVerifier),
		CodeChallengeMethod: "S256",
		Scope:               Scope,
		Resource:            testIssuer + "/",
	}
}

// consent — согласие человека; возвращает код из адреса возврата
func (f *fixture) consent(t *testing.T, usrId int64, clientId string) string {
	t.Helper()
	redirect, err := f.uc.Authorize(f.as(usrId), authorizeReq(clientId), true)
	require.NoError(t, err)

	parsed, err := url.Parse(redirect)
	require.NoError(t, err)
	assert.Equal(t, "claude.ai", parsed.Host)
	assert.Equal(t, "state-1", parsed.Query().Get("state"))

	code := parsed.Query().Get("code")
	require.NotEmpty(t, code)
	return code
}

func (f *fixture) exchange(t *testing.T, clientId, code string) *TokenRep {
	t.Helper()
	rep, err := f.uc.Token(context.Background(), &TokenReq{
		GrantType:    "authorization_code",
		Code:         code,
		RedirectUri:  testRedirect,
		CodeVerifier: testVerifier,
		ClientId:     clientId,
	})
	require.NoError(t, err)
	return rep
}

func oauthCode(t *testing.T, err error) string {
	t.Helper()
	require.Error(t, err)
	var oauthErr *Error
	require.ErrorAs(t, err, &oauthErr)
	return oauthErr.Code
}

func TestFlow(t *testing.T) {
	f := newFixture(t)
	client := f.register(t)
	assert.Empty(t, client.ClientSecret, "клиент без секрета")

	info, err := f.uc.DescribeAuthorization(f.as(2), authorizeReq(client.ClientId))
	require.NoError(t, err)
	assert.Equal(t, "Claude", info.ClientName)
	assert.Equal(t, "claude.ai", info.RedirectHost)

	code := f.consent(t, 2, client.ClientId)

	grants, err := f.uc.ListGrants(f.as(2))
	require.NoError(t, err)
	assert.Empty(t, grants, "до обмена кода подключение не состоялось")

	tokens := f.exchange(t, client.ClientId, code)
	assert.Equal(t, int64(3600), tokens.ExpiresIn)
	assert.Equal(t, Scope, tokens.Scope)

	session, err := f.uc.McpSessionFromToken(context.Background(), tokens.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, int64(2), session.Session.Id)
	assert.False(t, session.Session.Admin)
	assert.Equal(t, "Claude", session.ClientName)

	grants, err = f.uc.ListGrants(f.as(2))
	require.NoError(t, err)
	require.Len(t, grants, 1)
	assert.NotNil(t, grants[0].LastUsedAt)

	others, err := f.uc.ListGrants(f.as(1))
	require.NoError(t, err)
	assert.Empty(t, others, "чужие подключения не видны")

	t.Run("код одноразовый", func(t *testing.T) {
		_, err := f.uc.Token(context.Background(), &TokenReq{
			GrantType: "authorization_code", Code: code, RedirectUri: testRedirect,
			CodeVerifier: testVerifier, ClientId: client.ClientId,
		})
		assert.Equal(t, ErrInvalidGrant, oauthCode(t, err))
	})

	t.Run("refresh меняет пару, старый больше не годится", func(t *testing.T) {
		next, err := f.uc.Token(context.Background(), &TokenReq{
			GrantType: "refresh_token", RefreshToken: tokens.RefreshToken, ClientId: client.ClientId,
		})
		require.NoError(t, err)
		assert.NotEqual(t, tokens.AccessToken, next.AccessToken)

		_, err = f.uc.Token(context.Background(), &TokenReq{
			GrantType: "refresh_token", RefreshToken: tokens.RefreshToken, ClientId: client.ClientId,
		})
		assert.Equal(t, ErrInvalidGrant, oauthCode(t, err))

		_, err = f.uc.McpSessionFromToken(context.Background(), next.AccessToken)
		require.NoError(t, err)

		// refresh-токен как access не принимается
		_, err = f.uc.McpSessionFromToken(context.Background(), next.RefreshToken)
		assert.ErrorIs(t, err, errs.NotAuthorized)
	})

	t.Run("чужое подключение не отключить", func(t *testing.T) {
		assert.ErrorIs(t, f.uc.DeleteGrant(f.as(1), grants[0].Id), errs.ObjectNotFound)
	})

	t.Run("отключение действует сразу", func(t *testing.T) {
		require.NoError(t, f.uc.DeleteGrant(f.as(2), grants[0].Id))
		_, err := f.uc.McpSessionFromToken(context.Background(), tokens.AccessToken)
		assert.ErrorIs(t, err, errs.NotAuthorized)
	})
}

func TestAdminSession(t *testing.T) {
	f := newFixture(t)
	client := f.register(t)
	tokens := f.exchange(t, client.ClientId, f.consent(t, 1, client.ClientId))

	session, err := f.uc.McpSessionFromToken(context.Background(), tokens.AccessToken)
	require.NoError(t, err)
	assert.True(t, session.Session.Admin)

	// роль читается из живой записи: сняли админа — следующий же запрос без неё
	f.usrs[1].IsAdmin = false
	session, err = f.uc.McpSessionFromToken(context.Background(), tokens.AccessToken)
	require.NoError(t, err)
	assert.False(t, session.Session.Admin)
}

func TestDisabledUser(t *testing.T) {
	f := newFixture(t)
	client := f.register(t)
	code := f.consent(t, 2, client.ClientId)
	tokens := f.exchange(t, client.ClientId, code)

	f.usrs[2].Active = false

	_, err := f.uc.McpSessionFromToken(context.Background(), tokens.AccessToken)
	assert.ErrorIs(t, err, errs.NotAuthorized, "отключённый человек — токен не работает сразу")

	_, err = f.uc.Token(context.Background(), &TokenReq{
		GrantType: "refresh_token", RefreshToken: tokens.RefreshToken, ClientId: client.ClientId,
	})
	assert.Equal(t, ErrInvalidGrant, oauthCode(t, err))

	_, err = f.uc.Authorize(f.as(2), authorizeReq(client.ClientId), true)
	assert.ErrorIs(t, err, errs.NotAuthorized, "отключённый не подключает новых клиентов")
}

func TestExpiredAccessToken(t *testing.T) {
	f := newFixture(t)
	client := f.register(t)
	tokens := f.exchange(t, client.ClientId, f.consent(t, 2, client.ClientId))

	f.repo.Expire(tokens.AccessToken)

	_, err := f.uc.McpSessionFromToken(context.Background(), tokens.AccessToken)
	assert.ErrorIs(t, err, errs.NotAuthorized)
}

func TestConsentRequiresLogin(t *testing.T) {
	f := newFixture(t)
	client := f.register(t)

	_, err := f.uc.DescribeAuthorization(context.Background(), authorizeReq(client.ClientId))
	assert.ErrorIs(t, err, errs.NotAuthorized)
	_, err = f.uc.Authorize(context.Background(), authorizeReq(client.ClientId), true)
	assert.ErrorIs(t, err, errs.NotAuthorized)
	_, err = f.uc.ListGrants(context.Background())
	assert.ErrorIs(t, err, errs.NotAuthorized)
}

func TestDeny(t *testing.T) {
	f := newFixture(t)
	client := f.register(t)

	redirect, err := f.uc.Authorize(f.as(2), authorizeReq(client.ClientId), false)
	require.NoError(t, err)

	parsed, err := url.Parse(redirect)
	require.NoError(t, err)
	assert.Equal(t, "access_denied", parsed.Query().Get("error"))
	assert.Equal(t, "state-1", parsed.Query().Get("state"))
	assert.Empty(t, parsed.Query().Get("code"))
	assert.Zero(t, f.repo.GrantCount())
}

func TestAuthorizeReqChecks(t *testing.T) {
	f := newFixture(t)
	client := f.register(t)

	cases := map[string]func(*AuthorizeReq){
		"чужой client_id":          func(r *AuthorizeReq) { r.ClientId = "zkcl_forged.sig" },
		"незнакомый адрес":         func(r *AuthorizeReq) { r.RedirectUri = "https://evil.example/cb" },
		"не code":                  func(r *AuthorizeReq) { r.ResponseType = "token" },
		"без PKCE":                 func(r *AuthorizeReq) { r.CodeChallenge = "" },
		"PKCE не S256":             func(r *AuthorizeReq) { r.CodeChallengeMethod = "plain" },
		"resource другого сервера": func(r *AuthorizeReq) { r.Resource = "https://other.example.com" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			req := authorizeReq(client.ClientId)
			mutate(req)
			_, err := f.uc.Authorize(f.as(2), req, true)
			require.Error(t, err)
			assert.Equal(t, errs.OAuthInvalidRequest.Error(), errs.Code(err))
		})
	}
}

func TestTokenChecks(t *testing.T) {
	f := newFixture(t)
	client := f.register(t)
	other := f.register(t)

	base := func(code string) *TokenReq {
		return &TokenReq{
			GrantType: "authorization_code", Code: code, RedirectUri: testRedirect,
			CodeVerifier: testVerifier, ClientId: client.ClientId,
		}
	}

	t.Run("неверный verifier, код остаётся годным", func(t *testing.T) {
		code := f.consent(t, 2, client.ClientId)
		req := base(code)
		req.CodeVerifier = "wrong-wrong-wrong-wrong-wrong-wrong-wrong-wrong"
		_, err := f.uc.Token(context.Background(), req)
		assert.Equal(t, ErrInvalidGrant, oauthCode(t, err))

		_, err = f.uc.Token(context.Background(), base(code))
		require.NoError(t, err)
	})

	t.Run("код другого клиента", func(t *testing.T) {
		req := base(f.consent(t, 2, client.ClientId))
		req.ClientId = other.ClientId
		_, err := f.uc.Token(context.Background(), req)
		assert.Equal(t, ErrInvalidGrant, oauthCode(t, err))
	})

	t.Run("другой адрес возврата", func(t *testing.T) {
		req := base(f.consent(t, 2, client.ClientId))
		req.RedirectUri = "http://localhost:1234/cb"
		_, err := f.uc.Token(context.Background(), req)
		assert.Equal(t, ErrInvalidGrant, oauthCode(t, err))
	})

	t.Run("истёкший код", func(t *testing.T) {
		code := f.consent(t, 2, client.ClientId)
		f.repo.Expire(code)
		_, err := f.uc.Token(context.Background(), base(code))
		assert.Equal(t, ErrInvalidGrant, oauthCode(t, err))
	})

	t.Run("неизвестный клиент", func(t *testing.T) {
		req := base("zkc_x")
		req.ClientId = "nope"
		_, err := f.uc.Token(context.Background(), req)
		assert.Equal(t, ErrInvalidClient, oauthCode(t, err))
	})

	t.Run("неизвестный grant_type", func(t *testing.T) {
		req := base("zkc_x")
		req.GrantType = "password"
		_, err := f.uc.Token(context.Background(), req)
		assert.Equal(t, ErrUnsupportedGrantType, oauthCode(t, err))
	})
}

func TestRegisterChecks(t *testing.T) {
	f := newFixture(t)

	_, err := f.uc.Register(context.Background(), &RegisterReq{RedirectUris: []string{"https://evil.example/cb"}})
	assert.Equal(t, ErrInvalidRedirectUri, oauthCode(t, err))

	_, err = f.uc.Register(context.Background(), &RegisterReq{})
	assert.Equal(t, ErrInvalidRedirectUri, oauthCode(t, err))

	// Claude Code слушает случайный порт на localhost
	rep, err := f.uc.Register(context.Background(), &RegisterReq{RedirectUris: []string{"http://localhost:53124/callback"}})
	require.NoError(t, err)
	assert.NotEmpty(t, rep.ClientSecret, "без указания способа — клиент с секретом")
	assert.Equal(t, defaultClientName, rep.Client.Name)
}

func TestConfidentialClient(t *testing.T) {
	f := newFixture(t)
	client, err := f.uc.Register(context.Background(), &RegisterReq{
		RedirectUris:            []string{testRedirect},
		TokenEndpointAuthMethod: model.ClientAuthSecretPost,
	})
	require.NoError(t, err)

	code := f.consent(t, 2, client.ClientId)
	req := &TokenReq{
		GrantType: "authorization_code", Code: code, RedirectUri: testRedirect,
		CodeVerifier: testVerifier, ClientId: client.ClientId,
	}

	_, err = f.uc.Token(context.Background(), req)
	assert.Equal(t, ErrInvalidClient, oauthCode(t, err), "без секрета")

	req.ClientSecret = client.ClientSecret
	_, err = f.uc.Token(context.Background(), req)
	require.NoError(t, err)
}

func TestRevoke(t *testing.T) {
	f := newFixture(t)
	client := f.register(t)
	tokens := f.exchange(t, client.ClientId, f.consent(t, 2, client.ClientId))

	require.NoError(t, f.uc.Revoke(context.Background(), "zko_unknown", client.ClientId, ""), "неизвестный токен — не ошибка")

	require.NoError(t, f.uc.Revoke(context.Background(), tokens.RefreshToken, client.ClientId, ""))
	_, err := f.uc.McpSessionFromToken(context.Background(), tokens.AccessToken)
	assert.ErrorIs(t, err, errs.NotAuthorized, "отзыв любого токена удаляет подключение")
}

func TestDisabled(t *testing.T) {
	f := newFixture(t)
	client := f.register(t)
	f.uc.conf.Enabled = false

	_, err := f.uc.Register(context.Background(), &RegisterReq{RedirectUris: []string{testRedirect}})
	assert.Equal(t, ErrTemporarilyUnavail, oauthCode(t, err))

	_, err = f.uc.Authorize(f.as(2), authorizeReq(client.ClientId), true)
	assert.ErrorIs(t, err, errs.OAuthNotEnabled)

	_, err = f.uc.McpSessionFromToken(context.Background(), "zko_x")
	assert.ErrorIs(t, err, errs.NotAuthorized)
}
