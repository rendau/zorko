// Package db — замок уборки записей на все копии (Postgres)
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repo struct {
	con *pgxpool.Pool
}

func New(con *pgxpool.Pool) *Repo {
	return &Repo{con: con}
}

// TryLock — pg_try_advisory_lock на отдельном соединении; ok=false — замок у другой копии
func (r *Repo) TryLock(ctx context.Context, key int64) (func(), bool, error) {
	conn, err := r.con.Acquire(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("acquire: %w", err)
	}
	var ok bool
	if err = conn.QueryRow(ctx, `select pg_try_advisory_lock($1)`, key).Scan(&ok); err != nil {
		conn.Release()
		return nil, false, fmt.Errorf("pg_try_advisory_lock: %w", err)
	}
	if !ok {
		conn.Release()
		return nil, false, nil
	}
	return func() {
		_, _ = conn.Exec(context.WithoutCancel(ctx), `select pg_advisory_unlock($1)`, key)
		conn.Release()
	}, true, nil
}
