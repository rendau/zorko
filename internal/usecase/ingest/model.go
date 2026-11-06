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
	Replay     *RawReplay // сегмент записи сессии, если пришёл в этом запросе
}

// RawReplay — сегмент записи, разобранный по форме (handler); правила приёма — в Usecase
type RawReplay struct {
	ReplayId   string // 32 hex
	SegmentId  int
	Type       string // replayModel.TypeBuffer | TypeSession
	Start      time.Time
	Timestamp  time.Time
	Event      []byte // replay_event как пришёл: разберёт обработчик описаний
	Body       []byte // тело без строки-заголовка
	Compressed bool   // zlib; иначе — JSON
}

// Rep — что ответить SDK сверх кода
type Rep struct {
	ReplayResult string        // исход сегмента записи (ReplayResult*); пусто — записи в запросе не было
	ReplayStop   time.Duration // > 0 — остановить запись в браузере (X-Sentry-Rate-Limits, категория replay)
}

// исходы сегмента записи: метка метрики replay_segment_total
const (
	ReplayResultAccepted       = "accepted"
	ReplayResultDisabled       = "disabled"         // режим off или S3 не настроен
	ReplayResultTypeNotAllowed = "type_not_allowed" // запись доли сессий, а в проекте — только при ошибке
	ReplayResultHourLimit      = "hour_limit"
	ReplayResultInvalid        = "invalid"
	ReplayResultTooLarge       = "too_large"
	ReplayResultUnavailable    = "storage_unavailable"
)

// ReplayStopMode — на сколько остановить запись, если проекту записи не нужны
const ReplayStopMode = time.Hour

// RateLimitedError — превышен предел ключа; SDK должен повторить не раньше RetryAfter
type RateLimitedError struct {
	RetryAfter time.Duration
}

func (e RateLimitedError) Error() string {
	return "rate limited, retry after " + strconv.Itoa(int(e.RetryAfter.Seconds())) + "s"
}
