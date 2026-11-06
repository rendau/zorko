package ingest

import (
	"bytes"
	"encoding/json"
	"errors"

	replayModel "github.com/rendau/zorko/internal/domain/replay/model"
	usecase "github.com/rendau/zorko/internal/usecase/ingest"
)

// Запись сессии от SDK (@sentry/replay 11.x): конверт из двух частей —
//
//	replay_event     — JSON-описание сегмента (replay_id, segment_id, replay_type, replay_start_timestamp…)
//	replay_recording — строка {"segment_id":N}, "\n", затем тело: zlib (первый байт 0x78) или JSON-массив rrweb

var errReplayInvalid = errors.New("replay is invalid")

type replayEventRep struct {
	ReplayId             string   `json:"replay_id"`
	SegmentId            *int     `json:"segment_id"`
	ReplayType           string   `json:"replay_type"`
	ReplayStartTimestamp *float64 `json:"replay_start_timestamp"`
	Timestamp            *float64 `json:"timestamp"`
}

type recordingHeaderRep struct {
	SegmentId *int `json:"segment_id"`
}

// parseReplay — разбор пары частей; проверка формы, а не правил приёма (они — в usecase)
func parseReplay(event, recording []byte) (*usecase.RawReplay, error) {
	var ev replayEventRep
	if err := json.Unmarshal(event, &ev); err != nil {
		return nil, errReplayInvalid
	}

	replayId := replayModel.NormalizeId(ev.ReplayId)
	if replayId == "" || ev.SegmentId == nil || *ev.SegmentId < 0 || *ev.SegmentId > replayModel.MaxSegmentId ||
		ev.ReplayStartTimestamp == nil || ev.Timestamp == nil {
		return nil, errReplayInvalid
	}
	if ev.ReplayType != replayModel.TypeBuffer && ev.ReplayType != replayModel.TypeSession {
		return nil, errReplayInvalid
	}

	headerLine, body, ok := bytes.Cut(recording, []byte{'\n'})
	if !ok || len(body) == 0 {
		return nil, errReplayInvalid
	}
	var header recordingHeaderRep
	if err := json.Unmarshal(headerLine, &header); err != nil || header.SegmentId == nil || *header.SegmentId != *ev.SegmentId {
		return nil, errReplayInvalid
	}

	// zlib: первый байт 0x78 (deflate, окно 32 КБ); иначе — несжатый JSON-массив событий rrweb
	compressed := body[0] == 0x78
	if !compressed && (body[0] != '[' || !json.Valid(body)) {
		return nil, errReplayInvalid
	}

	return &usecase.RawReplay{
		ReplayId:   replayId,
		SegmentId:  *ev.SegmentId,
		Type:       ev.ReplayType,
		Start:      replayModel.SecondsToTime(*ev.ReplayStartTimestamp),
		Timestamp:  replayModel.SecondsToTime(*ev.Timestamp),
		Event:      event,
		Body:       body,
		Compressed: compressed,
	}, nil
}
