package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	oauthRepoMem "github.com/rendau/zorko/internal/domain/oauth/repo/mem"
	oauthService "github.com/rendau/zorko/internal/domain/oauth/service"
	orgModel "github.com/rendau/zorko/internal/domain/org/model"
	projectModel "github.com/rendau/zorko/internal/domain/project/model"
	sessionModel "github.com/rendau/zorko/internal/domain/session/model"
	sessionService "github.com/rendau/zorko/internal/domain/session/service"
	usrModel "github.com/rendau/zorko/internal/domain/usr/model"
	"github.com/rendau/zorko/internal/errs"
	accessService "github.com/rendau/zorko/internal/service/access/service"
	oauthUsc "github.com/rendau/zorko/internal/usecase/oauth"
	orgUsc "github.com/rendau/zorko/internal/usecase/org"
	projectUsc "github.com/rendau/zorko/internal/usecase/project"
)

const (
	testAdminURL = "https://zorko.example.com"
	testRedirect = "http://localhost:53124/callback"
	testVerifier = "verifier-verifier-verifier-verifier-verifier"
)

type usrFake map[int64]*usrModel.Main

func (f usrFake) Get(_ context.Context, id int64, _ bool) (*usrModel.Main, bool, error) {
	v, ok := f[id]
	return v, ok, nil
}

type orgFake []*orgModel.Main

func (f orgFake) List(_ context.Context, pars *orgModel.ListReq) ([]*orgModel.Main, int64, error) {
	if pars.Page > 0 {
		return nil, 0, nil
	}
	items := lo.Filter(f, func(o *orgModel.Main, _ int) bool {
		return pars.IDs == nil || slices.Contains(*pars.IDs, o.Id)
	})
	return items, int64(len(items)), nil
}

func (f orgFake) Get(context.Context, int64, bool) (*orgModel.Main, bool, error) {
	return nil, false, nil
}
func (f orgFake) Create(context.Context, *orgModel.Edit) (int64, error) { return 0, nil }
func (f orgFake) Update(context.Context, int64, *orgModel.Edit) error   { return nil }
func (f orgFake) Delete(context.Context, int64) error                   { return nil }

type projectFake []*projectModel.Main

func (f projectFake) List(_ context.Context, pars *projectModel.ListReq) ([]*projectModel.Main, int64, error) {
	if pars.Page > 0 {
		return nil, 0, nil
	}
	items := lo.Filter(f, func(p *projectModel.Main, _ int) bool {
		return (pars.OrgId == nil || p.OrgId == *pars.OrgId) && (pars.IDs == nil || slices.Contains(*pars.IDs, p.Id))
	})
	return items, int64(len(items)), nil
}

func (f projectFake) Get(_ context.Context, id int64, _ bool) (*projectModel.Main, bool, error) {
	p, ok := lo.Find(f, func(p *projectModel.Main) bool { return p.Id == id })
	return p, ok, nil
}
func (f projectFake) Create(context.Context, *projectModel.Edit) (int64, error) { return 0, nil }
func (f projectFake) Update(context.Context, int64, *projectModel.Edit) error   { return nil }
func (f projectFake) Delete(context.Context, int64) error                       { return nil }

type toolCall struct {
	tool string
	err  error
}

type fixture struct {
	server   *httptest.Server
	oauth    *oauthUsc.Usecase
	sessions *sessionService.Service
	usrs     usrFake

	mu    sync.Mutex
	calls []toolCall
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	f := &fixture{
		sessions: sessionService.New("test-secret"),
		usrs: usrFake{
			1: {Id: 1, Active: true, IsAdmin: true, Username: "admin"},
			2: {Id: 2, Active: true, Username: "oleg", ProjectIds: []int64{11}},
		},
	}
	orgs := orgFake{{Id: 1, Slug: "shop", Name: "Shop"}, {Id: 2, Slug: "bank", Name: "Bank"}}
	projects := projectFake{
		{Id: 11, OrgId: 1, Slug: "web", Name: "Web", Platform: "javascript-vue", ReplayMode: "error", SiteFetchHeaderValue: "site-secret"},
		{Id: 12, OrgId: 1, Slug: "app", Name: "App"},
		{Id: 21, OrgId: 2, Slug: "web", Name: "Bank web"},
	}
	access := accessService.New(f.sessions, f.usrs)

	// адрес сервера известен только после запуска, а OAuth нужен до него — обработчик подставляется позже
	var handler http.Handler
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(f.server.Close)

	f.oauth = oauthUsc.New(oauthUsc.Config{
		Enabled:      true,
		Issuer:       f.server.URL,
		AdminURL:     testAdminURL,
		RedirectUris: []string{"http://localhost"},
		AccessTTL:    time.Hour,
		RefreshTTL:   24 * time.Hour,
	}, oauthService.New(oauthRepoMem.New()), oauthService.NewClientCodec("test-secret"), f.usrs, f.sessions)

	handler = New(Config{AdminURL: testAdminURL + "/", Version: "test"}, f.sessions, f.oauth,
		orgUsc.New(orgs, f.sessions, projects, access),
		projectUsc.New(projects, f.sessions, access, true),
		nil, nil,
		func(tool string, err error, _ time.Duration) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.calls = append(f.calls, toolCall{tool: tool, err: err})
		},
	).HTTPHandler()

	return f
}

func (f *fixture) postForm(t *testing.T, path string, form url.Values, out any) int {
	t.Helper()
	resp, err := http.PostForm(f.server.URL+path, form)
	require.NoError(t, err)
	defer resp.Body.Close()
	if out != nil {
		require.NoError(t, json.NewDecoder(resp.Body).Decode(out))
	}
	return resp.StatusCode
}

func (f *fixture) getJSON(t *testing.T, path string, out any) int {
	t.Helper()
	resp, err := http.Get(f.server.URL + path)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.NoError(t, json.NewDecoder(resp.Body).Decode(out))
	return resp.StatusCode
}

// connect проходит весь вход, как MCP-клиент: регистрация, согласие человека, обмен кода
func (f *fixture) connect(t *testing.T, usrId int64) (accessToken, refreshToken, clientId string) {
	t.Helper()

	body := strings.NewReader(`{"client_name":"Claude Code","redirect_uris":["` + testRedirect + `"],"token_endpoint_auth_method":"none"}`)
	resp, err := http.Post(f.server.URL+"/register", "application/json", body)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var client struct {
		ClientId string `json:"client_id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&client))

	sum := sha256.Sum256([]byte(testVerifier))
	redirect, err := f.oauth.Authorize(
		f.sessions.WithContext(context.Background(), &sessionModel.Session{Id: usrId}),
		&oauthUsc.AuthorizeReq{
			ResponseType:        "code",
			ClientId:            client.ClientId,
			RedirectUri:         testRedirect,
			CodeChallenge:       base64.RawURLEncoding.EncodeToString(sum[:]),
			CodeChallengeMethod: "S256",
			Resource:            f.server.URL,
		}, true)
	require.NoError(t, err)
	parsed, err := url.Parse(redirect)
	require.NoError(t, err)

	var tokens struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
	}
	status := f.postForm(t, "/token", url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {parsed.Query().Get("code")},
		"redirect_uri":  {testRedirect},
		"code_verifier": {testVerifier},
		"client_id":     {client.ClientId},
		"resource":      {f.server.URL},
	}, &tokens)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "Bearer", tokens.TokenType)
	require.True(t, strings.HasPrefix(tokens.AccessToken, oauthService.AccessTokenPrefix))

	return tokens.AccessToken, tokens.RefreshToken, client.ClientId
}

type bearerTransport struct {
	token string
}

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

func (f *fixture) session(t *testing.T, token string) *mcpsdk.ClientSession {
	t.Helper()
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "0"}, nil)
	session, err := client.Connect(context.Background(), &mcpsdk.StreamableClientTransport{
		Endpoint:             f.server.URL,
		HTTPClient:           &http.Client{Transport: bearerTransport{token: token}},
		DisableStandaloneSSE: true,
	}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func resultText(res *mcpsdk.CallToolResult) string {
	parts := lo.FilterMap(res.Content, func(c mcpsdk.Content, _ int) (string, bool) {
		text, ok := c.(*mcpsdk.TextContent)
		if !ok {
			return "", false
		}
		return text.Text, true
	})
	return strings.Join(parts, "\n")
}

func TestUnauthorized(t *testing.T) {
	f := newFixture(t)

	post := func(token string) *http.Response {
		req, err := http.NewRequest(http.MethodPost, f.server.URL, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { _ = resp.Body.Close() })
		return resp
	}

	// без токена: 401 и адрес, с которого клиент начинает вход
	resp := post("")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("WWW-Authenticate"), `resource_metadata="`+f.server.URL+`/.well-known/oauth-protected-resource"`)

	for _, token := range []string{"garbage", "zko_unknown", "eyJhbGciOiJIUzI1NiJ9.e30.x"} {
		resp = post(token)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, token)
		assert.Contains(t, resp.Header.Get("WWW-Authenticate"), `error="invalid_token"`, token)
	}

	// refresh-токен на MCP-порту не работает
	_, refresh, _ := f.connect(t, 2)
	assert.Equal(t, http.StatusUnauthorized, post(refresh).StatusCode)
}

func TestMetadata(t *testing.T) {
	f := newFixture(t)

	var resource struct {
		Resource             string   `json:"resource"`
		AuthorizationServers []string `json:"authorization_servers"`
	}
	require.Equal(t, http.StatusOK, f.getJSON(t, "/.well-known/oauth-protected-resource", &resource))
	assert.Equal(t, f.server.URL, resource.Resource)
	assert.Equal(t, []string{f.server.URL}, resource.AuthorizationServers)

	var server struct {
		Issuer                string   `json:"issuer"`
		AuthorizationEndpoint string   `json:"authorization_endpoint"`
		TokenEndpoint         string   `json:"token_endpoint"`
		RegistrationEndpoint  string   `json:"registration_endpoint"`
		Methods               []string `json:"code_challenge_methods_supported"`
	}
	require.Equal(t, http.StatusOK, f.getJSON(t, "/.well-known/oauth-authorization-server", &server))
	assert.Equal(t, f.server.URL, server.Issuer)
	// согласие — страница интерфейса, остальное — на MCP-порту
	assert.Equal(t, testAdminURL+"/oauth/authorize", server.AuthorizationEndpoint)
	assert.Equal(t, f.server.URL+"/token", server.TokenEndpoint)
	assert.Equal(t, f.server.URL+"/register", server.RegistrationEndpoint)
	assert.Equal(t, []string{"S256"}, server.Methods)
}

func TestRegisterRejectsUnknownRedirect(t *testing.T) {
	f := newFixture(t)

	resp, err := http.Post(f.server.URL+"/register", "application/json",
		strings.NewReader(`{"redirect_uris":["https://evil.example/cb"]}`))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var body struct {
		Error string `json:"error"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "invalid_redirect_uri", body.Error)
}

func TestTools(t *testing.T) {
	f := newFixture(t)
	token, _, _ := f.connect(t, 2)
	session := f.session(t, token)

	tools, err := session.ListTools(context.Background(), nil)
	require.NoError(t, err)

	names := lo.Map(tools.Tools, func(tool *mcpsdk.Tool, _ int) string { return tool.Name })
	assert.ElementsMatch(t, []string{
		"list_project",
		"list_issue", "get_issue", "get_issue_stat", "get_issue_tags",
		"list_event", "get_event",
		"list_tag_key", "list_tag_value",
		"list_replay", "get_replay", "list_issue_replay",
	}, names)
	for _, tool := range tools.Tools {
		require.NotNil(t, tool.Annotations, tool.Name)
		assert.True(t, tool.Annotations.ReadOnlyHint, "%s: инструментов записи нет", tool.Name)
		assert.NotEmpty(t, tool.Description, tool.Name)
	}
}

func TestListProject(t *testing.T) {
	f := newFixture(t)

	call := func(usrId int64) ListProjectOut {
		token, _, _ := f.connect(t, usrId)
		res, err := f.session(t, token).CallTool(context.Background(), &mcpsdk.CallToolParams{Name: "list_project"})
		require.NoError(t, err)
		require.False(t, res.IsError, resultText(res))
		assert.NotContains(t, resultText(res), "site-secret", "секрет проекта в ответ не попадает")

		var out ListProjectOut
		require.NoError(t, json.Unmarshal([]byte(resultText(res)), &out))
		return out
	}

	// участник видит только организацию и проект, которые ему выдали
	member := call(2)
	require.Len(t, member.Orgs, 1)
	assert.Equal(t, "shop", member.Orgs[0].Slug)
	assert.Equal(t, []ProjectOut{{Id: 11, Slug: "web", Name: "Web", Platform: "javascript-vue", ReplayMode: "error"}}, member.Orgs[0].Projects)

	admin := call(1)
	require.Len(t, admin.Orgs, 2)
	assert.Len(t, admin.Orgs[0].Projects, 2)

	f.mu.Lock()
	defer f.mu.Unlock()
	require.Len(t, f.calls, 2)
	assert.Equal(t, toolCall{tool: "list_project"}, f.calls[0])
}

func TestToolErrors(t *testing.T) {
	f := newFixture(t)
	adminToken, _, _ := f.connect(t, 1)
	memberToken, _, _ := f.connect(t, 2)

	call := func(token, tool string, args map[string]any) *mcpsdk.CallToolResult {
		res, err := f.session(t, token).CallTool(context.Background(), &mcpsdk.CallToolParams{Name: tool, Arguments: args})
		require.NoError(t, err)
		return res
	}

	// админу доступны две организации — нужно назвать одну
	res := call(adminToken, "list_issue", nil)
	require.True(t, res.IsError)
	assert.Contains(t, resultText(res), "invalid_request")
	assert.Contains(t, resultText(res), "shop, bank")

	// чужая организация для участника — как несуществующая
	res = call(memberToken, "list_tag_key", map[string]any{"org": "bank"})
	require.True(t, res.IsError)
	assert.Contains(t, resultText(res), `"bank" не найдена, доступны: shop`)

	// чужой проект — тоже
	res = call(memberToken, "list_replay", map[string]any{"projects": []string{"app"}})
	require.True(t, res.IsError)
	assert.Contains(t, resultText(res), `проект "app" не найден`)

	// один slug в двух организациях — get_replay просит id
	res = call(adminToken, "get_replay", map[string]any{"project": "web", "replay_id": "x"})
	require.True(t, res.IsError)
	assert.Contains(t, resultText(res), "11, 21")

	f.mu.Lock()
	defer f.mu.Unlock()
	require.Len(t, f.calls, 4)
	for _, c := range f.calls {
		assert.Equal(t, errs.InvalidRequest.Error(), errs.Code(c.err), c.tool)
	}
}

func TestDisconnectedGrant(t *testing.T) {
	f := newFixture(t)
	token, _, _ := f.connect(t, 2)
	session := f.session(t, token)

	ctx := f.sessions.WithContext(context.Background(), &sessionModel.Session{Id: 2})
	grants, err := f.oauth.ListGrants(ctx)
	require.NoError(t, err)
	require.Len(t, grants, 1)
	require.NoError(t, f.oauth.DeleteGrant(ctx, grants[0].Id))

	// отключили в интерфейсе — следующий же вызов не проходит
	_, err = session.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: "list_project"})
	require.Error(t, err)
}
