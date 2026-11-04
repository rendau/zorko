//go:build integration

package service_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rendau/zorko/internal/domain/org_token/model"
	"github.com/rendau/zorko/internal/domain/org_token/repo/db"
	"github.com/rendau/zorko/internal/domain/org_token/service"
	"github.com/rendau/zorko/internal/errs"
	"github.com/rendau/zorko/internal/testutil"
)

func TestCreateAuthDelete(t *testing.T) {
	ctx := context.Background()
	pool := testutil.PgPool(t)
	projectId := testutil.NewProject(t, pool)
	var orgId int64
	require.NoError(t, pool.QueryRow(ctx, `select org_id from project where id = $1`, projectId).Scan(&orgId))

	svc := service.New(db.New(pool))
	created, err := svc.Create(ctx, orgId, "o", nil, "gitlab ci")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(created.Token, "sntrys_"))
	assert.Equal(t, created.Token[len(created.Token)-4:], created.Hint)
	assert.Equal(t, "gitlab ci", created.Name)
	assert.Nil(t, created.LastUsedAt)

	var stored []byte
	require.NoError(t, pool.QueryRow(ctx, `select token_hash from org_token where id = $1`, created.Id).Scan(&stored))
	assert.NotContains(t, string(stored), created.Token, "в базе только отпечаток")

	got, err := svc.Auth(ctx, created.Token)
	require.NoError(t, err)
	assert.Equal(t, orgId, got.OrgId)

	items, err := svc.List(ctx, &model.ListReq{OrgId: orgId})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.NotNil(t, items[0].LastUsedAt)

	// чужой, но правильный по виду токен
	other, err := svc.Create(ctx, orgId, "o", nil, "other")
	require.NoError(t, err)
	_, err = svc.Auth(ctx, other.Token[:len(other.Token)-4]+"AAAA")
	assert.ErrorIs(t, err, errs.TokenInvalid)

	require.NoError(t, svc.Delete(ctx, created.Id))
	_, err = svc.Auth(ctx, created.Token)
	assert.ErrorIs(t, err, errs.TokenInvalid, "удалённый токен перестаёт работать сразу")
}
