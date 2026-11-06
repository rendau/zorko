package ingest

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	projectModel "github.com/rendau/zorko/internal/domain/project/model"
	projectKeyModel "github.com/rendau/zorko/internal/domain/project_key/model"
	replayModel "github.com/rendau/zorko/internal/domain/replay/model"
	"github.com/rendau/zorko/internal/service/projectlabel"
	queueModel "github.com/rendau/zorko/internal/service/queue/model"
	ratelimitService "github.com/rendau/zorko/internal/service/ratelimit/service"
	replaylimitService "github.com/rendau/zorko/internal/service/replaylimit/service"
	usecase "github.com/rendau/zorko/internal/usecase/ingest"
)

const testKey = "0123456789abcdef0123456789abcdef"

type keyCacheMock struct {
	keys map[string]*projectKeyModel.Main
}

func (m *keyCacheMock) Find(projectId int64, publicKey string) (*projectKeyModel.Main, bool) {
	key, ok := m.keys[publicKey]
	if !ok || key.ProjectId != projectId {
		return nil, false
	}
	return key, true
}

type queueMock struct {
	events []*queueModel.Event
	err    error
}

func (m *queueMock) PublishEvents(_ context.Context, events []*queueModel.Event) error {
	if m.err != nil {
		return m.err
	}
	m.events = append(m.events, events...)
	return nil
}

type projectLabelMock struct {
	labels map[int64]string
	replay map[int64]projectlabel.ReplaySettings
}

func (m *projectLabelMock) Label(projectId int64) string {
	if label, ok := m.labels[projectId]; ok {
		return label
	}
	return "unknown"
}

func (m *projectLabelMock) Replay(projectId int64) projectlabel.ReplaySettings {
	if v, ok := m.replay[projectId]; ok {
		return v
	}
	return projectlabel.ReplaySettings{Mode: projectModel.ReplayModeOff}
}

// replayStoreMock — S3 для тел сегментов
type replayStoreMock struct {
	bodies     map[string][]byte
	compressed map[string]bool
	err        error
}

func (m *replayStoreMock) PutSegmentBody(_ context.Context, seg *replayModel.Segment, body []byte, compressed bool) (int64, error) {
	if m.err != nil {
		return 0, m.err
	}
	key := replayModel.SegmentKey(seg.ReceivedAt, seg.ProjectId, seg.ReplayId, seg.SegmentId)
	m.bodies[key] = body
	m.compressed[key] = compressed
	return int64(len(body)), nil
}

type testEnv struct {
	router       http.Handler
	queue        *queueMock
	keys         *keyCacheMock
	projects     *projectLabelMock
	replayStore  *replayStoreMock
	replayQueue  *queueMock
	replayLimits *replaylimitService.Service
}

func newTestEnv(maxBody, maxEvent int64) *testEnv {
	keys := &keyCacheMock{keys: map[string]*projectKeyModel.Main{
		testKey: {Id: 1, ProjectId: 1, PublicKey: testKey, Active: true},
	}}
	queue := &queueMock{}
	projects := &projectLabelMock{labels: map[int64]string{1: "mechta"}, replay: map[int64]projectlabel.ReplaySettings{}}
	store := &replayStoreMock{bodies: map[string][]byte{}, compressed: map[string]bool{}}
	replayQueue := &queueMock{}
	limits := replaylimitService.New()
	env := &testEnv{queue: queue, keys: keys, projects: projects, replayStore: store, replayQueue: replayQueue, replayLimits: limits}
	env.router = New(env.usecase(), maxBody, maxEvent, 1<<20).Router()
	return env
}

func (e *testEnv) usecase() *usecase.Usecase {
	return usecase.New(e.keys, ratelimitService.New(), e.queue, e.projects, e.replayStore, e.replayQueue, e.replayLimits)
}

// newUsecaseWithoutReplay — как в app.go без S3: хранилище и очередь записей — nil-интерфейсы
func newUsecaseWithoutReplay(e *testEnv) *usecase.Usecase {
	return usecase.New(e.keys, ratelimitService.New(), e.queue, e.projects, nil, nil, e.replayLimits)
}

func (e *testEnv) do(t *testing.T, r *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, r)
	return w
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)
	return b
}

func envelopeReq(path string, body []byte) *http.Request {
	r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/x-sentry-envelope")
	return r
}

func TestRealSdkEnvelopes(t *testing.T) {
	cases := []struct {
		fixture    string
		wantEvents int
	}{
		{"sentry-node-11.10.0/exception.envelope", 1},
		{"sentry-node-11.10.0/message.envelope", 1},
		{"sentry-node-11.10.0/session.envelope", 0},
	}
	for _, c := range cases {
		t.Run(c.fixture, func(t *testing.T) {
			env := newTestEnv(1<<20, 1<<20)
			body := readFixture(t, c.fixture)

			// так шлёт браузерный и node SDK: ключ в query
			w := env.do(t, envelopeReq("/api/1/envelope/?sentry_version=7&sentry_key="+testKey, body))
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			require.Len(t, env.queue.events, c.wantEvents)

			if c.wantEvents > 0 {
				ev := env.queue.events[0]
				header, _ := parseEnvelope(body)
				assert.Equal(t, header.Header.EventId, ev.EventId)
				assert.Equal(t, int64(1), ev.ProjectId)
				assert.False(t, ev.ReceivedAt.IsZero())
				assert.True(t, json.Valid(ev.Payload))

				var rep map[string]string
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rep))
				assert.Equal(t, ev.EventId, rep["id"])
			}
		})
	}
}

func TestKeySources(t *testing.T) {
	body := readFixture(t, "sentry-node-11.10.0/exception.envelope")

	t.Run("X-Sentry-Auth", func(t *testing.T) {
		env := newTestEnv(1<<20, 1<<20)
		r := envelopeReq("/api/1/envelope", body)
		r.Header.Set("X-Sentry-Auth", "Sentry sentry_version=7, sentry_key="+testKey+", sentry_client=x/1")
		assert.Equal(t, http.StatusOK, env.do(t, r).Code)
	})

	t.Run("dsn в заголовке envelope", func(t *testing.T) {
		env := newTestEnv(1<<20, 1<<20)
		withDsn := []byte(`{"event_id":"9172448878484ab3b8d683bbe4c8c0de","dsn":"https://` + testKey + `@zorko.test/1"}` + "\n" +
			`{"type":"event"}` + "\n" + `{"message":"hi"}` + "\n")
		assert.Equal(t, http.StatusOK, env.do(t, envelopeReq("/api/1/envelope/", withDsn)).Code)
	})

	t.Run("без ключа", func(t *testing.T) {
		env := newTestEnv(1<<20, 1<<20)
		assert.Equal(t, http.StatusForbidden, env.do(t, envelopeReq("/api/1/envelope/", body)).Code)
	})

	t.Run("ключ чужого проекта", func(t *testing.T) {
		env := newTestEnv(1<<20, 1<<20)
		w := env.do(t, envelopeReq("/api/2/envelope/?sentry_key="+testKey, body))
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Empty(t, env.queue.events)
	})

	t.Run("мусорный id проекта", func(t *testing.T) {
		env := newTestEnv(1<<20, 1<<20)
		assert.Equal(t, http.StatusBadRequest, env.do(t, envelopeReq("/api/abc/envelope/?sentry_key="+testKey, body)).Code)
	})
}

func TestCompression(t *testing.T) {
	body := readFixture(t, "sentry-node-11.10.0/exception.envelope")

	var gz bytes.Buffer
	gw := gzip.NewWriter(&gz)
	_, _ = gw.Write(body)
	_ = gw.Close()

	var br bytes.Buffer
	bw := brotli.NewWriter(&br)
	_, _ = bw.Write(body)
	_ = bw.Close()

	for name, c := range map[string]struct {
		encoding string
		body     []byte
	}{"gzip": {"gzip", gz.Bytes()}, "br": {"br", br.Bytes()}} {
		t.Run(name, func(t *testing.T) {
			env := newTestEnv(1<<20, 1<<20)
			r := envelopeReq("/api/1/envelope/?sentry_key="+testKey, c.body)
			r.Header.Set("Content-Encoding", c.encoding)
			w := env.do(t, r)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			require.Len(t, env.queue.events, 1)
		})
	}

	t.Run("неизвестное сжатие", func(t *testing.T) {
		env := newTestEnv(1<<20, 1<<20)
		r := envelopeReq("/api/1/envelope/?sentry_key="+testKey, body)
		r.Header.Set("Content-Encoding", "lzma")
		assert.Equal(t, http.StatusBadRequest, env.do(t, r).Code)
	})
}

func TestLimits(t *testing.T) {
	t.Run("сжатая бомба", func(t *testing.T) {
		// 50 МБ нулей сжимаются в ~50 КБ; предел распакованного тела — 1 МБ
		var gz bytes.Buffer
		gw, _ := gzip.NewWriterLevel(&gz, gzip.BestCompression)
		_, _ = gw.Write(make([]byte, 50<<20))
		_ = gw.Close()
		require.Less(t, gz.Len(), 1<<20)

		env := newTestEnv(1<<20, 1<<20)
		r := envelopeReq("/api/1/envelope/?sentry_key="+testKey, gz.Bytes())
		r.Header.Set("Content-Encoding", "gzip")
		assert.Equal(t, http.StatusRequestEntityTooLarge, env.do(t, r).Code)
	})

	t.Run("сырое тело больше предела", func(t *testing.T) {
		env := newTestEnv(1024, 1024)
		r := envelopeReq("/api/1/envelope/?sentry_key="+testKey, bytes.Repeat([]byte("a"), 2048))
		assert.Equal(t, http.StatusRequestEntityTooLarge, env.do(t, r).Code)
	})

	t.Run("событие больше предела выбрасывается, ответ 200", func(t *testing.T) {
		env := newTestEnv(1<<20, 100)
		body := readFixture(t, "sentry-node-11.10.0/exception.envelope")
		w := env.do(t, envelopeReq("/api/1/envelope/?sentry_key="+testKey, body))
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Empty(t, env.queue.events)
	})

	t.Run("предел ключа", func(t *testing.T) {
		env := newTestEnv(1<<20, 1<<20)
		env.keys.keys[testKey].RateLimitPerSec = 1
		body := readFixture(t, "sentry-node-11.10.0/exception.envelope")

		assert.Equal(t, http.StatusOK, env.do(t, envelopeReq("/api/1/envelope/?sentry_key="+testKey, body)).Code)
		w := env.do(t, envelopeReq("/api/1/envelope/?sentry_key="+testKey, body))
		assert.Equal(t, http.StatusTooManyRequests, w.Code)
		assert.Equal(t, "1", w.Header().Get("Retry-After"))
		assert.Equal(t, "1::project", w.Header().Get("X-Sentry-Rate-Limits"))
	})
}

func TestBadEnvelope(t *testing.T) {
	cases := map[string]string{
		"битый заголовок":          "not json\n",
		"битый заголовок элемента": `{}` + "\n" + `{oops` + "\n" + `{}`,
		"length больше тела":       `{}` + "\n" + `{"type":"event","length":1000}` + "\n" + `{}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			env := newTestEnv(1<<20, 1<<20)
			w := env.do(t, envelopeReq("/api/1/envelope/?sentry_key="+testKey, []byte(body)))
			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestEnvelopeWithLength(t *testing.T) {
	// payload с переводом строки внутри и length — граница элемента по длине, а не по "\n"
	payload := `{"message":"line1` + "\n" + `line2"}`
	body := `{"event_id":"9172448878484ab3b8d683bbe4c8c0de"}` + "\n" +
		`{"type":"attachment","length":3}` + "\n" + "abc" + "\n" +
		`{"type":"event","length":` + itoa(len(payload)) + `}` + "\n" + payload + "\n"

	env, err := parseEnvelope([]byte(body))
	require.NoError(t, err)
	require.Len(t, env.Items, 2)
	assert.Equal(t, "attachment", env.Items[0].Type)
	assert.Equal(t, "abc", string(env.Items[0].Payload))
	assert.Equal(t, payload, string(env.Items[1].Payload))
}

func TestOrigin(t *testing.T) {
	body := readFixture(t, "sentry-node-11.10.0/exception.envelope")

	env := newTestEnv(1<<20, 1<<20)
	env.keys.keys[testKey].AllowedOrigin = []string{"*.mechta.kz"}

	r := envelopeReq("/api/1/envelope/?sentry_key="+testKey, body)
	r.Header.Set("Origin", "https://shop.mechta.kz")
	assert.Equal(t, http.StatusOK, env.do(t, r).Code)

	r = envelopeReq("/api/1/envelope/?sentry_key="+testKey, body)
	r.Header.Set("Origin", "https://evil.example")
	assert.Equal(t, http.StatusForbidden, env.do(t, r).Code)

	r = envelopeReq("/api/1/envelope/?sentry_key="+testKey, body)
	assert.Equal(t, http.StatusOK, env.do(t, r).Code, "без Origin и Referer — не браузер, пропускаем")
}

// Мобильные приложения на Capacitor шлют Origin capacitor://localhost (iOS) и https://localhost
// (Android): без localhost в списке — 403, с ним — проходят (подсказка в окне ключа админки)
func TestOriginMobileApp(t *testing.T) {
	body := readFixture(t, "sentry-node-11.10.0/exception.envelope")
	origins := []string{"capacitor://localhost", "https://localhost"}

	env := newTestEnv(1<<20, 1<<20)
	env.keys.keys[testKey].AllowedOrigin = []string{"*.mechta.kz"}
	for _, origin := range origins {
		r := envelopeReq("/api/1/envelope/?sentry_key="+testKey, body)
		r.Header.Set("Origin", origin)
		assert.Equal(t, http.StatusForbidden, env.do(t, r).Code, origin)
	}

	env.keys.keys[testKey].AllowedOrigin = []string{"*.mechta.kz", "localhost"}
	for _, origin := range origins {
		r := envelopeReq("/api/1/envelope/?sentry_key="+testKey, body)
		r.Header.Set("Origin", origin)
		assert.Equal(t, http.StatusOK, env.do(t, r).Code, origin)
	}
}

func TestQueueUnavailable(t *testing.T) {
	env := newTestEnv(1<<20, 1<<20)
	env.queue.err = errors.New("broker down")
	body := readFixture(t, "sentry-node-11.10.0/exception.envelope")
	assert.Equal(t, http.StatusServiceUnavailable, env.do(t, envelopeReq("/api/1/envelope/?sentry_key="+testKey, body)).Code)
}

func TestStore(t *testing.T) {
	env := newTestEnv(1<<20, 1<<20)
	r := httptest.NewRequest(http.MethodPost, "/api/1/store/", strings.NewReader(`{"event_id":"AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE","message":"hi"}`))
	r.Header.Set("X-Sentry-Auth", "Sentry sentry_key="+testKey)
	w := env.do(t, r)
	require.Equal(t, http.StatusOK, w.Code)
	require.Len(t, env.queue.events, 1)
	assert.Equal(t, "aaaaaaaabbbbccccddddeeeeeeeeeeee", env.queue.events[0].EventId, "id приводится к формату Sentry")
}

func metricValue(t *testing.T, m prometheus.Metric) float64 {
	t.Helper()
	var out dto.Metric
	require.NoError(t, m.Write(&out))
	if out.Histogram != nil {
		return float64(out.Histogram.GetSampleCount())
	}
	return out.GetCounter().GetValue()
}

func TestMetricsProjectLabel(t *testing.T) {
	env := newTestEnv(1<<20, 1<<20)
	store := func(path string) int {
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"message":"hi"}`))
		r.Header.Set("X-Sentry-Auth", "Sentry sentry_key="+testKey)
		return env.do(t, r).Code
	}

	accepted := metricRequestTotal.WithLabelValues(resultAccepted, "mechta")
	events := metricEventAccepted.WithLabelValues("mechta")
	forbiddenUnknown := metricRequestTotal.WithLabelValues(resultForbidden, "unknown")
	badUnknown := metricRequestTotal.WithLabelValues(resultBadRequest, "unknown")
	durationMechta := metricRequestDuration.WithLabelValues("mechta").(prometheus.Metric)
	durationUnknown := metricRequestDuration.WithLabelValues("unknown").(prometheus.Metric)

	before := lo.Map([]prometheus.Metric{accepted, events, forbiddenUnknown, badUnknown, durationMechta, durationUnknown},
		func(m prometheus.Metric, _ int) float64 { return metricValue(t, m) })

	require.Equal(t, http.StatusOK, store("/api/1/store/"))
	require.Equal(t, http.StatusForbidden, store("/api/999/store/"), "чужой номер проекта")
	require.Equal(t, http.StatusBadRequest, store("/api/abc/store/"))

	assert.Equal(t, before[0]+1, metricValue(t, accepted))
	assert.Equal(t, before[1]+1, metricValue(t, events))
	assert.Equal(t, before[2]+1, metricValue(t, forbiddenUnknown), "номер из запроса не становится меткой")
	assert.Equal(t, before[3]+1, metricValue(t, badUnknown))
	assert.Equal(t, before[4]+1, metricValue(t, durationMechta))
	assert.Equal(t, before[5]+2, metricValue(t, durationUnknown))
}

func TestMetricsNoEvents(t *testing.T) {
	env := newTestEnv(1<<20, 1<<20)
	accepted := metricRequestTotal.WithLabelValues(resultAccepted, "mechta")
	noEvents := metricRequestTotal.WithLabelValues(resultNoEvents, "mechta")
	acceptedBefore, noEventsBefore := metricValue(t, accepted), metricValue(t, noEvents)

	// отметка сессии без ошибок: SDK шлёт такие при открытии страниц
	body := "{\"sent_at\":\"2026-11-06T10:00:00Z\"}\n{\"type\":\"session\"}\n{\"sid\":\"abc\",\"status\":\"ok\"}\n"
	r := envelopeReq("/api/1/envelope/", []byte(body))
	r.Header.Set("X-Sentry-Auth", "Sentry sentry_key="+testKey)
	w := env.do(t, r)

	require.Equal(t, http.StatusOK, w.Code, "служебное принимается, чтобы SDK не повторял")
	assert.Empty(t, env.queue.events)
	assert.Equal(t, noEventsBefore+1, metricValue(t, noEvents))
	assert.Equal(t, acceptedBefore, metricValue(t, accepted), "accepted — только запросы с ошибками")
}

func TestCorsPreflight(t *testing.T) {
	env := newTestEnv(1<<20, 1<<20)
	r := httptest.NewRequest(http.MethodOptions, "/api/1/envelope/", nil)
	r.Header.Set("Origin", "https://shop.mechta.kz")
	r.Header.Set("Access-Control-Request-Method", "POST")
	r.Header.Set("Access-Control-Request-Headers", "content-type,x-sentry-auth")
	w := env.do(t, r)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "https://shop.mechta.kz", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "content-type,x-sentry-auth", w.Header().Get("Access-Control-Allow-Headers"))
}

func TestClientIp(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.RemoteAddr = "10.0.0.5:5555"
	assert.Equal(t, "10.0.0.5", clientIp(r))

	r.Header.Set("X-Real-IP", "192.0.2.7")
	assert.Equal(t, "192.0.2.7", clientIp(r))

	r.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.1")
	assert.Equal(t, "203.0.113.9", clientIp(r))
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
