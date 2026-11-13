package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	commonRepoPg "github.com/rendau/zorko/internal/domain/common/repo/pg"
	"github.com/rendau/zorko/internal/domain/oauth/model"
)

type Repo struct {
	*commonRepoPg.Base
}

func New(con *pgxpool.Pool) *Repo {
	return &Repo{Base: commonRepoPg.NewBase(con)}
}

const grantColumns = `id, usr_id, client_id, client_name, redirect_uri, code_challenge, created_at, connected_at, last_used_at`

func scanGrant(row pgx.Row) (*model.Grant, error) {
	m := &model.Grant{}
	err := row.Scan(&m.Id, &m.UsrId, &m.ClientId, &m.ClientName, &m.RedirectUri, &m.CodeChallenge,
		&m.CreatedAt, &m.ConnectedAt, &m.LastUsedAt)
	return m, err
}

// ListGrants — состоявшиеся подключения человека (код уже обменян на токены), старые первыми
func (r *Repo) ListGrants(ctx context.Context, usrId int64) ([]*model.Grant, error) {
	rows, err := r.Con.Query(ctx, `select `+grantColumns+` from oauth_grant
		where usr_id = $1 and connected_at is not null order by created_at, id`, usrId)
	if err != nil {
		return nil, fmt.Errorf("select oauth_grant: %w", err)
	}
	defer rows.Close()

	result := make([]*model.Grant, 0)
	for rows.Next() {
		m, err := scanGrant(rows)
		if err != nil {
			return nil, fmt.Errorf("scan oauth_grant: %w", err)
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

func (r *Repo) GetGrant(ctx context.Context, id string) (*model.Grant, bool, error) {
	m, err := scanGrant(r.Con.QueryRow(ctx, `select `+grantColumns+` from oauth_grant where id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("select oauth_grant: %w", err)
	}
	return m, true, nil
}

// CreateGrant заводит подключение вместе с его кодом авторизации: либо оба, либо ничего
func (r *Repo) CreateGrant(ctx context.Context, obj *model.Grant, code *model.Token) (string, error) {
	tx, err := r.Con.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id string
	err = tx.QueryRow(ctx, `insert into oauth_grant (usr_id, client_id, client_name, redirect_uri, code_challenge)
		values ($1, $2, $3, $4, $5) returning id`,
		obj.UsrId, obj.ClientId, obj.ClientName, obj.RedirectUri, obj.CodeChallenge).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("insert oauth_grant: %w", err)
	}

	code.GrantId = id
	if err = insertTokens(ctx, tx, []*model.Token{code}); err != nil {
		return "", err
	}

	if err = tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}
	return id, nil
}

func (r *Repo) DeleteGrant(ctx context.Context, id string) error {
	if _, err := r.Con.Exec(ctx, `delete from oauth_grant where id = $1`, id); err != nil {
		return fmt.Errorf("delete oauth_grant: %w", err)
	}
	return nil
}

func (r *Repo) TouchGrantLastUsed(ctx context.Context, id string) error {
	if _, err := r.Con.Exec(ctx, `update oauth_grant set last_used_at = now() where id = $1`, id); err != nil {
		return fmt.Errorf("update oauth_grant: %w", err)
	}
	return nil
}

// GetToken находит токен по хешу, в том числе истёкший
func (r *Repo) GetToken(ctx context.Context, hash string) (*model.Token, bool, error) {
	m := &model.Token{Hash: hash}
	err := r.Con.QueryRow(ctx, `select grant_id, kind, expires_at from oauth_token where token_hash = $1`, hash).
		Scan(&m.GrantId, &m.Kind, &m.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("select oauth_token: %w", err)
	}
	return m, true, nil
}

// ExchangeToken гасит одноразовый токен (код или refresh) и в той же транзакции выдаёт новые и
// отмечает подключение состоявшимся. false — токена уже нет (использован или проигран второму
// такому же запросу): два одновременных обмена не могут оба выиграть, новых токенов тогда нет.
func (r *Repo) ExchangeToken(ctx context.Context, hash, kind string, tokens []*model.Token) (bool, error) {
	tx, err := r.Con.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var grantId string
	err = tx.QueryRow(ctx, `delete from oauth_token where token_hash = $1 and kind = $2 and expires_at > now()
		returning grant_id`, hash, kind).Scan(&grantId)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("delete oauth_token: %w", err)
	}

	if _, err = tx.Exec(ctx, `update oauth_grant set connected_at = coalesce(connected_at, now()) where id = $1`, grantId); err != nil {
		return false, fmt.Errorf("update oauth_grant: %w", err)
	}

	for _, t := range tokens {
		t.GrantId = grantId
	}
	if err = insertTokens(ctx, tx, tokens); err != nil {
		return false, err
	}

	if err = tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}
	return true, nil
}

func (r *Repo) DeleteExpiredTokens(ctx context.Context) (int64, error) {
	tag, err := r.Con.Exec(ctx, `delete from oauth_token where expires_at < now()`)
	if err != nil {
		return 0, fmt.Errorf("delete oauth_token: %w", err)
	}
	return tag.RowsAffected(), nil
}

// DeleteIdleGrants удаляет подключения без токенов, созданные раньше createdBefore (брошенное
// согласие или истёкшее подключение); возвращает их число
func (r *Repo) DeleteIdleGrants(ctx context.Context, createdBefore time.Time) (int64, error) {
	tag, err := r.Con.Exec(ctx, `delete from oauth_grant g
		where g.created_at < $1
		  and not exists (select 1 from oauth_token t where t.grant_id = g.id)`, createdBefore)
	if err != nil {
		return 0, fmt.Errorf("delete oauth_grant: %w", err)
	}
	return tag.RowsAffected(), nil
}

func insertTokens(ctx context.Context, tx pgx.Tx, tokens []*model.Token) error {
	for _, t := range tokens {
		_, err := tx.Exec(ctx, `insert into oauth_token (token_hash, grant_id, kind, expires_at) values ($1, $2, $3, $4)`,
			t.Hash, t.GrantId, t.Kind, t.ExpiresAt)
		if err != nil {
			return fmt.Errorf("insert oauth_token: %w", err)
		}
	}
	return nil
}
