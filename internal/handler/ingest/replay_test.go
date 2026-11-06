package ingest

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	projectModel "github.com/rendau/zorko/internal/domain/project/model"
	replayModel "github.com/rendau/zorko/internal/domain/replay/model"
	"github.com/rendau/zorko/internal/service/projectlabel"
)

const testReplayId = "a1b2c3d4e5f60718293a4b5c6d7e8f90"

// replayEnvelope — как собирает конверт @sentry/replay 11.x (createReplayEnvelope): описание без
// length, запись с length; тело — строка {"segment_id":N} и zlib или JSON
func replayEnvelope(t *testing.T, replayId string, segmentId int, replayType string, compressed bool, headerSegment int) []byte {
	t.Helper()
	now := float64(time.Now().UnixMilli()) / 1000
	event := fmt.Sprintf(`{"type":"replay_event","replay_id":%q,"segment_id":%d,"replay_type":%q,`+
		`"replay_start_timestamp":%.3f,"timestamp":%.3f,"urls":["https://seller.mechta.kz/orders"],"error_ids":[],`+
		`"platform":"javascript","environment":"production","request":{"headers":{"User-Agent":"Mozilla/5.0"}},`+
		`"sdk":{"name":"sentry.javascript.vue","version":"11.11.0"}}`, replayId, segmentId, replayType, now-60, now)

	events := []byte(`[{"type":4,"data":{"href":"https://seller.mechta.kz/orders","width":1280,"height":720},"timestamp":1}]`)
	body := events
	if compressed {
		var buf bytes.Buffer
		zw := zlib.NewWriter(&buf)
		_, _ = zw.Write(events)
		require.NoError(t, zw.Close())
		body = buf.Bytes()
	}
	recording := append([]byte(`{"segment_id":`+strconv.Itoa(headerSegment)+"}\n"), body...)

	var env bytes.Buffer
	env.WriteString(`{"event_id":"` + replayId + `","sent_at":"2026-11-06T10:00:00.000Z","sdk":{"name":"sentry.javascript.vue","version":"11.11.0"}}` + "\n")
	env.WriteString(`{"type":"replay_event"}` + "\n" + event + "\n")
	env.WriteString(`{"type":"replay_recording","length":` + strconv.Itoa(len(recording)) + "}\n")
	env.Write(recording)
	return env.Bytes()
}

func (e *testEnv) sendReplay(t *testing.T, body []byte) *http.Response {
	t.Helper()
	w := e.do(t, envelopeReq("/api/1/envelope/?sentry_key="+testKey, body))
	return w.Result()
}

func TestReplayAccepted(t *testing.T) {
	env := newTestEnv(1<<20, 1<<20)
	env.projects.replay[1] = projectlabel.ReplaySettings{Mode: projectModel.ReplayModeError, HourLimit: 100}

	rep := env.sendReplay(t, replayEnvelope(t, testReplayId, 0, replayModel.TypeBuffer, true, 0))
	require.Equal(t, http.StatusOK, rep.StatusCode)
	assert.Empty(t, rep.Header.Get("X-Sentry-Rate-Limits"))

	require.Len(t, env.replayStore.bodies, 1)
	for key, body := range env.replayStore.bodies {
		assert.True(t, strings.HasPrefix(key, "replay/"+time.Now().UTC().Format("20060102")+"/1/"+testReplayId+"/0"), key)
		assert.Equal(t, byte(0x78), body[0], "тело без строки-заголовка")
		assert.True(t, env.replayStore.compressed[key])
	}
	require.Len(t, env.replayQueue.events, 1)
	msg := env.replayQueue.events[0]
	assert.Equal(t, testReplayId, msg.EventId, "сегменты одной записи — в один раздел")
	assert.EqualValues(t, 1, msg.ProjectId)
	assert.Positive(t, msg.BodySize)
	assert.Contains(t, string(msg.Payload), `"replay_type":"buffer"`)
	assert.Empty(t, env.queue.events, "в очередь ошибок ничего")
}

func TestReplayRawJsonBody(t *testing.T) {
	env := newTestEnv(1<<20, 1<<20)
	env.projects.replay[1] = projectlabel.ReplaySettings{Mode: projectModel.ReplayModeSession, HourLimit: 100}

	rep := env.sendReplay(t, replayEnvelope(t, testReplayId, 3, replayModel.TypeSession, false, 3))
	require.Equal(t, http.StatusOK, rep.StatusCode)
	require.Len(t, env.replayStore.bodies, 1)
	for key, body := range env.replayStore.bodies {
		assert.False(t, env.replayStore.compressed[key], "несжатое тело (CSP без blob:) сожмёт хранилище")
		assert.Equal(t, byte('['), body[0])
	}
}

func TestReplayStop(t *testing.T) {
	cases := []struct {
		name       string
		mode       string
		replayType string
	}{
		{"режим выключен", projectModel.ReplayModeOff, replayModel.TypeBuffer},
		{"доля сессий при режиме «при ошибке»", projectModel.ReplayModeError, replayModel.TypeSession},
		{"неизвестный режим", "bogus", replayModel.TypeBuffer},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := newTestEnv(1<<20, 1<<20)
			env.projects.replay[1] = projectlabel.ReplaySettings{Mode: c.mode, HourLimit: 100}

			rep := env.sendReplay(t, replayEnvelope(t, testReplayId, 0, c.replayType, true, 0))
			require.Equal(t, http.StatusOK, rep.StatusCode, "отказ по записи — не ошибка SDK")
			assert.Equal(t, "3600:replay:project", rep.Header.Get("X-Sentry-Rate-Limits"), "браузер перестаёт писать сессию")
			assert.Empty(t, env.replayStore.bodies)
			assert.Empty(t, env.replayQueue.events)
		})
	}
}

func TestReplayDisabledWithoutS3(t *testing.T) {
	env := newTestEnv(1<<20, 1<<20)
	env.projects.replay[1] = projectlabel.ReplaySettings{Mode: projectModel.ReplayModeError, HourLimit: 100}
	// приём без S3: nil-интерфейсы вместо хранилища и очереди
	env.router = New(newUsecaseWithoutReplay(env), 1<<20, 1<<20, 1<<20).Router()

	rep := env.sendReplay(t, replayEnvelope(t, testReplayId, 0, replayModel.TypeBuffer, true, 0))
	require.Equal(t, http.StatusOK, rep.StatusCode)
	assert.Equal(t, "3600:replay:project", rep.Header.Get("X-Sentry-Rate-Limits"))
}

func TestReplayHourLimit(t *testing.T) {
	env := newTestEnv(1<<20, 1<<20)
	env.projects.replay[1] = projectlabel.ReplaySettings{Mode: projectModel.ReplayModeError, HourLimit: 1}

	first := env.sendReplay(t, replayEnvelope(t, testReplayId, 0, replayModel.TypeBuffer, true, 0))
	assert.Empty(t, first.Header.Get("X-Sentry-Rate-Limits"))

	other := "ffffffffffffffffffffffffffffffff"
	second := env.sendReplay(t, replayEnvelope(t, other, 0, replayModel.TypeBuffer, true, 0))
	require.Equal(t, http.StatusOK, second.StatusCode)
	limit := second.Header.Get("X-Sentry-Rate-Limits")
	require.True(t, strings.HasSuffix(limit, ":replay:project"), limit)
	seconds, err := strconv.Atoi(strings.TrimSuffix(limit, ":replay:project"))
	require.NoError(t, err)
	assert.True(t, seconds >= 1 && seconds <= 3600, "до начала следующего часа")

	cont := env.sendReplay(t, replayEnvelope(t, testReplayId, 1, replayModel.TypeBuffer, true, 1))
	assert.Empty(t, cont.Header.Get("X-Sentry-Rate-Limits"), "начатая запись дописывается")
	assert.Len(t, env.replayQueue.events, 2)
}

func TestReplayInvalid(t *testing.T) {
	cases := map[string][]byte{
		"номер сегмента в заголовке записи другой": replayEnvelope(t, testReplayId, 2, replayModel.TypeBuffer, true, 3),
		"номер записи не hex":                      replayEnvelope(t, strings.Repeat("z", 32), 0, replayModel.TypeBuffer, true, 0),
		"неизвестный вид записи":                   replayEnvelope(t, testReplayId, 0, "video", true, 0),
	}
	// описание без записи: SDK так не шлёт
	only := replayEnvelope(t, testReplayId, 0, replayModel.TypeBuffer, true, 0)
	cases["описание без записи"] = only[:bytes.Index(only, []byte(`{"type":"replay_recording"`))]

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			env := newTestEnv(1<<20, 1<<20)
			env.projects.replay[1] = projectlabel.ReplaySettings{Mode: projectModel.ReplayModeSession, HourLimit: 100}

			rep := env.sendReplay(t, body)
			require.Equal(t, http.StatusOK, rep.StatusCode)
			assert.Empty(t, rep.Header.Get("X-Sentry-Rate-Limits"), "кривой сегмент — не повод останавливать запись")
			assert.Empty(t, env.replayStore.bodies)
		})
	}
}

func TestReplayStorageUnavailable(t *testing.T) {
	env := newTestEnv(1<<20, 1<<20)
	env.projects.replay[1] = projectlabel.ReplaySettings{Mode: projectModel.ReplayModeError, HourLimit: 100}
	env.replayStore.err = errors.New("s3 down")

	rep := env.sendReplay(t, replayEnvelope(t, testReplayId, 0, replayModel.TypeBuffer, true, 0))
	assert.Equal(t, http.StatusServiceUnavailable, rep.StatusCode, "SDK повторит отправку")
}

func TestReplayTooLarge(t *testing.T) {
	env := newTestEnv(1<<20, 1<<20)
	env.projects.replay[1] = projectlabel.ReplaySettings{Mode: projectModel.ReplayModeError, HourLimit: 100}
	env.router = New(env.usecase(), 1<<20, 1<<20, 10).Router()

	rep := env.sendReplay(t, replayEnvelope(t, testReplayId, 0, replayModel.TypeBuffer, true, 0))
	require.Equal(t, http.StatusOK, rep.StatusCode)
	assert.Empty(t, env.replayStore.bodies)
}
