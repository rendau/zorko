//go:build integration

package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	artifactModel "github.com/rendau/zorko/internal/domain/artifact/model"
	domainArtifactRepoBlobP "github.com/rendau/zorko/internal/domain/artifact/repo/blob"
	domainArtifactRepoDbP "github.com/rendau/zorko/internal/domain/artifact/repo/db"
	domainArtifactServiceP "github.com/rendau/zorko/internal/domain/artifact/service"
	"github.com/rendau/zorko/internal/service/artifactgc/service"
	"github.com/rendau/zorko/internal/testutil"
)

func TestRunOnce(t *testing.T) {
	ctx := context.Background()
	pool := testutil.PgPool(t)
	s3 := testutil.S3(t)
	projectId := testutil.NewProject(t, pool)
	var orgId int64
	require.NoError(t, pool.QueryRow(ctx, `select org_id from project where id = $1`, projectId).Scan(&orgId))

	art := domainArtifactServiceP.New(domainArtifactRepoDbP.New(pool), domainArtifactRepoBlobP.New(s3))
	newBundle := func(content string) (int64, string) {
		blob, err := art.StoreFile(ctx, []byte(content))
		require.NoError(t, err)
		b, claimed, err := art.ClaimBundle(ctx, &artifactModel.NewBundle{OrgId: orgId, Checksum: domainArtifactServiceP.Sha1Hex([]byte(content)),
			Source: artifactModel.SourceUpload, ProjectIds: []int64{projectId}}, time.Minute, time.Minute)
		require.NoError(t, err)
		require.True(t, claimed)
		require.NoError(t, art.CompleteBundle(ctx, b.Id, "", "", []*artifactModel.File{{Url: "~/a.js.map", Type: artifactModel.FileTypeSourceMap, Blob: blob, Size: 1}}))
		return b.Id, blob
	}
	staleId, staleBlob := newBundle("stale " + uuid.NewString())
	usedId, _ := newBundle("used " + uuid.NewString())
	_, err := pool.Exec(ctx, `update artifact_bundle set created_at = now() - interval '40 days' where id = any($1)`, []int64{staleId, usedId})
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `update artifact_bundle set last_used_at = now() - interval '2 days' where id = $1`, usedId)
	require.NoError(t, err)

	gc := service.New(service.Config{}, art)
	require.NoError(t, gc.RunOnce(ctx))

	exists := func(id int64) bool {
		var n int
		require.NoError(t, pool.QueryRow(ctx, `select count(*) from artifact_bundle where id = $1`, id).Scan(&n))
		return n == 1
	}
	assert.False(t, exists(staleId), "40 дней не пригождался — удалён")
	assert.True(t, exists(usedId), "пригодился 2 дня назад — остаётся")

	ok, err := s3.Exists(ctx, "file/"+staleBlob)
	require.NoError(t, err)
	assert.True(t, ok, "файл без ссылок, но тронут недавно — сутки не удаляется")

	_, err = pool.Exec(ctx, `update blob set created_at = now() - interval '2 days' where sha1 = $1`, staleBlob)
	require.NoError(t, err)
	require.NoError(t, gc.RunOnce(ctx))
	ok, err = s3.Exists(ctx, "file/"+staleBlob)
	require.NoError(t, err)
	assert.False(t, ok, "файл без ссылок удалён из S3")
	var n int
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from blob where sha1 = $1`, staleBlob).Scan(&n))
	assert.Zero(t, n)

	// замок: вторая копия пропускает
	unlock, locked, err := art.GcLock(ctx)
	require.NoError(t, err)
	require.True(t, locked)
	require.NoError(t, gc.RunOnce(ctx))
	unlock()
}
