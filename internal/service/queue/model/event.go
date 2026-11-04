package model

import "time"

// Event — событие SDK в очереди: сырое тело и то, что о нём знает приём
type Event struct {
	EventId    string
	ProjectId  int64
	ReceivedAt time.Time
	ClientIp   string
	Payload    []byte
}
