//go:build integration

package service_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf16"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainArtifactRepoBlobP "github.com/rendau/zorko/internal/domain/artifact/repo/blob"
	domainArtifactRepoDbP "github.com/rendau/zorko/internal/domain/artifact/repo/db"
	domainArtifactServiceP "github.com/rendau/zorko/internal/domain/artifact/service"
	eventModel "github.com/rendau/zorko/internal/domain/event/model"
	domainProjectRepoDbP "github.com/rendau/zorko/internal/domain/project/repo/db"
	domainProjectServiceP "github.com/rendau/zorko/internal/domain/project/service"
	sitefetchService "github.com/rendau/zorko/internal/service/sitefetch/service"
	"github.com/rendau/zorko/internal/service/symbolicate/service"
	"github.com/rendau/zorko/internal/testutil"
)

// Карты с сайта целиком: CI карты нет → zorko скачивает JS и карту с сайта, складывает в S3 и PG,
// переводит стек; повторные ошибки не ходят на сайт; выключено — ни одного запроса
func TestSiteFetchEndToEnd(t *testing.T) {
	ctx := context.Background()
	pool := testutil.PgPool(t)
	projectId := testutil.NewProject(t, pool)

	js, err := os.ReadFile("../testdata/vite/index-CC8DwTDE.js")
	require.NoError(t, err)
	mapData, err := os.ReadFile("../testdata/vite/index-CC8DwTDE.js.map")
	require.NoError(t, err)
	var hits atomic.Int64
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.URL.Path {
		case "/assets/index-CC8DwTDE.js":
			_, _ = w.Write(append(append([]byte{}, js...), []byte("\n//# sourceMappingURL=index-CC8DwTDE.js.map")...))
		case "/assets/index-CC8DwTDE.js.map":
			_, _ = w.Write(mapData)
		default:
			http.NotFound(w, r)
		}
	}))
	defer site.Close()
	host := strings.TrimPrefix(site.URL, "http://")

	_, err = pool.Exec(ctx, `update project set site_fetch_enabled = true, site_fetch_host = $2 where id = $1`, projectId, []string{host})
	require.NoError(t, err)

	art := domainArtifactServiceP.New(domainArtifactRepoDbP.New(pool), domainArtifactRepoBlobP.New(testutil.S3(t)))
	fetcher := sitefetchService.New(sitefetchService.Config{AllowLoopback: true},
		domainProjectServiceP.New(domainProjectRepoDbP.New(pool)), art)
	s := service.New(service.Config{}, art, fetcher)

	src := string(js)
	off := strings.Index(src, "throw RangeError")
	lineStart := strings.LastIndex(src[:off], "\n") + 1
	col := len(utf16.Encode([]rune(src[lineStart:off]))) + 1
	line := strings.Count(src[:off], "\n") + 1
	absUrl := site.URL + "/assets/index-CC8DwTDE.js"

	newEvent := func() *eventModel.Event {
		data, _ := json.Marshal(map[string]any{"exception": map[string]any{"values": []any{map[string]any{
			"type": "RangeError", "stacktrace": map[string]any{"frames": []any{map[string]any{
				"filename": absUrl, "abs_path": absUrl, "function": "t", "lineno": line, "colno": col}}}}}}})
		return &eventModel.Event{ProjectId: projectId, Data: data, Exceptions: []*eventModel.Exception{{Frames: []*eventModel.Frame{
			{Filename: absUrl, AbsPath: absUrl, Function: "t", Lineno: line, Colno: col}}}}}
	}

	ev := newEvent()
	s.Symbolicate(ctx, []*eventModel.Event{ev})
	f := ev.Exceptions[0].Frames[0]
	assert.Equal(t, "src/promo.js", f.Filename)
	assert.Equal(t, "validateRule", f.Function)
	assert.EqualValues(t, 2, hits.Load(), "JS и карта")

	var state, source string
	require.NoError(t, pool.QueryRow(ctx, `select sf.state, b.source from site_fetch sf join artifact_bundle b on b.id = sf.bundle_id
		where sf.project_id = $1 and sf.url = $2`, projectId, absUrl).Scan(&state, &source))
	assert.Equal(t, "ok", state)
	assert.Equal(t, "site", source)

	// новый экземпляр (как после перезапуска, без памяти поиска) — карта из базы, не с сайта
	s2 := service.New(service.Config{}, art, fetcher)
	ev = newEvent()
	s2.Symbolicate(ctx, []*eventModel.Event{ev, newEvent(), newEvent()})
	assert.Equal(t, "validateRule", ev.Exceptions[0].Frames[0].Function)
	assert.EqualValues(t, 2, hits.Load(), "повторные ошибки на сайт не ходят")

	// выключено — ни одного запроса
	_, err = pool.Exec(ctx, `update project set site_fetch_enabled = false where id = $1`, projectId)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `delete from site_fetch where project_id = $1`, projectId)
	require.NoError(t, err)
	s3 := service.New(service.Config{}, art, fetcher)
	ev = newEvent()
	s3.Symbolicate(ctx, []*eventModel.Event{ev})
	assert.Equal(t, "t", ev.Exceptions[0].Frames[0].Function)
	assert.EqualValues(t, 2, hits.Load())
}
