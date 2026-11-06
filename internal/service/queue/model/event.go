package model

import "time"

// Event — событие SDK в очереди: сырое тело и то, что о нём знает приём.
// В теме записей сессий (ADR-012) — описание сегмента (replay_event), EventId — номер записи
// (сегменты одной записи попадают в один раздел), BodySize — размер тела сегмента в S3.
type Event struct {
	EventId    string
	ProjectId  int64
	ReceivedAt time.Time
	ClientIp   string
	Payload    []byte
	BodySize   int64
}
