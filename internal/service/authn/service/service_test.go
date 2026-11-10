package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rendau/zorko/internal/constant"
	sessionService "github.com/rendau/zorko/internal/domain/session/service"
	usrModel "github.com/rendau/zorko/internal/domain/usr/model"
	"github.com/rendau/zorko/internal/errs"
)

// usrFake — «Люди» в памяти с теми же уникальными логином и ext_sub, что в БД
type usrFake struct {
	mu      sync.Mutex
	items   map[int64]*usrModel.Main
	nextId  int64
	creates int
}

func newUsrFake(items ...*usrModel.Main) *usrFake {
	f := &usrFake{items: map[int64]*usrModel.Main{}}
	for _, v := range items {
		f.items[v.Id] = v
		f.nextId = max(f.nextId, v.Id)
	}
	return f
}

func (f *usrFake) find(match func(v *usrModel.Main) bool) (*usrModel.Main, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, v := range f.items {
		if match(v) {
			c := *v
			return &c, true, nil
		}
	}
	return nil, false, nil
}

func (f *usrFake) GetByExtSub(_ context.Context, extSub string) (*usrModel.Main, bool, error) {
	return f.find(func(v *usrModel.Main) bool { return v.ExtSub == extSub })
}

func (f *usrFake) GetByUsername(_ context.Context, username string) (*usrModel.Main, bool, error) {
	return f.find(func(v *usrModel.Main) bool { return v.Username == username })
}

func (f *usrFake) conflict(id int64, obj *usrModel.Edit) error {
	for _, v := range f.items {
		if v.Id == id {
			continue
		}
		if obj.Username != nil && v.Username == *obj.Username {
			return errs.UsrUsernameExists
		}
		if obj.ExtSub != nil && *obj.ExtSub != "" && v.ExtSub == *obj.ExtSub {
			return errs.UsrExtSubExists
		}
	}
	return nil
}

func (f *usrFake) Create(_ context.Context, obj *usrModel.Edit) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.conflict(0, obj); err != nil {
		return 0, err
	}
	f.nextId++
	f.creates++
	v := &usrModel.Main{Id: f.nextId}
	apply(v, obj)
	f.items[v.Id] = v
	return v.Id, nil
}

func (f *usrFake) Update(_ context.Context, id int64, obj *usrModel.Edit) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.conflict(id, obj); err != nil {
		return err
	}
	if v, ok := f.items[id]; ok {
		apply(v, obj)
	}
	return nil
}

func (f *usrFake) get(id int64) usrModel.Main {
	f.mu.Lock()
	defer f.mu.Unlock()
	return *f.items[id]
}

func apply(v *usrModel.Main, obj *usrModel.Edit) {
	if obj.Active != nil {
		v.Active = *obj.Active
	}
	if obj.IsAdmin != nil {
		v.IsAdmin = *obj.IsAdmin
	}
	if obj.Name != nil {
		v.Name = *obj.Name
	}
	if obj.Username != nil {
		v.Username = *obj.Username
	}
	if obj.ExtSub != nil {
		v.ExtSub = *obj.ExtSub
	}
	if obj.AllProjects != nil {
		v.AllProjects = *obj.AllProjects
	}
	if obj.ProjectIds != nil {
		v.ProjectIds = *obj.ProjectIds
	}
}

// unsigned — токен без годной подписи: режиму proxy подпись не важна
func unsigned(t *testing.T, claims jwtv5.MapClaims) string {
	t.Helper()
	token, err := jwtv5.NewWithClaims(jwtv5.SigningMethodHS256, claims).SignedString([]byte("not-checked"))
	require.NoError(t, err)
	return token
}

func newProxy(t *testing.T, usr *usrFake) *Service {
	t.Helper()
	s, err := New(Config{Mode: constant.AuthModeProxy, AdminRole: "zorko:admin"}, sessionService.New(""), usr)
	require.NoError(t, err)
	return s
}

func adminClaims(sub, username string) jwtv5.MapClaims {
	return jwtv5.MapClaims{
		"sub": sub, "preferred_username": username, "name": "Админ",
		"resource_access": map[string]any{"zorko": map[string]any{"roles": []any{"admin"}}},
	}
}

func TestNewConfig(t *testing.T) {
	_, err := New(Config{Mode: "ldap"}, sessionService.New(""), newUsrFake())
	assert.Error(t, err, "неизвестный режим")
	_, err = New(Config{Mode: constant.AuthModeOidc}, sessionService.New(""), newUsrFake())
	assert.Error(t, err, "oidc без OIDC_ISSUER")
}

func TestLocal(t *testing.T) {
	sessions := sessionService.New("test-secret")
	s, err := New(Config{Mode: constant.AuthModeLocal}, sessions, nil)
	require.NoError(t, err)

	token, err := sessions.CreateToken(7, true)
	require.NoError(t, err)
	session, err := s.Authenticate(context.Background(), token)
	require.NoError(t, err)
	assert.Equal(t, int64(7), session.Id)
	assert.True(t, session.Admin)

	session, err = s.Authenticate(context.Background(), unsigned(t, adminClaims("sub-1", "admin")))
	require.NoError(t, err, "чужой токен — не сбой")
	assert.Nil(t, session, "в режиме local внешний токен не годится")
}

func TestProxyCreatesUserWithoutProjects(t *testing.T) {
	usr := newUsrFake()
	s := newProxy(t, usr)
	ctx := context.Background()

	session, err := s.Authenticate(ctx, unsigned(t, jwtv5.MapClaims{"sub": "sub-oleg", "preferred_username": "Oleg", "name": "Олег"}))
	require.NoError(t, err)
	require.NotNil(t, session)
	assert.False(t, session.Admin)

	oleg := usr.get(session.Id)
	assert.Equal(t, "oleg", oleg.Username)
	assert.Equal(t, "Олег", oleg.Name)
	assert.Equal(t, "sub-oleg", oleg.ExtSub)
	assert.True(t, oleg.Active)
	assert.False(t, oleg.AllProjects, "новый — ничего не видит")
	assert.Empty(t, oleg.ProjectIds)

	_, err = s.Authenticate(ctx, unsigned(t, jwtv5.MapClaims{"sub": "sub-oleg", "preferred_username": "oleg", "name": "Олег"}))
	require.NoError(t, err)
	assert.Equal(t, 1, usr.creates, "второй вход — тот же человек")

	session, err = s.Authenticate(ctx, "not-a-jwt")
	require.NoError(t, err, "мусор — пустая сессия, не сбой")
	assert.Nil(t, session)
	session, err = s.Authenticate(ctx, unsigned(t, jwtv5.MapClaims{"preferred_username": "oleg"}))
	require.NoError(t, err)
	assert.Nil(t, session, "без sub — не годится")
}

func TestProxyAdminFromToken(t *testing.T) {
	usr := newUsrFake()
	s := newProxy(t, usr)
	ctx := context.Background()

	session, err := s.Authenticate(ctx, unsigned(t, adminClaims("sub-a", "admin")))
	require.NoError(t, err)
	assert.True(t, session.Admin)
	assert.True(t, usr.get(session.Id).IsAdmin, "флаг в «Людях» — как в токене")

	// роль сняли у провайдера: со следующим токеном — не админ, кэш не мешает
	session, err = s.Authenticate(ctx, unsigned(t, jwtv5.MapClaims{"sub": "sub-a", "preferred_username": "admin", "name": "Админ"}))
	require.NoError(t, err)
	assert.False(t, session.Admin)
	assert.False(t, usr.get(session.Id).IsAdmin)
}

func TestProxyLinksPrecreatedUser(t *testing.T) {
	usr := newUsrFake(&usrModel.Main{Id: 5, Active: true, Username: "oleg", Password: "hash", ProjectIds: []int64{2, 3}})
	s := newProxy(t, usr)

	session, err := s.Authenticate(context.Background(), unsigned(t, jwtv5.MapClaims{"sub": "sub-oleg", "preferred_username": "oleg", "name": "Олег"}))
	require.NoError(t, err)
	assert.Equal(t, int64(5), session.Id, "заведённый заранее — привязан по логину")
	oleg := usr.get(5)
	assert.Equal(t, "sub-oleg", oleg.ExtSub)
	assert.Equal(t, []int64{2, 3}, oleg.ProjectIds, "проекты сохранились")
	assert.Equal(t, "Олег", oleg.Name)
	assert.Equal(t, 0, usr.creates)
}

func TestProxyUsernameTakenByOtherSub(t *testing.T) {
	usr := newUsrFake(&usrModel.Main{Id: 5, Active: true, Username: "oleg", ExtSub: "sub-old"})
	s := newProxy(t, usr)

	session, err := s.Authenticate(context.Background(), unsigned(t, jwtv5.MapClaims{"sub": "sub-new", "preferred_username": "oleg"}))
	require.NoError(t, err)
	assert.NotEqual(t, int64(5), session.Id, "чужой sub — не тот же человек")
	assert.Equal(t, "oleg-"+shortHash("sub-new"), usr.get(session.Id).Username)
	assert.Equal(t, "sub-old", usr.get(5).ExtSub)
}

func TestProxyRenameKeepsTakenUsername(t *testing.T) {
	usr := newUsrFake(
		&usrModel.Main{Id: 5, Active: true, Username: "oleg", ExtSub: "sub-oleg"},
		&usrModel.Main{Id: 6, Active: true, Username: "o.kim"},
	)
	s := newProxy(t, usr)

	session, err := s.Authenticate(context.Background(), unsigned(t, jwtv5.MapClaims{"sub": "sub-oleg", "preferred_username": "o.kim", "name": "Олег Ким"}))
	require.NoError(t, err)
	assert.Equal(t, int64(5), session.Id)
	oleg := usr.get(5)
	assert.Equal(t, "oleg", oleg.Username, "новый логин занят — остаётся старый")
	assert.Equal(t, "Олег Ким", oleg.Name, "остальное обновилось")
}

func TestProxyDisabledUser(t *testing.T) {
	usr := newUsrFake(&usrModel.Main{Id: 5, Active: false, Username: "oleg", ExtSub: "sub-oleg"})
	s := newProxy(t, usr)

	_, err := s.Authenticate(context.Background(), unsigned(t, jwtv5.MapClaims{"sub": "sub-oleg", "preferred_username": "oleg"}))
	assert.ErrorIs(t, err, errs.UsrDisabled, "отключён в «Людях» — отказ, а не новый человек")
	assert.Equal(t, 0, usr.creates)
}

func TestProxyConcurrentFirstLogin(t *testing.T) {
	usr := newUsrFake()
	s := newProxy(t, usr)
	token := unsigned(t, jwtv5.MapClaims{"sub": "sub-oleg", "preferred_username": "oleg"})

	ids := make([]int64, 8)
	var wg sync.WaitGroup
	for i := range ids {
		wg.Go(func() {
			session, err := s.Authenticate(context.Background(), token)
			assert.NoError(t, err)
			if session != nil {
				ids[i] = session.Id
			}
		})
	}
	wg.Wait()
	for _, id := range ids {
		assert.Equal(t, ids[0], id, "все запросы — один человек")
	}
	assert.Equal(t, 1, usr.creates)
}

// oidcProvider — discovery и JWKS на httptest: ключ провайдера RSA, токены RS256
type oidcProvider struct {
	server *httptest.Server
	key    *rsa.PrivateKey
}

func newOidcProvider(t *testing.T) *oidcProvider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	p := &oidcProvider{key: key}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                p.server.URL,
			"authorization_endpoint":                p.server.URL + "/auth",
			"token_endpoint":                        p.server.URL + "/token",
			"jwks_uri":                              p.server.URL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{
			"kty": "RSA", "kid": "k1", "use": "sig", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	})
	p.server = httptest.NewServer(mux)
	t.Cleanup(p.server.Close)
	return p
}

func (p *oidcProvider) sign(t *testing.T, key *rsa.PrivateKey, claims jwtv5.MapClaims) string {
	t.Helper()
	token := jwtv5.NewWithClaims(jwtv5.SigningMethodRS256, claims)
	token.Header["kid"] = "k1"
	signed, err := token.SignedString(key)
	require.NoError(t, err)
	return signed
}

func (p *oidcProvider) claims(extra jwtv5.MapClaims) jwtv5.MapClaims {
	claims := jwtv5.MapClaims{
		"iss": p.server.URL, "sub": "sub-oleg", "preferred_username": "oleg", "aud": "zorko",
		"exp": time.Now().Add(5 * time.Minute).Unix(), "iat": time.Now().Unix(),
	}
	for k, v := range extra {
		claims[k] = v
	}
	return claims
}

func TestOidc(t *testing.T) {
	p := newOidcProvider(t)
	usr := newUsrFake()
	s, err := New(Config{Mode: constant.AuthModeOidc, OidcIssuer: p.server.URL + "/", OidcAudience: "zorko", AdminRole: "zorko-admin"}, sessionService.New(""), usr)
	require.NoError(t, err)
	ctx := context.Background()

	session, err := s.Authenticate(ctx, p.sign(t, p.key, p.claims(jwtv5.MapClaims{
		"realm_access": map[string]any{"roles": []any{"zorko-admin"}},
	})))
	require.NoError(t, err)
	require.NotNil(t, session, "подпись провайдера, свой iss и aud")
	assert.True(t, session.Admin)
	assert.Equal(t, "oleg", usr.get(session.Id).Username)

	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	rejected := map[string]string{
		"чужая подпись": p.sign(t, otherKey, p.claims(nil)),
		"просрочен":     p.sign(t, p.key, p.claims(jwtv5.MapClaims{"exp": time.Now().Add(-time.Minute).Unix()})),
		"чужой iss":     p.sign(t, p.key, p.claims(jwtv5.MapClaims{"iss": "https://evil.example"})),
		"чужой aud":     p.sign(t, p.key, p.claims(jwtv5.MapClaims{"aud": "orders"})),
		"без подписи":   unsigned(t, p.claims(nil)),
		"не JWT":        "garbage",
	}
	for name, token := range rejected {
		session, err = s.Authenticate(ctx, token)
		assert.NoError(t, err, name+": не сбой")
		assert.Nil(t, session, name)
	}
}

func TestOidcProviderUnavailable(t *testing.T) {
	p := newOidcProvider(t)
	issuer := p.server.URL
	p.server.Close()

	s, err := New(Config{Mode: constant.AuthModeOidc, OidcIssuer: issuer}, sessionService.New(""), newUsrFake())
	require.NoError(t, err, "провайдер недоступен — zorko всё равно поднимается")

	_, err = s.Authenticate(context.Background(), "any.jwt.token")
	assert.ErrorIs(t, err, errProviderUnavailable, "сбой (503), а не негодный токен (401)")
	_, err = s.Authenticate(context.Background(), "any.jwt.token")
	assert.ErrorIs(t, err, errProviderUnavailable, "повтор discovery — не чаще раза в 10 с")
}
