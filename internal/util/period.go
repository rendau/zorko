package util

import (
	"time"

	"github.com/rendau/zorko/internal/errs"
)

// periods — периоды списка и графиков; события хранятся 30 дней, глубже смотреть нечего
var periods = map[string]time.Duration{
	"1h":  time.Hour,
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
	"14d": 14 * 24 * time.Hour,
	"30d": 30 * 24 * time.Hour,
}

// ParsePeriod — длительность периода; пусто — def
func ParsePeriod(v string, def time.Duration) (time.Duration, error) {
	if v == "" {
		return def, nil
	}
	d, ok := periods[v]
	if !ok {
		return 0, errs.ErrFull{Err: errs.InvalidRequest, Desc: "period: 1h, 24h, 7d, 14d, 30d", Fields: map[string]string{"period": "invalid"}}
	}
	return d, nil
}
