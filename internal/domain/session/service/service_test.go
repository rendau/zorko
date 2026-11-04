package service

import (
	"testing"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccessToken(t *testing.T) {
	s := New("secret-1")

	token, err := s.CreateToken(42, true)
	require.NoError(t, err)

	session, err := s.FromToken(token)
	require.NoError(t, err)
	assert.Equal(t, int64(42), session.Id)
	assert.True(t, session.IsAdmin())

	_, err = New("other-secret").FromToken(token)
	assert.Error(t, err, "чужая подпись")

	_, err = s.FromToken(token + "x")
	assert.Error(t, err, "испорченный токен")
}

func TestRefreshTokenIsNotAccess(t *testing.T) {
	s := New("secret-1")
	refresh, err := s.CreateRefreshToken(42, "hash-1")
	require.NoError(t, err)

	_, err = s.FromToken(refresh)
	assert.Error(t, err, "refresh-токеном нельзя ходить в API")

	access, _ := s.CreateToken(42, false)
	_, err = s.RefreshTokenUserId(access)
	assert.Error(t, err, "access-токен не обменивается на новую пару")

	id, err := s.RefreshTokenUserId(refresh)
	require.NoError(t, err)
	assert.Equal(t, int64(42), id)

	assert.NoError(t, s.CheckRefreshToken(refresh, "hash-1"))
	assert.Error(t, s.CheckRefreshToken(refresh, "hash-2"), "пароль сменили — refresh отозван")
}

func TestExpiredAndForeignAlg(t *testing.T) {
	s := New("secret-1")

	expired, err := s.signClaims(jwtv5.MapClaims{"typ": "access", "id": 1, "is_admin": true, "exp": time.Now().Add(-time.Minute).Unix()})
	require.NoError(t, err)
	_, err = s.FromToken(expired)
	assert.Error(t, err)

	noExp, err := s.signClaims(jwtv5.MapClaims{"typ": "access", "id": 1, "is_admin": true})
	require.NoError(t, err)
	_, err = s.FromToken(noExp)
	assert.Error(t, err, "токен без срока не принимается")

	none, err := jwtv5.NewWithClaims(jwtv5.SigningMethodNone, jwtv5.MapClaims{"typ": "access", "id": 1, "is_admin": true,
		"exp": time.Now().Add(time.Hour).Unix()}).SignedString(jwtv5.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)
	_, err = s.FromToken(none)
	assert.Error(t, err, "alg=none не принимается")
}

func TestEmptySecret(t *testing.T) {
	_, err := New("").CreateToken(1, true)
	assert.Error(t, err)
}
