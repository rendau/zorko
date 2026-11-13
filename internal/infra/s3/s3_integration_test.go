//go:build integration

package s3_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/rendau/zorko/internal/testutil"
)

func TestClient(t *testing.T) {
	ctx := context.Background()
	c := testutil.S3(t)
	key := "test/" + uuid.NewString()

	ok, err := c.Exists(ctx, key)
	require.NoError(t, err)
	assert.False(t, ok)

	_, found, err := c.Get(ctx, key, 1024)
	require.NoError(t, err)
	assert.False(t, found)

	require.NoError(t, c.Put(ctx, key, []byte("hello")))
	data, found, err := c.Get(ctx, key, 1024)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "hello", string(data))

	_, _, err = c.Get(ctx, key, 3)
	require.Error(t, err, "предел размера")

	keys, err := c.ListOlder(ctx, "test/", time.Now().Add(time.Minute))
	require.NoError(t, err)
	assert.Contains(t, keys, key)

	require.NoError(t, c.Delete(ctx, key))
	ok, err = c.Exists(ctx, key)
	require.NoError(t, err)
	assert.False(t, ok)
}

// TestDeletePrefixStand — удаление идёт одновременно с листингом: на настоящем S3 страницы листинга
// (их тут три) не должны терять ключи из-за уже удалённых
func TestDeletePrefixStand(t *testing.T) {
	ctx := context.Background()
	c := testutil.S3(t)
	prefix := "test/" + uuid.NewString() + "/"
	const total = 2300

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(32)
	for i := range total {
		g.Go(func() error { return c.Put(gCtx, prefix+strconv.Itoa(i), []byte("x")) })
	}
	require.NoError(t, g.Wait())
	other := "test/" + uuid.NewString()
	require.NoError(t, c.Put(ctx, other, []byte("x")))
	t.Cleanup(func() { _ = c.Delete(context.Background(), other) })

	n, err := c.DeletePrefix(ctx, prefix)
	require.NoError(t, err)
	assert.Equal(t, total, n)

	keys, err := c.ListOlder(ctx, prefix, time.Now().Add(time.Minute))
	require.NoError(t, err)
	assert.Empty(t, keys)

	ok, err := c.Exists(ctx, other)
	require.NoError(t, err)
	assert.True(t, ok, "чужой ключ не тронут")

	n, err = c.DeletePrefix(ctx, prefix)
	require.NoError(t, err)
	assert.Zero(t, n, "пустой префикс")
}
