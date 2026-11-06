package replaylimit

import "time"

// ReplayLimit — предел новых записей сессий в час на проект (ADR-013), в памяти копии приёма
type ReplayLimit interface {
	// Allow — можно ли начать новую запись; при отказе — сколько ждать до следующего часа
	Allow(projectId int64, limit int32, now time.Time) (bool, time.Duration)
}
