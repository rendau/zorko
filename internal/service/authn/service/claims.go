package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	jwtv5 "github.com/golang-jwt/jwt/v5"
)

// errProviderUnavailable — настройки провайдера (discovery) не получены: это сбой, а не негодный
// токен — интерфейс получит 503, а не 401 с уходом на вход
var errProviderUnavailable = errors.New("oidc provider unavailable")

// discoveryRetryAfter — не чаще одной попытки discovery за этот срок, пока провайдер недоступен
const discoveryRetryAfter = 10 * time.Second

// proxyClaims — режим proxy: токен уже проверил шлюз, подпись и срок не проверяются
type proxyClaims struct{}

func (proxyClaims) Claims(_ context.Context, token string) (map[string]any, error) {
	claims := jwtv5.MapClaims{}
	if _, _, err := jwtv5.NewParser().ParseUnverified(token, claims); err != nil {
		return nil, fmt.Errorf("ParseUnverified: %w", err)
	}
	return claims, nil
}

// oidcClaims — режим oidc: подпись по ключам провайдера (адрес — из discovery, ключи
// перечитываются при смене kid), iss, exp и, если задан, aud. Discovery — при первом токене,
// а не при старте: недоступный провайдер не мешает zorko подняться.
type oidcClaims struct {
	issuer   string
	audience string
	client   *http.Client

	mu       sync.Mutex
	verifier *oidc.IDTokenVerifier
	failedAt time.Time
}

func newOidcClaims(issuer, audience string, client *http.Client) *oidcClaims {
	return &oidcClaims{
		issuer:   strings.TrimSuffix(strings.TrimSpace(issuer), "/"),
		audience: strings.TrimSpace(audience),
		client:   client,
	}
}

func (o *oidcClaims) Claims(ctx context.Context, token string) (map[string]any, error) {
	verifier, err := o.getVerifier(ctx)
	if err != nil {
		return nil, err
	}

	idToken, err := verifier.Verify(oidc.ClientContext(ctx, o.client), token)
	if err != nil {
		return nil, fmt.Errorf("verifier.Verify: %w", err)
	}

	claims := map[string]any{}
	if err = idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("idToken.Claims: %w", err)
	}
	return claims, nil
}

func (o *oidcClaims) getVerifier(ctx context.Context) (*oidc.IDTokenVerifier, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.verifier != nil {
		return o.verifier, nil
	}
	if !o.failedAt.IsZero() && time.Since(o.failedAt) < discoveryRetryAfter {
		return nil, errProviderUnavailable
	}

	// контекст нужен только на сам запрос discovery: ключи go-oidc потом загружает
	// со своим фоновым контекстом и нашим клиентом
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()

	provider, err := oidc.NewProvider(oidc.ClientContext(ctx, o.client), o.issuer)
	if err != nil {
		o.failedAt = time.Now()
		return nil, fmt.Errorf("%w: oidc.NewProvider: %w", errProviderUnavailable, err)
	}

	o.verifier = provider.Verifier(&oidc.Config{
		ClientID:          o.audience,
		SkipClientIDCheck: o.audience == "",
	})
	return o.verifier, nil
}
