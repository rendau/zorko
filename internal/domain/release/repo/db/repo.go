package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	commonRepoPg "github.com/rendau/zorko/internal/domain/common/repo/pg"
	"github.com/rendau/zorko/internal/domain/release/model"
)

type Repo struct {
	*commonRepoPg.Base
}

func New(con *pgxpool.Pool) *Repo {
	return &Repo{Base: commonRepoPg.NewBase(con)}
}

const selectSql = `select r.id, r.created_at, r.org_id, r.version, r.date_released,
	coalesce(array_agg(rp.project_id order by rp.project_id) filter (where rp.project_id is not null), '{}')
	from release r left join release_project rp on rp.release_id = r.id
	where r.org_id = $1 and r.version = $2
	group by r.id`

func (r *Repo) Get(ctx context.Context, orgId int64, version string) (*model.Main, bool, error) {
	m := &model.Main{}
	err := r.Con.QueryRow(ctx, selectSql, orgId, version).
		Scan(&m.Id, &m.CreatedAt, &m.OrgId, &m.Version, &m.DateReleased, &m.ProjectIds)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("select release: %w", err)
	}
	return m, true, nil
}

// Ensure — создать релиз или взять существующий; проекты дописываются
func (r *Repo) Ensure(ctx context.Context, orgId int64, version string, projectIds []int64, dateReleased *time.Time) (bool, error) {
	var id int64
	err := r.Con.QueryRow(ctx, `insert into release (org_id, version, date_released) values ($1, $2, $3)
		on conflict (org_id, version) do nothing returning id`, orgId, version, dateReleased).Scan(&id)
	created := err == nil
	if errors.Is(err, pgx.ErrNoRows) {
		err = r.Con.QueryRow(ctx, `select id from release where org_id = $1 and version = $2`, orgId, version).Scan(&id)
	}
	if err != nil {
		return false, fmt.Errorf("insert release: %w", err)
	}
	if !created && dateReleased != nil {
		if err = r.SetDateReleased(ctx, id, *dateReleased); err != nil {
			return false, err
		}
	}
	if len(projectIds) > 0 {
		if _, err = r.Con.Exec(ctx, `insert into release_project (release_id, project_id)
			select $1, unnest($2::bigint[]) on conflict do nothing`, id, projectIds); err != nil {
			return false, fmt.Errorf("insert release_project: %w", err)
		}
	}
	return created, nil
}

func (r *Repo) SetDateReleased(ctx context.Context, id int64, at time.Time) error {
	if _, err := r.Con.Exec(ctx, `update release set date_released = $2 where id = $1`, id, at); err != nil {
		return fmt.Errorf("update release: %w", err)
	}
	return nil
}
