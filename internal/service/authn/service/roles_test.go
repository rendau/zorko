package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAdminRule(t *testing.T) {
	idMdev := map[string]any{ // id.mdev.kz, «Один на все API»: роли по разделам
		"resource_access": map[string]any{
			"zorko":  map[string]any{"roles": []any{"admin"}},
			"orders": map[string]any{"roles": []any{"manager"}},
		},
	}
	keycloakRealm := map[string]any{"realm_access": map[string]any{"roles": []any{"zorko-admin", "offline_access"}}}
	flat := map[string]any{"roles": []any{"admin"}}
	groups := map[string]any{"groups": []any{"devs", "zorko-admins"}}
	zitadel := map[string]any{"urn:zitadel:iam:org:project:roles": map[string]any{"admin": map[string]any{"1": "org"}}}
	auth0 := map[string]any{"https://mechta.kz/roles": []any{"admin"}}
	spaced := map[string]any{"role": "viewer admin"}

	cases := []struct {
		name   string
		role   string
		path   string
		claims map[string]any
		want   bool
	}{
		{"клиент:роль — у этого клиента", "zorko:admin", "", idMdev, true},
		{"клиент:роль — роль другого клиента не считается", "orders:admin", "", idMdev, false},
		{"просто роль — у любого клиента", "manager", "", idMdev, true},
		{"роль realm Keycloak", "zorko-admin", "", keycloakRealm, true},
		{"плоский roles", "admin", "", flat, true},
		{"клиент:роль не ищется в плоском roles", "zorko:admin", "", flat, false},
		{"строка через пробел", "admin", "", spaced, true},
		{"путь: groups", "zorko-admins", "groups", groups, true},
		{"путь: без пути groups не смотрим", "zorko-admins", "", groups, false},
		{"путь: роли — ключи объекта (Zitadel)", "admin", "urn:zitadel:iam:org:project:roles", zitadel, true},
		{"путь с точками целиком (Auth0)", "admin", "https://mechta.kz/roles", auth0, true},
		{"путь: вложенный", "admin", "resource_access.zorko.roles", idMdev, true},
		{"путь: роль как есть, без разбора клиента", "zorko:admin", "resource_access.zorko.roles", idMdev, false},
		{"роль не задана — админов нет", "", "", flat, false},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, newAdminRule(c.role, c.path).matches(c.claims), c.name)
	}
}

func TestIdentityFromClaims(t *testing.T) {
	rule := newAdminRule("zorko:admin", "")

	id, ok := identityFromClaims(map[string]any{
		"sub": "f3a1", "preferred_username": "Oleg", "email": "oleg@mechta.kz", "name": "Олег Ким",
		"resource_access": map[string]any{"zorko": map[string]any{"roles": []any{"admin"}}},
	}, "", rule)
	assert.True(t, ok)
	assert.Equal(t, identity{sub: "f3a1", username: "oleg", name: "Олег Ким", admin: true}, id)

	id, _ = identityFromClaims(map[string]any{"sub": "f3a1", "email": "Oleg@Mechta.kz", "given_name": "Олег", "family_name": "Ким"}, "", rule)
	assert.Equal(t, "oleg@mechta.kz", id.username, "нет preferred_username — почта")
	assert.Equal(t, "Олег Ким", id.name)
	assert.False(t, id.admin)

	id, _ = identityFromClaims(map[string]any{"sub": "f3a1", "preferred_username": "oleg", "upn": "o.kim"}, "upn", rule)
	assert.Equal(t, "o.kim", id.username, "AUTH_USERNAME_CLAIM")

	id, _ = identityFromClaims(map[string]any{"sub": "f3a1"}, "", rule)
	assert.Equal(t, "f3a1", id.username, "без логина — sub")

	_, ok = identityFromClaims(map[string]any{"preferred_username": "oleg"}, "", rule)
	assert.False(t, ok, "без sub человека не узнать")
}
