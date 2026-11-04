//go:build integration

package s3_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
