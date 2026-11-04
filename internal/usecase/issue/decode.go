package issue

import (
	eventModel "github.com/rendau/zorko/internal/domain/event/model"
	searchqueryModel "github.com/rendau/zorko/internal/service/searchquery/model"
)

func decodeCond(c searchqueryModel.Cond, _ int) eventModel.TagCond {
	return eventModel.TagCond{Key: c.Key, Op: string(c.Op), Value: c.Value}
}
