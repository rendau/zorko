package ingest

import (
	"strconv"
	"time"

	eventModel "github.com/rendau/zorko/internal/domain/event/model"
)

// Req — один запрос SDK: ключ, откуда пришёл и события из него (возможно, ни одного)
type Req struct {
	ProjectId  int64
	PublicKey  string
	OriginHost string // хост из Origin/Referer; пусто — запрос не из браузера
	ClientIp   string
	Events     []*eventModel.RawEvent
}

// RateLimitedError — превышен предел ключа; SDK должен повторить не раньше RetryAfter
type RateLimitedError struct {
	RetryAfter time.Duration
}

func (e RateLimitedError) Error() string {
	return "rate limited, retry after " + strconv.Itoa(int(e.RetryAfter.Seconds())) + "s"
}
