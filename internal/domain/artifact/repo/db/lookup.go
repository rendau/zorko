package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/rendau/zorko/internal/domain/artifact/model"
)

const fileColumns = `f.id, f.bundle_id, f.url, f.type, coalesce(f.debug_id::text, ''), f.sourcemap, f.blob, f.size`

func scanFiles(rows pgx.Rows) ([]*model.File, error) {
	defer rows.Close()
	result := make([]*model.File, 0)
	for rows.Next() {
		f := &model.File{}
		if err := rows.Scan(&f.Id, &f.BundleId, &f.Url, &f.Type, &f.DebugId, &f.Sourcemap, &f.Blob, &f.Size); err != nil {
			return nil, fmt.Errorf("scan artifact_file: %w", err)
		}
		result = append(result, f)
	}
	return result, rows.Err()
}

// readyBundleOfProject — готовые архивы проекта, свежие первыми
const readyBundleOfProject = ` from artifact_file f
	join artifact_bundle b on b.id = f.bundle_id and b.state = 'ok'
	join artifact_bundle_project bp on bp.bundle_id = b.id and bp.project_id = $1`

// FilesByDebugIds — файлы с этими метками в архивах проекта
func (r *Repo) FilesByDebugIds(ctx context.Context, projectId int64, debugIds []string) ([]*model.File, error) {
	if len(debugIds) == 0 {
		return nil, nil
	}
	rows, err := r.Con.Query(ctx, `select `+fileColumns+readyBundleOfProject+`
		where f.debug_id = any($2::uuid[]) order by b.created_at desc, f.id`, projectId, debugIds)
	if err != nil {
		return nil, fmt.Errorf("select artifact_file by debug_id: %w", err)
	}
	return scanFiles(rows)
}

// FilesByReleaseUrls — файлы с этими путями в архивах проекта для релиза и dist (старый способ)
func (r *Repo) FilesByReleaseUrls(ctx context.Context, projectId int64, release, dist string, urls []string) ([]*model.File, error) {
	if len(urls) == 0 || release == "" {
		return nil, nil
	}
	rows, err := r.Con.Query(ctx, `select `+fileColumns+readyBundleOfProject+`
		where b.release = $2 and b.dist = $3 and f.url = any($4) order by b.created_at desc, f.id`,
		projectId, release, dist, urls)
	if err != nil {
		return nil, fmt.Errorf("select artifact_file by url: %w", err)
	}
	return scanFiles(rows)
}

// FilesOfBundle — файлы архива; urls пусто — все
func (r *Repo) FilesOfBundle(ctx context.Context, bundleId int64, urls []string) ([]*model.File, error) {
	q := `select ` + fileColumns + ` from artifact_file f where f.bundle_id = $1`
	args := []any{bundleId}
	if len(urls) > 0 {
		q += ` and f.url = any($2)`
		args = append(args, urls)
	}
	rows, err := r.Con.Query(ctx, q+` order by f.url, f.id`, args...)
	if err != nil {
		return nil, fmt.Errorf("select artifact_file of bundle: %w", err)
	}
	return scanFiles(rows)
}

// MarkUsed — отметка «пригодился», не чаще раза в час на архив
func (r *Repo) MarkUsed(ctx context.Context, bundleIds []int64) error {
	if len(bundleIds) == 0 {
		return nil
	}
	_, err := r.Con.Exec(ctx, `update artifact_bundle set last_used_at = now()
		where id = any($1) and (last_used_at is null or last_used_at < now() - interval '1 hour')`, bundleIds)
	if err != nil {
		return fmt.Errorf("update artifact_bundle.last_used_at: %w", err)
	}
	return nil
}
