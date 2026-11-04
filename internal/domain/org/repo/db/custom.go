package db

import (
	"github.com/rendau/zorko/internal/domain/org/model"
)

var allowedSortFields = map[string]string{
	"id":         "id",
	"created_at": "created_at",
	"slug":       "slug",
	"name":       "name",
}

func (r *Repo) getConditions(pars *model.ListReq) (map[string]any, map[string][]any) {
	conditions := make(map[string]any, 2)
	conditionExps := make(map[string][]any)

	if pars == nil {
		return conditions, conditionExps
	}

	if pars.Slug != nil {
		conditions["slug"] = *pars.Slug
	}
	if pars.IDs != nil {
		conditions["id"] = *pars.IDs
	}

	return conditions, conditionExps
}
