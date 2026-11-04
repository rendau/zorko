package service

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Разбор повторяет sentry-cli 3.8.2 (org_auth_token.rs): префикс, payload — base64 с padding,
// секрет — base64 без padding ровно на 32 байта, сегментов ровно три
func TestTokenFormatLikeSentryCli(t *testing.T) {
	raw, err := generateToken("https://zorko-ingest.mdev.kz", "mechta", time.Unix(1793778572, 0))
	require.NoError(t, err)

	require.True(t, strings.HasPrefix(raw, "sntrys_"))
	parts := strings.Split(raw, "_")
	require.Len(t, parts, 3)

	payloadBytes, err := base64.StdEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(payloadBytes, &payload))
	assert.Equal(t, "mechta", payload["org"])
	assert.Equal(t, "https://zorko-ingest.mdev.kz", payload["url"])
	assert.Equal(t, "https://zorko-ingest.mdev.kz", payload["region_url"])
	assert.InDelta(t, 1793778572, payload["iat"], 0)

	secret, err := base64.RawStdEncoding.DecodeString(parts[2])
	require.NoError(t, err)
	assert.Len(t, secret, 32)

	org, ok := parseToken(raw)
	require.True(t, ok)
	assert.Equal(t, "mechta", org)
	assert.Len(t, hashToken(raw), 32)
	assert.Equal(t, raw[len(raw)-4:], tokenHint(raw))
}

func TestParseTokenRejects(t *testing.T) {
	good, err := generateToken("http://localhost:8000", "o", time.Now())
	require.NoError(t, err)
	parts := strings.Split(good, "_")

	for name, raw := range map[string]string{
		"empty":            "",
		"user token":       "sntryu_0123456789abcdef",
		"no secret":        "sntrys_" + parts[1],
		"extra segment":    good + "_x",
		"short secret":     "sntrys_" + parts[1] + "_" + base64.RawStdEncoding.EncodeToString([]byte("short")),
		"payload not b64":  "sntrys_!!!_" + parts[2],
		"payload not json": "sntrys_" + base64.StdEncoding.EncodeToString([]byte("x")) + "_" + parts[2],
	} {
		_, ok := parseToken(raw)
		assert.False(t, ok, name)
	}
}
