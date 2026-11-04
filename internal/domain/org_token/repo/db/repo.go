package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	commonRepoPg "github.com/rendau/zorko/internal/domain/common/repo/pg"
	"github.com/rendau/zorko/internal/domain/org_token/model"
	"github.com/rendau/zorko/internal/errs"
)

type Repo struct {
	*commonRepoPg.Base
}

func New(con *pgxpool.Pool) *Repo {
	return &Repo{Base: commonRepoPg.NewBase(con)}
}

const selectColumns = `t.id, t.created_at, t.org_id, t.created_by, coalesce(nullif(u.name, ''), u.username, ''),
	t.name, t.hint, t.last_used_at`

const selectFrom = ` from org_token t left join usr u on u.id = t.created_by`

func scan(row pgx.Row) (*model.Main, error) {
	m := &model.Main{}
	err := row.Scan(&m.Id, &m.CreatedAt, &m.OrgId, &m.CreatedBy, &m.CreatedByName, &m.Name, &m.Hint, &m.LastUsedAt)
	return m, err
}

// List — токены организации, новые первыми
func (r *Repo) List(ctx context.Context, pars *model.ListReq) ([]*model.Main, error) {
	rows, err := r.Con.Query(ctx, `select `+selectColumns+selectFrom+` where t.org_id = $1 order by t.created_at desc, t.id desc`, pars.OrgId)
	if err != nil {
		return nil, fmt.Errorf("select org_token: %w", err)
	}
	defer rows.Close()

	result := make([]*model.Main, 0)
	for rows.Next() {
		m, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("scan org_token: %w", err)
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

func (r *Repo) Get(ctx context.Context, id int64) (*model.Main, bool, error) {
	m, err := scan(r.Con.QueryRow(ctx, `select `+selectColumns+selectFrom+` where t.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("select org_token: %w", err)
	}
	return m, true, nil
}

func (r *Repo) GetByHash(ctx context.Context, hash []byte) (*model.Main, bool, error) {
	m, err := scan(r.Con.QueryRow(ctx, `select `+selectColumns+selectFrom+` where t.token_hash = $1`, hash))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("select org_token by hash: %w", err)
	}
	return m, true, nil
}

func (r *Repo) Create(ctx context.Context, orgId int64, createdBy *int64, name, hint string, hash []byte) (int64, error) {
	var id int64
	err := r.Con.QueryRow(ctx, `insert into org_token (org_id, created_by, name, hint, token_hash)
		values ($1, $2, $3, $4, $5) returning id`, orgId, createdBy, name, hint, hash).Scan(&id)
	if err != nil {
		if commonRepoPg.IsForeignKeyViolation(err, "org_token_org_id_fkey") {
			return 0, errs.OrgNotFound
		}
		return 0, fmt.Errorf("insert org_token: %w", err)
	}
	return id, nil
}

func (r *Repo) Delete(ctx context.Context, id int64) error {
	if _, err := r.Con.Exec(ctx, `delete from org_token where id = $1`, id); err != nil {
		return fmt.Errorf("delete org_token: %w", err)
	}
	return nil
}

// TouchUsed — отметка «пользовались», не чаще раза в минуту
func (r *Repo) TouchUsed(ctx context.Context, id int64) error {
	_, err := r.Con.Exec(ctx, `update org_token set last_used_at = now()
		where id = $1 and (last_used_at is null or last_used_at < now() - interval '1 minute')`, id)
	if err != nil {
		return fmt.Errorf("update org_token.last_used_at: %w", err)
	}
	return nil
}
