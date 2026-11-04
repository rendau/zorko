package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/rendau/zorko/internal/domain/artifact/model"
)

const siteFetchColumns = `project_id, url, state, detail, bundle_id, fetched_at`

func scanSiteFetch(row pgx.Row) (*model.SiteFetch, error) {
	m := &model.SiteFetch{}
	err := row.Scan(&m.ProjectId, &m.Url, &m.State, &m.Detail, &m.BundleId, &m.FetchedAt)
	return m, err
}

func (r *Repo) GetSiteFetch(ctx context.Context, projectId int64, url string) (*model.SiteFetch, bool, error) {
	m, err := scanSiteFetch(r.Con.QueryRow(ctx, `select `+siteFetchColumns+` from site_fetch where project_id = $1 and url = $2`, projectId, url))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("select site_fetch: %w", err)
	}
	return m, true, nil
}

func (r *Repo) SaveSiteFetch(ctx context.Context, m *model.SiteFetch) error {
	_, err := r.Con.Exec(ctx, `insert into site_fetch (project_id, url, state, detail, bundle_id, fetched_at)
		values ($1, $2, $3, $4, $5, now())
		on conflict (project_id, url) do update set state = excluded.state, detail = excluded.detail,
			bundle_id = excluded.bundle_id, fetched_at = excluded.fetched_at`,
		m.ProjectId, m.Url, m.State, m.Detail, m.BundleId)
	if err != nil {
		return fmt.Errorf("upsert site_fetch: %w", err)
	}
	return nil
}

// ListSiteFetch — последние записи проекта (state пусто — любые)
func (r *Repo) ListSiteFetch(ctx context.Context, projectId int64, state string, limit int) ([]*model.SiteFetch, error) {
	rows, err := r.Con.Query(ctx, `select `+siteFetchColumns+` from site_fetch
		where project_id = $1 and ($2 = '' or state = $2) order by fetched_at desc, url limit $3`, projectId, state, limit)
	if err != nil {
		return nil, fmt.Errorf("select site_fetch: %w", err)
	}
	defer rows.Close()
	result := make([]*model.SiteFetch, 0)
	for rows.Next() {
		m, err := scanSiteFetch(rows)
		if err != nil {
			return nil, fmt.Errorf("scan site_fetch: %w", err)
		}
		result = append(result, m)
	}
	return result, rows.Err()
}
