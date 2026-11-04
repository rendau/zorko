package db

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/rendau/zorko/internal/domain/issue/model"
)

const issueColumns = `id, created_at, modified_at, project_id, title, culprit, type, value, level, platform, status,
	first_seen, last_seen, times_seen, first_release, last_release`

func scanIssue(row pgx.Row) (*model.Main, error) {
	m := &model.Main{}
	err := row.Scan(&m.Id, &m.CreatedAt, &m.ModifiedAt, &m.ProjectId, &m.Title, &m.Culprit, &m.Type, &m.Value,
		&m.Level, &m.Platform, &m.Status, &m.FirstSeen, &m.LastSeen, &m.TimesSeen, &m.FirstRelease, &m.LastRelease)
	return m, err
}

func (r *Repo) Get(ctx context.Context, id int64) (*model.Main, bool, error) {
	m, err := scanIssue(r.Con.QueryRow(ctx, `select `+issueColumns+` from issue where id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("select issue: %w", err)
	}
	return m, true, nil
}

// ListByIds — проблемы по id (порядок не гарантирован)
func (r *Repo) ListByIds(ctx context.Context, ids []int64) ([]*model.Main, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.Con.Query(ctx, `select `+issueColumns+` from issue where id = any($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("select issues: %w", err)
	}
	defer rows.Close()

	result := make([]*model.Main, 0, len(ids))
	for rows.Next() {
		m, err := scanIssue(rows)
		if err != nil {
			return nil, fmt.Errorf("scan issue: %w", err)
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

// IdsByText — проблемы проектов, у которых каждое слово есть в заголовке или месте падения
func (r *Repo) IdsByText(ctx context.Context, projectIds []int64, words []string, limit int) ([]int64, error) {
	if len(projectIds) == 0 || len(words) == 0 {
		return nil, nil
	}

	conds := make([]string, 0, len(words))
	args := []any{projectIds}
	for _, w := range words {
		args = append(args, "%"+escapeLike(w)+"%")
		n := strconv.Itoa(len(args))
		conds = append(conds, "(title ilike $"+n+" or culprit ilike $"+n+")")
	}
	args = append(args, limit)

	rows, err := r.Con.Query(ctx, `
		select id from issue
		where project_id = any($1) and `+strings.Join(conds, " and ")+`
		order by last_seen desc
		limit $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("select issue ids by text: %w", err)
	}
	defer rows.Close()

	result := make([]int64, 0, 64)
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan issue id: %w", err)
		}
		result = append(result, id)
	}
	return result, rows.Err()
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
