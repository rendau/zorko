package searchquery

import (
	searchqueryModel "github.com/rendau/zorko/internal/service/searchquery/model"
)

// Parser — строка поиска как в Sentry (`browser.name:Chrome !level:info url:*cart* корзина`) → условия
type Parser interface {
	Parse(q string) (*searchqueryModel.Query, error)
}
