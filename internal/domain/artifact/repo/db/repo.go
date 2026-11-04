package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rendau/zorko/internal/domain/artifact/model"
	commonRepoPg "github.com/rendau/zorko/internal/domain/common/repo/pg"
)

type Repo struct {
	*commonRepoPg.Base
}

func New(con *pgxpool.Pool) *Repo {
	return &Repo{Base: commonRepoPg.NewBase(con)}
}

const bundleColumns = `id, created_at, org_id, checksum, source, state, detail, release, dist, file_count, size,
	assembled_at, last_used_at`

func scanBundle(row pgx.Row) (*model.Bundle, error) {
	m := &model.Bundle{}
	err := row.Scan(&m.Id, &m.CreatedAt, &m.OrgId, &m.Checksum, &m.Source, &m.State, &m.Detail, &m.Release, &m.Dist,
		&m.FileCount, &m.Size, &m.AssembledAt, &m.LastUsedAt)
	return m, err
}

func scanBundleOpt(row pgx.Row) (*model.Bundle, bool, error) {
	m, err := scanBundle(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return m, true, nil
}

// ClaimBundle — заявка на сборку архива; claimed=true — сборку начинает вызывающий. Строка в состоянии
// assembling служит замком между копиями: занять можно новую строку, «зависшую» дольше staleAfter
// или упавшую раньше чем errorRetryAfter назад.
func (r *Repo) ClaimBundle(ctx context.Context, nb *model.NewBundle, staleAfter, errorRetryAfter time.Duration) (*model.Bundle, bool, error) {
	b, inserted, err := scanBundleOpt(r.Con.QueryRow(ctx, `insert into artifact_bundle (org_id, checksum, source, release, dist)
		values ($1, $2, $3, $4, $5) on conflict (org_id, checksum) do nothing returning `+bundleColumns,
		nb.OrgId, nb.Checksum, nb.Source, nb.Release, nb.Dist))
	if err != nil {
		return nil, false, fmt.Errorf("insert artifact_bundle: %w", err)
	}
	claimed := inserted
	if !inserted {
		b, claimed, err = scanBundleOpt(r.Con.QueryRow(ctx, `update artifact_bundle
			set state = 'assembling', detail = '', created_at = now(), assembled_at = null
			where org_id = $1 and checksum = $2 and (
				(state = 'assembling' and created_at < now() - make_interval(secs => $3)) or
				(state = 'error' and assembled_at < now() - make_interval(secs => $4)))
			returning `+bundleColumns, nb.OrgId, nb.Checksum, staleAfter.Seconds(), errorRetryAfter.Seconds()))
		if err != nil {
			return nil, false, fmt.Errorf("reclaim artifact_bundle: %w", err)
		}
		if !claimed {
			b, err = scanBundle(r.Con.QueryRow(ctx, `select `+bundleColumns+` from artifact_bundle
				where org_id = $1 and checksum = $2`, nb.OrgId, nb.Checksum))
			if err != nil {
				return nil, false, fmt.Errorf("select artifact_bundle: %w", err)
			}
		}
	}
	if err = r.AddBundleProjects(ctx, b.Id, nb.ProjectIds); err != nil {
		return nil, false, err
	}
	return b, claimed, nil
}

func (r *Repo) AddBundleProjects(ctx context.Context, bundleId int64, projectIds []int64) error {
	if len(projectIds) == 0 {
		return nil
	}
	_, err := r.Con.Exec(ctx, `insert into artifact_bundle_project (bundle_id, project_id)
		select $1, unnest($2::bigint[]) on conflict do nothing`, bundleId, projectIds)
	if err != nil {
		return fmt.Errorf("insert artifact_bundle_project: %w", err)
	}
	return nil
}

// CompleteBundle — файлы архива и состояние ok одной транзакцией
func (r *Repo) CompleteBundle(ctx context.Context, bundleId int64, release, dist string, files []*model.File) error {
	tx, err := r.Con.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	blobSha := make([]string, 0, len(files))
	blobSize := make([]int64, 0, len(files))
	seen := make(map[string]bool, len(files))
	var total int64
	for _, f := range files {
		total += f.Size
		if !seen[f.Blob] {
			seen[f.Blob] = true
			blobSha = append(blobSha, f.Blob)
			blobSize = append(blobSize, f.Size)
		}
	}
	if _, err = tx.Exec(ctx, `insert into blob (sha1, size) select unnest($1::text[]), unnest($2::bigint[])
		on conflict do nothing`, blobSha, blobSize); err != nil {
		return fmt.Errorf("insert blob: %w", err)
	}
	if _, err = tx.Exec(ctx, `delete from artifact_file where bundle_id = $1`, bundleId); err != nil {
		return fmt.Errorf("delete artifact_file: %w", err)
	}
	rows := make([][]any, 0, len(files))
	for _, f := range files {
		var debugId any
		if f.DebugId != "" {
			debugId = f.DebugId
		}
		rows = append(rows, []any{bundleId, f.Url, f.Type, debugId, f.Sourcemap, f.Blob, f.Size})
	}
	if _, err = tx.CopyFrom(ctx, pgx.Identifier{"artifact_file"},
		[]string{"bundle_id", "url", "type", "debug_id", "sourcemap", "blob", "size"}, pgx.CopyFromRows(rows)); err != nil {
		return fmt.Errorf("copy artifact_file: %w", err)
	}
	if _, err = tx.Exec(ctx, `update artifact_bundle set state = 'ok', detail = '', assembled_at = now(),
		file_count = $2, size = $3,
		release = case when release = '' then $4 else release end,
		dist = case when dist = '' then $5 else dist end
		where id = $1`, bundleId, len(files), total, release, dist); err != nil {
		return fmt.Errorf("update artifact_bundle: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (r *Repo) FailBundle(ctx context.Context, bundleId int64, detail string) error {
	_, err := r.Con.Exec(ctx, `update artifact_bundle set state = 'error', detail = $2, assembled_at = now() where id = $1`,
		bundleId, detail)
	if err != nil {
		return fmt.Errorf("update artifact_bundle: %w", err)
	}
	return nil
}

func (r *Repo) GetBundle(ctx context.Context, id int64) (*model.Bundle, bool, error) {
	b, found, err := scanBundleOpt(r.Con.QueryRow(ctx, `select `+bundleColumns+` from artifact_bundle where id = $1`, id))
	if err != nil {
		return nil, false, fmt.Errorf("select artifact_bundle: %w", err)
	}
	return b, found, nil
}

func (r *Repo) GetBundleByChecksum(ctx context.Context, orgId int64, checksum string) (*model.Bundle, bool, error) {
	b, found, err := scanBundleOpt(r.Con.QueryRow(ctx, `select `+bundleColumns+` from artifact_bundle
		where org_id = $1 and checksum = $2`, orgId, checksum))
	if err != nil {
		return nil, false, fmt.Errorf("select artifact_bundle: %w", err)
	}
	return b, found, nil
}
