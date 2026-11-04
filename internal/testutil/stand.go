// Package testutil — подключения к тестовому стенду (scripts/stand.sh up) для интеграционных тестов
package testutil

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/rendau/zorko/internal/infra/chmigrate"
)

// адреса стенда — CLAUDE.md, «Тестовый стенд»
const (
	PgDsn        = "postgres://zorko:zorko@localhost:54330/zorko?sslmode=disable"
	ChDsn        = "clickhouse://zorko:zorko@localhost:19000/zorko"
	KafkaBrokers = "localhost:19092"
)

// RepoRoot — корень репозитория (каталог с go.mod)
func RepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err = os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, parent, dir, "go.mod not found")
		dir = parent
	}
}

// PgPool — пул к Postgres стенда с применёнными миграциями
func PgPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	m, err := migrate.New("file://"+filepath.Join(RepoRoot(t), "migrations"), PgDsn)
	require.NoError(t, err)
	if err = m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		require.NoError(t, err)
	}
	_, _ = m.Close()

	pool, err := pgxpool.New(context.Background(), PgDsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

// ChConn — соединение с ClickHouse стенда с применёнными миграциями
func ChConn(t *testing.T) driver.Conn {
	t.Helper()

	opts, err := clickhouse.ParseDSN(ChDsn)
	require.NoError(t, err)
	opts.DialTimeout = 2 * time.Second
	conn, err := clickhouse.Open(opts)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	chmigrate.Run(context.Background(), conn, filepath.Join(RepoRoot(t), "migrations-ch"))
	return conn
}

// NewProject заводит организацию и проект с уникальным slug и возвращает id проекта
func NewProject(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	ctx := context.Background()

	var orgId, projectId int64
	require.NoError(t, pool.QueryRow(ctx, `insert into org (slug) values ($1) returning id`, "t-"+uuid.NewString()[:8]).Scan(&orgId))
	require.NoError(t, pool.QueryRow(ctx, `insert into project (org_id, slug) values ($1, 'p') returning id`, orgId).Scan(&projectId))
	return projectId
}
