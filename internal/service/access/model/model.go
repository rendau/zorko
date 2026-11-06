package model

import (
	"slices"

	"github.com/samber/lo"
)

// ProjectAccess — проекты, видимые человеку: All — все, иначе только Ids
type ProjectAccess struct {
	All bool
	Ids []int64
}

func (a *ProjectAccess) Allows(projectId int64) bool {
	return a.All || slices.Contains(a.Ids, projectId)
}

// Filter — видимые из ids (порядок сохраняется)
func (a *ProjectAccess) Filter(ids []int64) []int64 {
	if a.All {
		return ids
	}
	return lo.Filter(ids, func(id int64, _ int) bool { return slices.Contains(a.Ids, id) })
}

// AllowsAny — виден хотя бы один из ids
func (a *ProjectAccess) AllowsAny(ids []int64) bool {
	return a.All || lo.SomeBy(ids, func(id int64) bool { return slices.Contains(a.Ids, id) })
}
