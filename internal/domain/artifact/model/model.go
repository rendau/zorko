package model

import (
	"time"

	commonModel "github.com/rendau/zorko/internal/domain/common/model"
)

// Архив карт (artifact bundle): залитый из CI или скачанный с сайта (ADR-009, ADR-010)
const (
	SourceUpload = "upload"
	SourceSite   = "site"

	StateAssembling = "assembling"
	StateOk         = "ok"
	StateError      = "error"
)

// Виды файлов архива — как в manifest.json sentry-cli
const (
	FileTypeMinified  = "minified_source"
	FileTypeSourceMap = "source_map"
	FileTypeSource    = "source"
)

type Bundle struct {
	Id          int64
	CreatedAt   time.Time
	OrgId       int64
	Checksum    string
	Source      string
	State       string
	Detail      string
	Release     string
	Dist        string
	FileCount   int32
	Size        int64
	AssembledAt *time.Time
	LastUsedAt  *time.Time
}

type File struct {
	Id        int64
	BundleId  int64
	Url       string // ~/assets/index-BhK3.js
	Type      string
	DebugId   string // uuid в нижнем регистре; пусто — нет метки
	Sourcemap string // ссылка на карту (заголовок Sourcemap или sourceMappingURL)
	Blob      string // SHA-1 содержимого
	Size      int64
}

// NewBundle — заявка на сборку архива
type NewBundle struct {
	OrgId      int64
	Checksum   string
	Source     string
	Release    string
	Dist       string
	ProjectIds []int64
}

// BundleListReq — список архивов проекта
type BundleListReq struct {
	commonModel.ListParams

	ProjectId int64
	Query     string // подстрока релиза или метка целиком
}

type SiteFetch struct {
	ProjectId int64
	Url       string
	State     string // ok | error
	Detail    string
	BundleId  *int64
	FetchedAt time.Time
}
