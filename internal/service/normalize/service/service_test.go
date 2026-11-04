package service

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventModel "github.com/rendau/zorko/internal/domain/event/model"
)

var receivedAt = time.Date(2026, 11, 4, 7, 49, 33, 0, time.UTC)

func normalize(t *testing.T, payload string) *eventModel.Event {
	t.Helper()
	ev, err := New().Normalize(&eventModel.RawEvent{
		EventId:    "ffffffffffffffffffffffffffffffff",
		ProjectId:  1,
		ReceivedAt: receivedAt,
		ClientIp:   "203.0.113.9",
		Payload:    []byte(payload),
	})
	require.NoError(t, err)
	return ev
}

func TestNodeSdkException(t *testing.T) {
	payload, err := os.ReadFile("testdata/node-exception.json")
	require.NoError(t, err)

	ev := normalize(t, string(payload))

	assert.Equal(t, "9172448878484ab3b8d683bbe4c8c0de", ev.EventId)
	assert.Equal(t, "error", ev.Level)
	assert.Equal(t, "node", ev.Platform)
	assert.Equal(t, "shop@1.4.2", ev.Release)
	assert.Equal(t, "production", ev.Environment)
	assert.Equal(t, "u-42", ev.User.Id)
	assert.Equal(t, "buyer@example.com", ev.User.Email)
	assert.Equal(t, "sentry.javascript.node", ev.SdkName)
	assert.True(t, strings.HasPrefix(ev.Runtime, "node v"))

	require.Len(t, ev.Exceptions, 1)
	exc := ev.Exceptions[0]
	assert.Equal(t, "TypeError", exc.Type)
	assert.Equal(t, "Cannot read properties of undefined (reading 'price')", exc.Value)
	assert.Equal(t, "generic", exc.Mechanism.Type, "механизм из SDK")
	assert.False(t, exc.Mechanism.Synthetic)
	assert.Equal(t, "yes", ev.Tags["handled"])
	require.NotEmpty(t, exc.Frames)
	top := exc.Frames[len(exc.Frames)-1]
	assert.Equal(t, "addItem", top.Function)
	require.NotNil(t, top.InApp)
	assert.True(t, *top.InApp)

	assert.Equal(t, "cart", ev.Tags["page"], "тег SDK")
	assert.Equal(t, "id:u-42", ev.Tags["user"])
	assert.Equal(t, "shop@1.4.2", ev.Tags["release"])
	assert.True(t, json.Valid(ev.Data))
}

func TestNodeSdkMessage(t *testing.T) {
	payload, err := os.ReadFile("testdata/node-message.json")
	require.NoError(t, err)

	ev := normalize(t, string(payload))
	assert.Equal(t, "warning", ev.Level)
	assert.Equal(t, "Order 81723 not found", ev.Message)
	require.Len(t, ev.Exceptions, 1, "SDK 11 прикладывает к сообщению стек как synthetic-исключение")
	assert.True(t, ev.Exceptions[0].Mechanism.Synthetic)
	assert.Empty(t, ev.Exceptions[0].Type)
}

func TestPolymorphicFields(t *testing.T) {
	t.Run("message строкой", func(t *testing.T) {
		ev := normalize(t, `{"message":"plain text"}`)
		assert.Equal(t, "plain text", ev.Message)
		assert.Equal(t, "plain text", ev.MessageTemplate)
	})

	t.Run("logentry с params: шаблон без подстановки", func(t *testing.T) {
		ev := normalize(t, `{"logentry":{"message":"Order %s not found","formatted":"Order 81723 not found","params":["81723"]}}`)
		assert.Equal(t, "Order 81723 not found", ev.Message)
		assert.Equal(t, "Order %s not found", ev.MessageTemplate)
	})

	t.Run("exception массивом (старые SDK)", func(t *testing.T) {
		ev := normalize(t, `{"exception":[{"type":"Error","value":"boom"}]}`)
		require.Len(t, ev.Exceptions, 1)
		assert.Equal(t, "Error", ev.Exceptions[0].Type)
	})

	t.Run("tags парами и не строками", func(t *testing.T) {
		ev := normalize(t, `{"tags":[["a","1"],["b",2],["c",true]]}`)
		assert.Equal(t, "1", ev.Tags["a"])
		assert.Equal(t, "2", ev.Tags["b"])
		assert.Equal(t, "true", ev.Tags["c"])
	})

	t.Run("user.id числом", func(t *testing.T) {
		ev := normalize(t, `{"user":{"id":42}}`)
		assert.Equal(t, "42", ev.User.Id)
	})

	t.Run("level warn", func(t *testing.T) {
		assert.Equal(t, "warning", normalize(t, `{"level":"WARN"}`).Level)
	})

	t.Run("значения по умолчанию", func(t *testing.T) {
		ev := normalize(t, `{}`)
		assert.Equal(t, "ffffffffffffffffffffffffffffffff", ev.EventId, "id от приёма")
		assert.Equal(t, "error", ev.Level)
		assert.Equal(t, "production", ev.Environment)
		assert.Equal(t, "other", ev.Platform)
		assert.Equal(t, receivedAt, ev.Timestamp)
	})
}

func TestTimestamp(t *testing.T) {
	inWindow := receivedAt.Add(-time.Hour)

	cases := map[string]struct {
		payload string
		want    time.Time
	}{
		"секунды":     {`{"timestamp":` + jsonNum(float64(inWindow.Unix())+0.25) + `}`, inWindow.Add(250 * time.Millisecond)},
		"RFC 3339":    {`{"timestamp":"` + inWindow.Format(time.RFC3339Nano) + `"}`, inWindow},
		"из будущего": {`{"timestamp":"2030-01-01T00:00:00Z"}`, receivedAt},
		"старше окна": {`{"timestamp":"2020-01-01T00:00:00Z"}`, receivedAt},
		"мусор":       {`{"timestamp":"вчера"}`, receivedAt},
		"минута вперёд — норма": {`{"timestamp":"` + receivedAt.Add(30*time.Second).Format(time.RFC3339) + `"}`, receivedAt.Add(30 * time.Second)},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got := normalize(t, c.payload).Timestamp
			assert.WithinDuration(t, c.want, got, time.Millisecond)
		})
	}
}

func TestScrub(t *testing.T) {
	ev := normalize(t, `{
		"request": {"url": "https://shop/cart", "headers": {"Authorization": "Bearer abc", "Cookie": "sid=1", "Accept": "text/html"},
			"data": {"login": "ivan", "password": "qwerty", "card_number": "4111111111111111"}},
		"extra": {"note": "оплата картой 4111 1111 1111 1111 не прошла", "order": "1234567890123456"},
		"breadcrumbs": {"values": [{"message": "POST /api/login", "data": {"api_key": "k-1", "status": 200}}]},
		"headers_list": [["X-Auth-Token", "secret-1"], ["Accept", "*/*"]]
	}`)

	var data map[string]any
	require.NoError(t, json.Unmarshal(ev.Data, &data))

	s := string(ev.Data)
	for _, leaked := range []string{"Bearer abc", "sid=1", "qwerty", "4111111111111111", "4111 1111 1111 1111", "k-1", "secret-1"} {
		assert.NotContains(t, s, leaked)
	}
	for _, kept := range []string{"text/html", "ivan", "1234567890123456", "POST /api/login", "*/*"} {
		assert.Contains(t, s, kept, "не чувствительное и не карта (не проходит Луна) остаётся")
	}
	assert.Equal(t, "https://shop/cart", ev.RequestUrl)
}

func TestIpAuto(t *testing.T) {
	ev := normalize(t, `{"user":{"ip_address":"{{auto}}"}}`)
	assert.Equal(t, "203.0.113.9", ev.User.IpAddress, "{{auto}} — IP клиента (sendDefaultPii)")
	assert.Contains(t, string(ev.Data), "203.0.113.9")

	ev = normalize(t, `{"user":{"id":"1"}}`)
	assert.Empty(t, ev.User.IpAddress, "без {{auto}} IP не сохраняется")
	assert.NotContains(t, string(ev.Data), "203.0.113.9")
}

func TestTagLimits(t *testing.T) {
	long := strings.Repeat("я", 300)
	ev := normalize(t, `{"tags":{"`+strings.Repeat("k", 40)+`":"v","ok":"`+long+`"}}`)
	assert.NotContains(t, ev.Tags, strings.Repeat("k", 40), "длинный ключ отбрасывается")
	assert.LessOrEqual(t, len(ev.Tags["ok"]), maxTagValueLen)
	assert.True(t, strings.HasPrefix(long, ev.Tags["ok"]), "обрезка не режет символ")
}

func TestBadPayload(t *testing.T) {
	_, err := New().Normalize(&eventModel.RawEvent{Payload: []byte(`not json`)})
	assert.Error(t, err)
	_, err = New().Normalize(&eventModel.RawEvent{Payload: []byte(`[1,2]`)})
	assert.Error(t, err)
}

func jsonNum(f float64) string {
	b, _ := json.Marshal(f)
	return string(b)
}
