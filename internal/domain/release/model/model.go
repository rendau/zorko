package model

import "time"

// Main — релиз (минимум для плагина; статусы, коммиты и деплои — шаг 4)
type Main struct {
	Id           int64
	CreatedAt    time.Time
	OrgId        int64
	Version      string
	DateReleased *time.Time
	ProjectIds   []int64
}
