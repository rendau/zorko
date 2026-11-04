package model

import "time"

// StatFilter — какие события считать: проекты, период, окружение, условия на теги
type StatFilter struct {
	ProjectIds  []int64
	IssueIds    *[]int64 // nil — все проблемы
	From, To    time.Time
	Environment string
	Conds       []TagCond
}

// TagCond — условие на тег (Op: eq, ne, like, not_like; для like Value — шаблон LIKE)
type TagCond struct {
	Key   string
	Op    string
	Value string
}

// IssueAgg — события проблемы за период
type IssueAgg struct {
	IssueId  int64
	Events   int64
	Users    int64
	LastSeen time.Time
}

// TagDist — распределение значений тега
type TagDist struct {
	Key    string
	Total  int64
	Values []TagValueCount
}

type TagValueCount struct {
	Value string
	Count int64
}

// EventSummary — строка таблицы событий проблемы
type EventSummary struct {
	EventId     string
	Timestamp   time.Time
	Level       string
	Release     string
	Environment string
	Browser     string
	Os          string
	Device      string
	UserId      string
	UserEmail   string
	UserIp      string
	Url         string
	Title       string
	Message     string
}

// EventDetail — событие целиком: исходный JSON Sentry после вычистки и соседи по времени
type EventDetail struct {
	EventSummary
	Data        string
	PrevEventId string // раньше
	NextEventId string // позже
}
