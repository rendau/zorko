package mcp

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	oauthUsc "github.com/rendau/zorko/internal/usecase/oauth"
)

const (
	wellKnownResource   = "/.well-known/oauth-protected-resource"
	wellKnownAuthServer = "/.well-known/oauth-authorization-server"
	wellKnownOpenId     = "/.well-known/openid-configuration"

	pathRegister = "/register"
	pathToken    = "/token"
	pathRevoke   = "/revoke"

	// страница согласия в админке
	adminAuthorizePath = "/oauth/authorize"

	oauthFormMaxBytes = 64 * 1024
)

type resourceMetadata struct {
	Resource               string   `json:"resource"`
	AuthorizationServers   []string `json:"authorization_servers"`
	ScopesSupported        []string `json:"scopes_supported"`
	BearerMethodsSupported []string `json:"bearer_methods_supported"`
	ResourceName           string   `json:"resource_name"`
}

type authServerMetadata struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	RegistrationEndpoint              string   `json:"registration_endpoint"`
	RevocationEndpoint                string   `json:"revocation_endpoint"`
	ScopesSupported                   []string `json:"scopes_supported"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
}

type registerIn struct {
	RedirectUris            []string `json:"redirect_uris"`
	ClientName              string   `json:"client_name"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
}

type registerOut struct {
	ClientId                string   `json:"client_id"`
	ClientSecret            string   `json:"client_secret,omitempty"`
	ClientIdIssuedAt        int64    `json:"client_id_issued_at"`
	ClientSecretExpiresAt   *int64   `json:"client_secret_expires_at,omitempty"`
	ClientName              string   `json:"client_name"`
	RedirectUris            []string `json:"redirect_uris"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
}

type tokenOut struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

type errorOut struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

// oauthEndpoints — OAuth-адреса на MCP-порту. issuer — публичный адрес
// MCP-сервера; его путь (если сервер опубликован под путём, например
// https://host/mcp) префиксует каждый адрес.
type oauthEndpoints struct {
	uc       *oauthUsc.Usecase
	issuer   string
	basePath string
	adminURL string
}

func newOAuthEndpoints(uc *oauthUsc.Usecase) *oauthEndpoints {
	conf := uc.Config()
	issuer := strings.TrimSuffix(conf.Issuer, "/")

	basePath := ""
	if parsed, err := url.Parse(issuer); err == nil {
		basePath = strings.TrimSuffix(parsed.Path, "/")
	}

	return &oauthEndpoints{
		uc:       uc,
		issuer:   issuer,
		basePath: basePath,
		adminURL: strings.TrimSuffix(conf.AdminURL, "/"),
	}
}

// resourceMetadataURL уходит в WWW-Authenticate ответа 401. Он под путём
// сервера, поэтому доходит до zorko, даже если опубликован только этот путь.
func (e *oauthEndpoints) resourceMetadataURL() string {
	return e.issuer + wellKnownResource
}

func (e *oauthEndpoints) register(mux *http.ServeMux) {
	patterns := map[string]http.HandlerFunc{}
	add := func(path string, handler http.HandlerFunc) {
		// при пустом basePath «вставленная» и «дописанная» формы well-known
		// из RFC 8414/9728 — один и тот же путь
		patterns[path] = handler
	}

	// метаданные: и под путём сервера, и в форме RFC, где путь сервера идёт
	// после well-known суффикса
	add(e.basePath+wellKnownResource, e.handleResourceMetadata)
	add(wellKnownResource+e.basePath, e.handleResourceMetadata)
	add(e.basePath+wellKnownAuthServer, e.handleAuthServerMetadata)
	add(wellKnownAuthServer+e.basePath, e.handleAuthServerMetadata)
	add(e.basePath+wellKnownOpenId, e.handleAuthServerMetadata)
	add(wellKnownOpenId+e.basePath, e.handleAuthServerMetadata)

	add(e.basePath+pathRegister, e.handleRegister)
	add(e.basePath+pathToken, e.handleToken)
	add(e.basePath+pathRevoke, e.handleRevoke)

	for path, handler := range patterns {
		mux.Handle(path, withCORS(handler))
	}
}

func (e *oauthEndpoints) handleResourceMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, &resourceMetadata{
		Resource:               e.issuer,
		AuthorizationServers:   []string{e.issuer},
		ScopesSupported:        []string{oauthUsc.Scope},
		BearerMethodsSupported: []string{"header"},
		ResourceName:           "zorko — трекер ошибок",
	})
}

func (e *oauthEndpoints) handleAuthServerMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, &authServerMetadata{
		Issuer:                            e.issuer,
		AuthorizationEndpoint:             e.adminURL + adminAuthorizePath,
		TokenEndpoint:                     e.issuer + pathToken,
		RegistrationEndpoint:              e.issuer + pathRegister,
		RevocationEndpoint:                e.issuer + pathRevoke,
		ScopesSupported:                   []string{oauthUsc.Scope},
		ResponseTypesSupported:            []string{"code"},
		GrantTypesSupported:               []string{"authorization_code", "refresh_token"},
		TokenEndpointAuthMethodsSupported: []string{"none", "client_secret_post", "client_secret_basic"},
		CodeChallengeMethodsSupported:     []string{"S256"},
	})
}

func (e *oauthEndpoints) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	in := &registerIn{}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, oauthFormMaxBytes)).Decode(in); err != nil {
		writeOAuthError(w, r, &oauthUsc.Error{Code: oauthUsc.ErrInvalidClientMetadata, Description: "body must be a JSON object"})
		return
	}

	rep, err := e.uc.Register(r.Context(), &oauthUsc.RegisterReq{
		RedirectUris:            in.RedirectUris,
		ClientName:              in.ClientName,
		TokenEndpointAuthMethod: in.TokenEndpointAuthMethod,
		GrantTypes:              in.GrantTypes,
		ResponseTypes:           in.ResponseTypes,
	})
	if err != nil {
		writeOAuthError(w, r, err)
		return
	}

	out := &registerOut{
		ClientId:                rep.ClientId,
		ClientSecret:            rep.ClientSecret,
		ClientIdIssuedAt:        rep.IssuedAt.Unix(),
		ClientName:              rep.Client.Name,
		RedirectUris:            rep.Client.RedirectUris,
		TokenEndpointAuthMethod: rep.Client.AuthMethod,
		GrantTypes:              rep.GrantTypes,
		ResponseTypes:           rep.ResponseTypes,
	}
	if rep.ClientSecret != "" {
		out.ClientSecretExpiresAt = new(int64(0)) // не истекает
	}

	slog.Info("mcp oauth: client registered",
		"client_name", rep.Client.Name,
		"redirect_uris", rep.Client.RedirectUris,
		"auth_method", rep.Client.AuthMethod,
		"client_ip", clientIP(r),
	)

	writeJSON(w, http.StatusCreated, out)
}

func (e *oauthEndpoints) handleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, oauthFormMaxBytes)
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, r, &oauthUsc.Error{Code: oauthUsc.ErrInvalidRequest, Description: "body must be a form"})
		return
	}

	clientId, clientSecret := clientCredentials(r)

	rep, err := e.uc.Token(r.Context(), &oauthUsc.TokenReq{
		GrantType:    r.PostForm.Get("grant_type"),
		Code:         r.PostForm.Get("code"),
		RedirectUri:  r.PostForm.Get("redirect_uri"),
		CodeVerifier: r.PostForm.Get("code_verifier"),
		RefreshToken: r.PostForm.Get("refresh_token"),
		Resource:     r.PostForm.Get("resource"),
		ClientId:     clientId,
		ClientSecret: clientSecret,
	})
	if err != nil {
		writeOAuthError(w, r, err)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, &tokenOut{
		AccessToken:  rep.AccessToken,
		TokenType:    "Bearer",
		ExpiresIn:    rep.ExpiresIn,
		RefreshToken: rep.RefreshToken,
		Scope:        rep.Scope,
	})
}

func (e *oauthEndpoints) handleRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, oauthFormMaxBytes)
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, r, &oauthUsc.Error{Code: oauthUsc.ErrInvalidRequest, Description: "body must be a form"})
		return
	}

	clientId, clientSecret := clientCredentials(r)

	if err := e.uc.Revoke(r.Context(), r.PostForm.Get("token"), clientId, clientSecret); err != nil {
		writeOAuthError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// clientCredentials берёт клиента из HTTP basic auth (client_secret_basic) или
// из формы (client_secret_post, публичные клиенты).
func clientCredentials(r *http.Request) (string, string) {
	if id, secret, ok := r.BasicAuth(); ok {
		// RFC 6749, 2.3.1: оба form-urlencoded до basic-кодирования
		if v, err := url.QueryUnescape(id); err == nil {
			id = v
		}
		if v, err := url.QueryUnescape(secret); err == nil {
			secret = v
		}
		return id, secret
	}
	return r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
}

func writeOAuthError(w http.ResponseWriter, r *http.Request, err error) {
	w.Header().Set("Cache-Control", "no-store")

	oauthErr, ok := errors.AsType[*oauthUsc.Error](err)
	if !ok {
		slog.Error("mcp oauth: request failed", "path", r.URL.Path, "client_ip", clientIP(r), "error", err)
		writeJSON(w, http.StatusInternalServerError, &errorOut{Error: "server_error", ErrorDescription: "internal error"})
		return
	}

	slog.Warn("mcp oauth: request rejected",
		"path", r.URL.Path,
		"grant_type", r.PostForm.Get("grant_type"),
		"error", oauthErr.Code,
		"description", oauthErr.Description,
		"client_ip", clientIP(r),
	)

	if oauthErr.Code == oauthUsc.ErrInvalidClient {
		w.Header().Set("WWW-Authenticate", `Basic realm="zorko"`)
	}
	writeJSON(w, oauthErr.HTTPStatus(), &errorOut{Error: oauthErr.Code, ErrorDescription: oauthErr.Description})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("mcp oauth: write response", "error", err)
	}
}

// withCORS пускает браузерные MCP-клиенты (например, MCP Inspector) к
// метаданным и token endpoint; cookies тут нет, поэтому подходит любой origin.
func withCORS(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, MCP-Protocol-Version")
		w.Header().Set("Access-Control-Max-Age", "86400")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	})
}
