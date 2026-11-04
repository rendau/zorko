package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/rendau/zorko/internal/domain/artifact/model"
)

const bundleColumnsB = `b.id, b.created_at, b.org_id, b.checksum, b.source, b.state, b.detail, b.release, b.dist,
	b.file_count, b.size, b.assembled_at, b.last_used_at`

const bundleListWhere = ` from artifact_bundle b
	join artifact_bundle_project bp on bp.bundle_id = b.id and bp.project_id = $1
	where ($2 = '' or b.release ilike '%' || $2 || '%' or b.checksum = $2
		or exists (select 1 from artifact_file f where f.bundle_id = b.id and f.debug_id::text = $2))`

// ListBundles — архивы проекта, свежие первыми; total — всего под условие
func (r *Repo) ListBundles(ctx context.Context, pars *model.BundleListReq) ([]*model.Bundle, int64, error) {
	q := strings.ToLower(strings.TrimSpace(pars.Query))
	var total int64
	if err := r.Con.QueryRow(ctx, `select count(*)`+bundleListWhere, pars.ProjectId, q).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count artifact_bundle: %w", err)
	}
	rows, err := r.Con.Query(ctx, `select `+bundleColumnsB+bundleListWhere+` order by b.created_at desc, b.id desc limit $3 offset $4`,
		pars.ProjectId, q, pars.PageSize, pars.Page*pars.PageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("select artifact_bundle: %w", err)
	}
	defer rows.Close()
	result := make([]*model.Bundle, 0)
	for rows.Next() {
		b, err := scanBundle(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan artifact_bundle: %w", err)
		}
		result = append(result, b)
	}
	return result, total, rows.Err()
}

// BundleProjectIds — проекты архива
func (r *Repo) BundleProjectIds(ctx context.Context, bundleId int64) ([]int64, error) {
	var ids []int64
	if err := r.Con.QueryRow(ctx, `select coalesce(array_agg(project_id order by project_id), '{}')
		from artifact_bundle_project where bundle_id = $1`, bundleId).Scan(&ids); err != nil {
		return nil, fmt.Errorf("select artifact_bundle_project: %w", err)
	}
	return ids, nil
}

func (r *Repo) DeleteBundle(ctx context.Context, id int64) error {
	if _, err := r.Con.Exec(ctx, `delete from artifact_bundle where id = $1`, id); err != nil {
		return fmt.Errorf("delete artifact_bundle: %w", err)
	}
	return nil
}
