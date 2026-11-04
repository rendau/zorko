package model

import (
	"strconv"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	queueModel "github.com/rendau/zorko/internal/service/queue/model"
)

// заголовки записи: метаданные приёма; тело записи — событие SDK без перекодирования
const (
	headerProjectId  = "project_id"
	headerReceivedAt = "received_at"
	headerClientIp   = "client_ip"
)

func EncodeEvent(v *queueModel.Event, topic string) *kgo.Record {
	return &kgo.Record{
		Topic: topic,
		Key:   []byte(v.EventId),
		Value: v.Payload,
		Headers: []kgo.RecordHeader{
			{Key: headerProjectId, Value: []byte(strconv.FormatInt(v.ProjectId, 10))},
			{Key: headerReceivedAt, Value: []byte(v.ReceivedAt.UTC().Format(time.RFC3339Nano))},
			{Key: headerClientIp, Value: []byte(v.ClientIp)},
		},
	}
}

// DecodeEvent — запись без нужных заголовков даёт событие с ProjectId = 0: его отбросит обработка
func DecodeEvent(v *kgo.Record, _ int) *queueModel.Event {
	result := &queueModel.Event{
		EventId: string(v.Key),
		Payload: v.Value,
	}
	for _, h := range v.Headers {
		switch h.Key {
		case headerProjectId:
			result.ProjectId, _ = strconv.ParseInt(string(h.Value), 10, 64)
		case headerReceivedAt:
			result.ReceivedAt, _ = time.Parse(time.RFC3339Nano, string(h.Value))
		case headerClientIp:
			result.ClientIp = string(h.Value)
		}
	}
	if result.ReceivedAt.IsZero() {
		result.ReceivedAt = v.Timestamp
	}
	return result
}
