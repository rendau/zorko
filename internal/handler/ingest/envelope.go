package ingest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// Формат envelope (develop.sentry.dev/sdk/data-model/envelopes):
//
//	Envelope = Headers { "\n" Item } [ "\n" ]
//	Item     = Headers "\n" Payload
//
// У элемента с length payload — ровно length байт, без length — до конца строки.

type envelopeHeader struct {
	EventId string `json:"event_id"`
	Dsn     string `json:"dsn"`
}

type envelopeItemHeader struct {
	Type   string `json:"type"`
	Length *int   `json:"length"`
}

type envelopeItem struct {
	Type    string
	Payload []byte
}

type envelope struct {
	Header envelopeHeader
	Items  []envelopeItem
}

var errBadEnvelope = errors.New("bad envelope")

func parseEnvelope(body []byte) (*envelope, error) {
	line, rest := cutLine(body)

	result := &envelope{}
	if err := json.Unmarshal(line, &result.Header); err != nil {
		return nil, fmt.Errorf("%w: header: %w", errBadEnvelope, err)
	}

	for len(rest) > 0 {
		line, rest = cutLine(rest)
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		var itemHeader envelopeItemHeader
		if err := json.Unmarshal(line, &itemHeader); err != nil {
			return nil, fmt.Errorf("%w: item header: %w", errBadEnvelope, err)
		}

		var payload []byte
		if itemHeader.Length != nil {
			n := *itemHeader.Length
			if n < 0 || n > len(rest) {
				return nil, fmt.Errorf("%w: item length %d exceeds body", errBadEnvelope, n)
			}
			payload, rest = rest[:n], rest[n:]
			if len(rest) > 0 && rest[0] == '\n' {
				rest = rest[1:]
			}
		} else {
			payload, rest = cutLine(rest)
		}

		result.Items = append(result.Items, envelopeItem{Type: itemHeader.Type, Payload: payload})
	}

	return result, nil
}

// cutLine — строка до "\n" (без него) и остаток после
func cutLine(b []byte) ([]byte, []byte) {
	line, rest, _ := bytes.Cut(b, []byte{'\n'})
	return line, rest
}
