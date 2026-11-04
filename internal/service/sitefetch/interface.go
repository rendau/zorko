package sitefetch

import (
	"context"

	"github.com/rendau/zorko/internal/service/sitefetch/model"
)

// SiteFetcher — карты с сайта, если из CI их нет (ADR-010): только по настройкам проекта, с памятью
// результата по адресу и защитой от походов во внутренние адреса
type SiteFetcher interface {
	// Fetch — nil без ошибки: скачивание в проекте выключено
	Fetch(ctx context.Context, projectId int64, absUrl string) (*model.Result, error)
}
