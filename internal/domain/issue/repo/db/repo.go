package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/samber/lo"

	commonRepoPg "github.com/rendau/zorko/internal/domain/common/repo/pg"
	"github.com/rendau/zorko/internal/domain/issue/model"
	"github.com/rendau/zorko/internal/errs"
)

type Repo struct {
	*commonRepoPg.Base
}

func New(con *pgxpool.Pool) *Repo {
	return &Repo{Base: commonRepoPg.NewBase(con)}
}

// ResolveHashes — проблемы по отпечаткам одним запросом; ненайденных в ответе нет
func (r *Repo) ResolveHashes(ctx context.Context, keys []model.HashKey) (map[model.HashKey]int64, error) {
	result := make(map[model.HashKey]int64, len(keys))
	if len(keys) == 0 {
		return result, nil
	}

	rows, err := r.Con.Query(ctx, `
		select h.project_id, h.hash, h.issue_id
		from issue_hash h
		join unnest($1::bigint[], $2::text[]) as k(project_id, hash)
		  on h.project_id = k.project_id and h.hash = k.hash`,
		lo.Map(keys, func(k model.HashKey, _ int) int64 { return k.ProjectId }),
		lo.Map(keys, func(k model.HashKey, _ int) string { return k.Hash }),
	)
	if err != nil {
		return nil, fmt.Errorf("query issue_hash: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var k model.HashKey
		var issueId int64
		if err = rows.Scan(&k.ProjectId, &k.Hash, &issueId); err != nil {
			return nil, fmt.Errorf("scan issue_hash: %w", err)
		}
		result[k] = issueId
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows issue_hash: %w", err)
	}

	return result, nil
}

// CreateWithHash заводит проблему и связь с отпечатком. Если другая копия обработки успела
// завести проблему с тем же отпечатком, своя откатывается и возвращается чужая.
// Удалённый проект — errs.ProjectNotFound.
func (r *Repo) CreateWithHash(ctx context.Context, hit *model.Hit) (int64, error) {
	tx, err := r.Con.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var issueId int64
	err = tx.QueryRow(ctx, `
		insert into issue (project_id, title, culprit, type, value, level, platform,
		                   first_seen, last_seen, first_release, last_release)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $8, $9, $9)
		returning id`,
		hit.ProjectId, hit.Title, hit.Culprit, hit.Type, hit.Value, hit.Level, hit.Platform,
		hit.FirstSeen, hit.FirstRelease,
	).Scan(&issueId)
	if err != nil {
		if commonRepoPg.IsForeignKeyViolation(err, "issue_project_id_fkey") {
			return 0, errs.ProjectNotFound
		}
		return 0, fmt.Errorf("insert issue: %w", err)
	}

	var linkedId int64
	err = tx.QueryRow(ctx, `
		insert into issue_hash (project_id, hash, version, issue_id)
		values ($1, $2, $3, $4)
		on conflict do nothing
		returning issue_id`,
		hit.ProjectId, hit.Hash, hit.HashVersion, issueId,
	).Scan(&linkedId)

	if errors.Is(err, pgx.ErrNoRows) {
		// гонка: отпечаток уже заведён другой копией — своя проблема откатывается
		_ = tx.Rollback(ctx)

		err = r.Con.QueryRow(ctx, `select issue_id from issue_hash where project_id = $1 and hash = $2`,
			hit.ProjectId, hit.Hash).Scan(&linkedId)
		if err != nil {
			return 0, fmt.Errorf("select raced issue_hash: %w", err)
		}
		return linkedId, nil
	}
	if err != nil {
		return 0, fmt.Errorf("insert issue_hash: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}

	return issueId, nil
}

// Bump одним запросом прибавляет события к проблемам и возвращает, сколько полных записей
// можно сохранить в минуту minute (не больше storeCap на проблему, ADR-003). Строки блокируются
// по возрастанию id — копии обработки не попадают во взаимную блокировку.
func (r *Repo) Bump(ctx context.Context, bumps []*model.Bump, storeCap int64, minute time.Time) (map[int64]int64, error) {
	result := make(map[int64]int64, len(bumps))
	if len(bumps) == 0 {
		return result, nil
	}

	args := make([]any, 0, 2+len(bumps)*5)
	args = append(args, minute, storeCap)
	values := make([]string, 0, len(bumps))
	for _, b := range bumps {
		n := len(args)
		values = append(values, fmt.Sprintf("($%d::bigint, $%d::bigint, $%d::timestamptz, $%d::timestamptz, $%d::text)",
			n+1, n+2, n+3, n+4, n+5))
		args = append(args, b.IssueId, b.Count, b.FirstSeen, b.LastSeen, b.LastRelease)
	}

	rows, err := r.Con.Query(ctx, `
		with v (id, cnt, first_seen, last_seen, last_release) as (values `+strings.Join(values, ", ")+`),
		old as (
			select i.id, i.stored_minute, i.stored_in_minute
			from issue i
			where i.id in (select id from v)
			order by i.id
			for update
		)
		update issue i
		set times_seen       = i.times_seen + v.cnt,
		    first_seen       = least(i.first_seen, v.first_seen),
		    last_seen        = greatest(i.last_seen, v.last_seen),
		    last_release     = case when v.last_seen >= i.last_seen and v.last_release <> ''
		                            then v.last_release else i.last_release end,
		    stored_in_minute = case when old.stored_minute = $1
		                            then least($2, old.stored_in_minute + v.cnt)
		                            else least($2, v.cnt) end,
		    stored_minute    = $1,
		    modified_at      = now()
		from v
		join old on old.id = v.id
		where i.id = v.id
		returning i.id,
		          i.stored_in_minute - case when old.stored_minute = $1 then old.stored_in_minute else 0 end`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("update issue: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var issueId, allowed int64
		if err = rows.Scan(&issueId, &allowed); err != nil {
			return nil, fmt.Errorf("scan issue: %w", err)
		}
		result[issueId] = max(allowed, 0)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows issue: %w", err)
	}

	return result, nil
}
