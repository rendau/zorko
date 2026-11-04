package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// runChMigrations применяет migrations-ch/*.up.sql по возрастанию номера.
// Свой раннер, а не golang-migrate: его драйвер clickhouse построен на clickhouse-go v1, который
// регистрирует database/sql-драйвер под тем же именем, что и clickhouse-go v2 (паника при старте).
// Блокировки между копиями нет — миграции пишутся идемпотентно (if not exists).
func runChMigrations(ctx context.Context, conn driver.Conn) {
	absPath, _ := filepath.Abs("./migrations-ch")

	files, _ := filepath.Glob(filepath.Join(absPath, "*.up.sql"))
	if len(files) == 0 {
		slog.Warn("CH-migrations folder has no migrations, nothing to apply", "path", absPath)
		return
	}
	slices.Sort(files)

	err := conn.Exec(ctx, `
		create table if not exists schema_migrations (
			version    UInt64,
			applied_at DateTime64(3) default now64(3)
		) engine = MergeTree order by version`)
	if err != nil {
		panic(fmt.Errorf("CH-migration: create schema_migrations: %w", err))
	}

	applied := map[uint64]bool{}
	rows, err := conn.Query(ctx, `select version from schema_migrations`)
	if err != nil {
		panic(fmt.Errorf("CH-migration: select versions: %w", err))
	}
	for rows.Next() {
		var v uint64
		if err = rows.Scan(&v); err != nil {
			panic(fmt.Errorf("CH-migration: scan version: %w", err))
		}
		applied[v] = true
	}
	rows.Close()

	for _, file := range files {
		name := filepath.Base(file)

		version, err := strconv.ParseUint(strings.SplitN(name, "_", 2)[0], 10, 64)
		if err != nil {
			panic(fmt.Errorf("CH-migration: bad file name %q: %w", name, err))
		}
		if applied[version] {
			continue
		}

		body, err := os.ReadFile(file)
		if err != nil {
			panic(fmt.Errorf("CH-migration: read %q: %w", name, err))
		}

		for _, stmt := range strings.Split(string(body), ";") {
			if strings.TrimSpace(stripSqlComments(stmt)) == "" {
				continue
			}
			if err = conn.Exec(ctx, stmt); err != nil {
				panic(fmt.Errorf("CH-migration %q: %w", name, err))
			}
		}

		if err = conn.Exec(ctx, `insert into schema_migrations (version) values (?)`, version); err != nil {
			panic(fmt.Errorf("CH-migration: save version %d: %w", version, err))
		}

		slog.Info("CH-migration applied", "file", name)
	}
}

func stripSqlComments(stmt string) string {
	lines := strings.Split(stmt, "\n")
	lines = slices.DeleteFunc(lines, func(l string) bool {
		return strings.HasPrefix(strings.TrimSpace(l), "--")
	})
	return strings.Join(lines, "\n")
}
