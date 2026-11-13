package model

import "time"

// Main — проблема: одинаковые ошибки, склеенные по отпечатку
type Main struct {
	Id           int64
	CreatedAt    time.Time
	ModifiedAt   time.Time
	ProjectId    int64
	Title        string
	Culprit      string
	Type         string
	Value        string
	Level        string
	Platform     string
	Status       string
	FirstSeen    time.Time
	LastSeen     time.Time
	TimesSeen    int64
	FirstRelease string
	LastRelease  string
}

const StatusUnresolved = "unresolved"

// Seen — когда проблему видели: лёгкая строка для сортировки списка, без текстов
type Seen struct {
	Id        int64
	FirstSeen time.Time
	LastSeen  time.Time
}

// Hit — все события пачки с одним отпечатком в одном проекте
type Hit struct {
	ProjectId   int64
	Hash        string
	HashVersion string

	// данные для заведения новой проблемы — по первому событию отпечатка в пачке
	Title    string
	Culprit  string
	Type     string
	Value    string
	Level    string
	Platform string

	Count        int64
	FirstSeen    time.Time
	LastSeen     time.Time
	FirstRelease string // релиз самого раннего события
	LastRelease  string // релиз самого позднего события
}

// HashKey — отпечаток в проекте
type HashKey struct {
	ProjectId int64
	Hash      string
}

// Bump — прибавка к счётчикам проблемы
type Bump struct {
	IssueId     int64
	Count       int64
	FirstSeen   time.Time
	LastSeen    time.Time
	LastRelease string
}

// HitResult — к какой проблеме относится отпечаток и сколько полных записей можно сохранить
type HitResult struct {
	IssueId      int64
	StoreAllowed int64
}
