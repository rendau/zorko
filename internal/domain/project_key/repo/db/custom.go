package db

import (
	"github.com/rendau/zorko/internal/domain/project_key/model"
)

var allowedSortFields = map[string]string{
	"id":         "id",
	"created_at": "created_at",
	"label":      "label",
}

func (r *Repo) getConditions(pars *model.ListReq) (map[string]any, map[string][]any) {
	conditions := make(map[string]any, 3)
	conditionExps := make(map[string][]any)

	if pars == nil {
		return conditions, conditionExps
	}

	if pars.ProjectId != nil {
		conditions["project_id"] = *pars.ProjectId
	}
	if pars.Active != nil {
		conditions["active"] = *pars.Active
	}
	if pars.IDs != nil {
		conditions["id"] = *pars.IDs
	}

	return conditions, conditionExps
}
