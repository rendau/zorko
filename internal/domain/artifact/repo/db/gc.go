package db

import (
	"context"
	"fmt"
	"time"
)

// TryLock — замок уборки на все копии (pg_try_advisory_lock на отдельном соединении); ok=false — занят
func (r *Repo) TryLock(ctx context.Context, key int64) (func(), bool, error) {
	conn, err := r.Con.Acquire(ctx)
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

// TouchBlob — файл снова нужен: уборка не тронет его ещё сутки (вызывается до записи в S3)
func (r *Repo) TouchBlob(ctx context.Context, sha1 string, size int64) error {
	_, err := r.Con.Exec(ctx, `insert into blob (sha1, size, created_at) values ($1, $2, now())
		on conflict (sha1) do update set created_at = now()`, sha1, size)
	if err != nil {
		return fmt.Errorf("upsert blob: %w", err)
	}
	return nil
}

// DeleteStaleBundles — архивы, которые не пригождались дольше retention, и сборки в error/assembling
// старше суток; пачками по limit, возвращает сколько удалено
func (r *Repo) DeleteStaleBundles(ctx context.Context, retention time.Duration, limit int) (int64, error) {
	var total int64
	for {
		tag, err := r.Con.Exec(ctx, `delete from artifact_bundle where id in (
			select id from artifact_bundle
			where (state = 'ok' and coalesce(last_used_at, created_at) < now() - make_interval(secs => $1))
			   or (state <> 'ok' and created_at < now() - interval '1 day')
			order by id limit $2)`, retention.Seconds(), limit)
		if err != nil {
			return total, fmt.Errorf("delete artifact_bundle: %w", err)
		}
		total += tag.RowsAffected()
		if tag.RowsAffected() < int64(limit) {
			return total, nil
		}
	}
}

// OrphanBlobs — файлы без ссылок, не тронутые дольше olderThan
func (r *Repo) OrphanBlobs(ctx context.Context, olderThan time.Duration, limit int) ([]string, error) {
	rows, err := r.Con.Query(ctx, `select b.sha1 from blob b
		where b.created_at < now() - make_interval(secs => $1)
		  and not exists (select 1 from artifact_file f where f.blob = b.sha1)
		order by b.created_at limit $2`, olderThan.Seconds(), limit)
	if err != nil {
		return nil, fmt.Errorf("select blob: %w", err)
	}
	defer rows.Close()
	result := make([]string, 0)
	for rows.Next() {
		var s string
		if err = rows.Scan(&s); err != nil {
			return nil, fmt.Errorf("scan blob: %w", err)
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

// DeleteBlobs — строки файлов, если они всё ещё без ссылок и не тронуты; возвращает удалённые
func (r *Repo) DeleteBlobs(ctx context.Context, sha1s []string, olderThan time.Duration) ([]string, error) {
	rows, err := r.Con.Query(ctx, `delete from blob b where b.sha1 = any($1)
		and b.created_at < now() - make_interval(secs => $2)
		and not exists (select 1 from artifact_file f where f.blob = b.sha1)
		returning b.sha1`, sha1s, olderThan.Seconds())
	if err != nil {
		return nil, fmt.Errorf("delete blob: %w", err)
	}
	defer rows.Close()
	result := make([]string, 0, len(sha1s))
	for rows.Next() {
		var s string
		if err = rows.Scan(&s); err != nil {
			return nil, fmt.Errorf("scan blob: %w", err)
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

func (r *Repo) DeleteOldSiteFetch(ctx context.Context, olderThan time.Duration) (int64, error) {
	tag, err := r.Con.Exec(ctx, `delete from site_fetch where fetched_at < now() - make_interval(secs => $1)`, olderThan.Seconds())
	if err != nil {
		return 0, fmt.Errorf("delete site_fetch: %w", err)
	}
	return tag.RowsAffected(), nil
}
