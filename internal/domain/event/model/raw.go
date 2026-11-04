package model

import "time"

// RawEvent — событие SDK как пришло: тело не разобрано, известно только то, что видит приём
type RawEvent struct {
	EventId    string
	ProjectId  int64
	ReceivedAt time.Time
	ClientIp   string
	Payload    []byte
}
