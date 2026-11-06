// Package model — записи сессий (ADR-012..014): сегменты, связи с проблемами, сводки для интерфейса
package model

import (
	"math"
	"strconv"
	"strings"
	"time"

	commonModel "github.com/rendau/zorko/internal/domain/common/model"
)

// виды записи по replay_type SDK
const (
	TypeBuffer  = "buffer"  // запись началась из-за ошибки: SDK держал последнюю минуту в памяти
	TypeSession = "session" // сессия попала в долю replaysSessionSampleRate
)

// MaxSegmentId — сегмент раз в ~5 с, запись не длиннее часа (~720 сегментов) — с запасом
const MaxSegmentId = 1000

// Segment — описание одного сегмента записи; тело — в S3 по SegmentKey(ReceivedAt, …)
type Segment struct {
	ProjectId  int64
	ReplayId   string // 32 hex
	SegmentId  int
	Type       string
	Start      time.Time // начало записи (replay_start_timestamp, часы устройства)
	Timestamp  time.Time
	ReceivedAt time.Time // приём (часы сервера) — по нему день в S3 и TTL
	Urls       []string
	ErrorIds   []string // 32 hex

	Environment string
	Release     string
	UserId      string
	UserEmail   string
	UserIp      string
	Browser     string
	Os          string
	Device      string
	SdkName     string
	SdkVersion  string

	Size int64 // байт тела в S3 (zlib)
}

// Link — ошибка с тегом replayId: запись ↔ проблема (ADR-014)
type Link struct {
	ProjectId int64
	IssueId   int64
	ReplayId  string
	EventId   string
	Timestamp time.Time
}

// Summary — запись целиком (по всем сегментам)
type Summary struct {
	ProjectId  int64
	ReplayId   string
	Type       string
	Start      time.Time
	End        time.Time // время последнего сегмента
	Segments   int       // число сегментов (номера 0..Segments-1, если ни один не потерялся)
	MaxSegment int
	Url        string // первый адрес записи
	ErrorCount int

	Environment string
	Release     string
	UserId      string
	UserEmail   string
	UserIp      string
	Browser     string
	Os          string
	Device      string
	SdkName     string
	SdkVersion  string

	// ErrorAt — время ошибки проблемы в этой записи (только в выборке записей проблемы)
	ErrorAt *time.Time
}

// ListReq — выборка записей
type ListReq struct {
	commonModel.ListParams

	ProjectIds  []int64 // обязателен: записи только видимых проектов
	Environment *string
	Release     *string
	Query       *string // подстрока адреса страницы, id или почта пользователя
	HasErrors   *bool
	Type        *string
}

// SegmentKey — ключ тела сегмента в S3: replay/<день приёма>/<проект>/<запись>/<сегмент>.
// День — по часам сервера: начало записи (часы устройства) бывает неточным, а SDK в режиме
// «при ошибке» передвигает его, пока копит минуту в памяти.
func SegmentKey(receivedAt time.Time, projectId int64, replayId string, segmentId int) string {
	return DayPrefix(receivedAt) + strconv.FormatInt(projectId, 10) + "/" + replayId + "/" + strconv.Itoa(segmentId)
}

// KeyPrefix — все записи в S3
const KeyPrefix = "replay/"

// DayPrefix — папка дня приёма: уборка удаляет её целиком
func DayPrefix(receivedAt time.Time) string {
	return KeyPrefix + receivedAt.UTC().Format("20060102") + "/"
}

// SecondsToTime — секунды Unix с дробной частью (как шлёт SDK) с точностью до миллисекунды; мусор — нулевое время
func SecondsToTime(v float64) time.Time {
	if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 || v > 1e11 {
		return time.Time{}
	}
	return time.UnixMilli(int64(math.Round(v * 1000))).UTC()
}

// NormalizeId — номер записи или события SDK (32 hex, uuid с дефисами тоже) в нижнем регистре; иначе ""
func NormalizeId(v string) string {
	v = strings.ToLower(strings.ReplaceAll(v, "-", ""))
	if len(v) != 32 || strings.Trim(v, "0123456789abcdef") != "" {
		return ""
	}
	return v
}
