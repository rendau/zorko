package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Формат токена организации Sentry, который разбирает сам sentry-cli (src/utils/auth_token/org_auth_token.rs):
// sntrys_<base64 JSON {iat, url, region_url, org}>_<base64 без padding от 32 случайных байт>
const (
	tokenPrefix      = "sntrys_"
	tokenSecretBytes = 32
)

type tokenPayload struct {
	Iat       float64 `json:"iat"`
	Url       string  `json:"url"`
	RegionUrl string  `json:"region_url"`
	Org       string  `json:"org"`
}

// generateToken — новый токен для организации orgSlug с адресом сервера publicUrl
func generateToken(publicUrl, orgSlug string, now time.Time) (string, error) {
	payload, err := json.Marshal(tokenPayload{
		Iat:       float64(now.Unix()),
		Url:       publicUrl,
		RegionUrl: publicUrl,
		Org:       orgSlug,
	})
	if err != nil {
		return "", fmt.Errorf("json.Marshal: %w", err)
	}
	secret := make([]byte, tokenSecretBytes)
	if _, err = rand.Read(secret); err != nil {
		return "", fmt.Errorf("rand.Read: %w", err)
	}
	return tokenPrefix + base64.StdEncoding.EncodeToString(payload) + "_" + base64.RawStdEncoding.EncodeToString(secret), nil
}

// parseToken — проверка формата так же, как в sentry-cli; возвращает организацию из токена
func parseToken(raw string) (orgSlug string, ok bool) {
	if !strings.HasPrefix(raw, tokenPrefix) {
		return "", false
	}
	parts := strings.Split(strings.TrimPrefix(raw, tokenPrefix), "_")
	if len(parts) != 2 {
		return "", false
	}
	payloadBytes, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		return "", false
	}
	var payload tokenPayload
	if err = json.Unmarshal(payloadBytes, &payload); err != nil {
		return "", false
	}
	secret, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil || len(secret) != tokenSecretBytes {
		return "", false
	}
	return payload.Org, true
}

func hashToken(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

func tokenHint(raw string) string {
	if len(raw) < 4 {
		return raw
	}
	return raw[len(raw)-4:]
}
