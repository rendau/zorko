package oauth

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/rendau/zorko/internal/domain/oauth/model"
	oauthService "github.com/rendau/zorko/internal/domain/oauth/service"
	"github.com/rendau/zorko/internal/errs"
)

const (
	grantTypeAuthorizationCode = "authorization_code"
	grantTypeRefreshToken      = "refresh_token"

	responseTypeCode = "code"

	codeChallengeMethodS256 = "S256"

	clientNameMaxLen  = 100
	redirectUrisMax   = 10
	redirectUriMaxLen = 2000
	defaultClientName = "MCP client"
)

var (
	supportedGrantTypes    = []string{grantTypeAuthorizationCode, grantTypeRefreshToken}
	supportedResponseTypes = []string{responseTypeCode}
	supportedAuthMethods   = []string{model.ClientAuthNone, model.ClientAuthSecretPost, model.ClientAuthSecretBasic}
)

type RegisterReq struct {
	RedirectUris            []string
	ClientName              string
	TokenEndpointAuthMethod string
	GrantTypes              []string
	ResponseTypes           []string
}

type RegisterRep struct {
	ClientId      string
	ClientSecret  string // пусто у клиента без секрета
	IssuedAt      time.Time
	Client        *model.Client
	GrantTypes    []string
	ResponseTypes []string
}

// Register — динамическая регистрация клиента (RFC 7591). По протоколу она открыта (клиент
// регистрируется до входа человека), поэтому принимает только адреса возврата из настроенного
// списка: код уйдёт лишь известному клиенту.
func (u *Usecase) Register(_ context.Context, req *RegisterReq) (*RegisterRep, error) {
	if !u.conf.Enabled {
		return nil, errNotEnabled
	}

	if len(req.RedirectUris) == 0 || len(req.RedirectUris) > redirectUrisMax {
		return nil, newError(ErrInvalidRedirectUri, "redirect_uris must list 1 to %d URIs", redirectUrisMax)
	}
	for _, uri := range req.RedirectUris {
		if len(uri) > redirectUriMaxLen || !oauthService.RedirectAllowed(u.conf.RedirectUris, uri) {
			return nil, newError(ErrInvalidRedirectUri, "redirect URI %q is not allowed by this server", uri)
		}
	}

	grantTypes := req.GrantTypes
	if len(grantTypes) == 0 {
		grantTypes = []string{grantTypeAuthorizationCode}
	}
	for _, v := range grantTypes {
		if !slices.Contains(supportedGrantTypes, v) {
			return nil, newError(ErrInvalidClientMetadata, "grant type %q is not supported", v)
		}
	}

	responseTypes := req.ResponseTypes
	if len(responseTypes) == 0 {
		responseTypes = supportedResponseTypes
	}
	for _, v := range responseTypes {
		if !slices.Contains(supportedResponseTypes, v) {
			return nil, newError(ErrInvalidClientMetadata, "response type %q is not supported", v)
		}
	}

	// RFC 7591: без указания — client_secret_basic
	authMethod := req.TokenEndpointAuthMethod
	if authMethod == "" {
		authMethod = model.ClientAuthSecretBasic
	}
	if !slices.Contains(supportedAuthMethods, authMethod) {
		return nil, newError(ErrInvalidClientMetadata, "token endpoint auth method %q is not supported", authMethod)
	}

	name := strings.TrimSpace(req.ClientName)
	if name == "" {
		name = defaultClientName
	}
	if runes := []rune(name); len(runes) > clientNameMaxLen {
		name = string(runes[:clientNameMaxLen])
	}

	client := &model.Client{
		Name:         name,
		RedirectUris: req.RedirectUris,
		AuthMethod:   authMethod,
	}

	clientId, err := u.clientCodec.Encode(client)
	if err != nil {
		return nil, fmt.Errorf("clientCodec.Encode: %w", err)
	}

	result := &RegisterRep{
		ClientId:      clientId,
		IssuedAt:      time.Now(),
		Client:        client,
		GrantTypes:    grantTypes,
		ResponseTypes: responseTypes,
	}
	if authMethod != model.ClientAuthNone {
		result.ClientSecret = u.clientCodec.Secret(clientId)
	}

	return result, nil
}

// AuthorizeReq — запрос авторизации клиента: параметры адреса страницы согласия как есть
type AuthorizeReq struct {
	ResponseType        string
	ClientId            string
	RedirectUri         string
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
	Scope               string
	Resource            string
}

type AuthorizeInfo struct {
	ClientName   string
	RedirectUri  string
	RedirectHost string
}

// DescribeAuthorization проверяет запрос авторизации для страницы согласия и описывает клиента
// человеку
func (u *Usecase) DescribeAuthorization(ctx context.Context, req *AuthorizeReq) (*AuthorizeInfo, error) {
	if !u.sessionSvc.FromContext(ctx).IsAuthorized() {
		return nil, errs.NotAuthorized
	}

	client, err := u.checkAuthorizeReq(req)
	if err != nil {
		return nil, err
	}

	redirect, _ := url.Parse(req.RedirectUri)

	return &AuthorizeInfo{
		ClientName:   client.Name,
		RedirectUri:  req.RedirectUri,
		RedirectHost: redirect.Host,
	}, nil
}

// Authorize записывает решение человека на странице согласия и возвращает адрес, куда вернуть
// браузер клиенту: с кодом при согласии, иначе с error=access_denied
func (u *Usecase) Authorize(ctx context.Context, req *AuthorizeReq, approve bool) (string, error) {
	session := u.sessionSvc.FromContext(ctx)
	if !session.IsAuthorized() {
		return "", errs.NotAuthorized
	}

	client, err := u.checkAuthorizeReq(req)
	if err != nil {
		return "", err
	}

	slog.Info("oauth: consent", "approve", approve, "client_name", client.Name, "redirect_uri", req.RedirectUri, "usr_id", session.Id)

	if !approve {
		return redirectWith(req.RedirectUri, map[string]string{
			"error": "access_denied",
			"state": req.State,
		}), nil
	}

	// свой токен входа живёт 15 минут без похода в БД — отключённый за это время не подключит клиента
	usr, found, err := u.usrSvc.Get(ctx, session.Id, false)
	if err != nil {
		return "", fmt.Errorf("usrSvc.Get: %w", err)
	}
	if !found || !usr.Active {
		return "", errs.NotAuthorized
	}

	_, code, err := u.svc.CreateGrant(ctx, &model.Grant{
		UsrId:         session.Id,
		ClientId:      req.ClientId,
		ClientName:    client.Name,
		RedirectUri:   req.RedirectUri,
		CodeChallenge: req.CodeChallenge,
	}, codeTTL)
	if err != nil {
		return "", fmt.Errorf("svc.CreateGrant: %w", err)
	}

	return redirectWith(req.RedirectUri, map[string]string{
		"code":  code,
		"state": req.State,
	}), nil
}

func (u *Usecase) checkAuthorizeReq(req *AuthorizeReq) (*model.Client, error) {
	if !u.conf.Enabled {
		return nil, errs.OAuthNotEnabled
	}

	client, ok := u.clientCodec.Decode(req.ClientId)
	if !ok {
		return nil, invalidAuthorize("unknown client_id: register the client again")
	}
	// адрес сверяется и с текущим конфигом: убранный из конфига адрес перестаёт работать и у
	// клиентов, зарегистрированных раньше
	if !slices.Contains(client.RedirectUris, req.RedirectUri) ||
		!oauthService.RedirectAllowed(u.conf.RedirectUris, req.RedirectUri) {
		return nil, invalidAuthorize("redirect_uri is not registered for this client")
	}

	if req.ResponseType != responseTypeCode {
		return nil, invalidAuthorize("response_type must be code")
	}
	if req.CodeChallenge == "" || req.CodeChallengeMethod != codeChallengeMethodS256 {
		return nil, invalidAuthorize("PKCE is required: code_challenge with code_challenge_method S256")
	}
	if !u.resourceMatches(req.Resource) {
		return nil, invalidAuthorize(fmt.Sprintf("resource %q is not this server (%s)", req.Resource, u.conf.Issuer))
	}

	return client, nil
}

// resourceMatches сверяет resource (RFC 8707), если он передан, с адресом MCP-сервера
func (u *Usecase) resourceMatches(resource string) bool {
	if resource == "" {
		return true
	}
	return normalizeURL(resource) == normalizeURL(u.conf.Issuer)
}

func normalizeURL(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return value
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	return parsed.String()
}

func redirectWith(redirectUri string, params map[string]string) string {
	parsed, err := url.Parse(redirectUri)
	if err != nil {
		return redirectUri
	}
	query := parsed.Query()
	for key, value := range params {
		if value != "" {
			query.Set(key, value)
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func invalidAuthorize(desc string) error {
	return errs.ErrFull{Err: errs.OAuthInvalidRequest, Desc: desc}
}
