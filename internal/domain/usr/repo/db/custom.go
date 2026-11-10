package db

import (
	"context"
	"fmt"

	"github.com/rendau/mobone/v2"

	"github.com/rendau/zorko/internal/domain/usr/model"
	repoModel "github.com/rendau/zorko/internal/domain/usr/repo/db/model"
)

var allowedSortFields = map[string]string{
	"id":         "id",
	"created_at": "created_at",
	"username":   "username",
	"name":       "name",
}

func (r *Repo) getConditions(pars *model.ListReq) (map[string]any, map[string][]any) {
	conditions := make(map[string]any, 2)
	conditionExps := make(map[string][]any)

	if pars == nil {
		return conditions, conditionExps
	}

	if pars.Active != nil {
		conditions["active"] = *pars.Active
	}
	if pars.IDs != nil {
		conditions["id"] = *pars.IDs
	}

	return conditions, conditionExps
}

func (r *Repo) GetByUsername(ctx context.Context, username string) (*model.Main, bool, error) {
	return r.getOne(ctx, map[string]any{"username": username})
}

// GetByExtSub — человек по id у внешнего провайдера входа (ADR-019)
func (r *Repo) GetByExtSub(ctx context.Context, extSub string) (*model.Main, bool, error) {
	return r.getOne(ctx, map[string]any{"ext_sub": extSub})
}

func (r *Repo) getOne(ctx context.Context, conditions map[string]any) (*model.Main, bool, error) {
	items := make([]*repoModel.Select, 0, 1)
	_, err := r.ModelStore.List(ctx, mobone.ListParams{
		Conditions: conditions,
		PageSize:   1,
	}, func(add bool) mobone.ListModelI {
		item := &repoModel.Select{}
		if add {
			items = append(items, item)
		}
		return item
	})
	if err != nil {
		return nil, false, fmt.Errorf("ModelStore.List: %w", err)
	}
	if len(items) == 0 {
		return nil, false, nil
	}
	return repoModel.EncodeSelect(items[0], 0), true, nil
}

func (r *Repo) HasAny(ctx context.Context) (bool, error) {
	var exists bool
	if err := r.Con.QueryRow(ctx, `select exists(select 1 from usr)`).Scan(&exists); err != nil {
		return false, fmt.Errorf("select exists usr: %w", err)
	}
	return exists, nil
}
