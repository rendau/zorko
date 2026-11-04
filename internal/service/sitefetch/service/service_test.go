package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	artifactModel "github.com/rendau/zorko/internal/domain/artifact/model"
	artifactService "github.com/rendau/zorko/internal/domain/artifact/service"
	projectModel "github.com/rendau/zorko/internal/domain/project/model"
)

type fakeProject struct{ p *projectModel.Main }

func (f *fakeProject) Get(context.Context, int64, bool) (*projectModel.Main, bool, error) {
	cp := *f.p
	return &cp, true, nil
}

type fakeArtifact struct {
	mu      sync.Mutex
	rows    map[string]*artifactModel.SiteFetch
	blobs   map[string][]byte
	bundles map[string]*artifactModel.Bundle
	files   map[int64][]*artifactModel.File
	next    int64
}

func newFakeArtifact() *fakeArtifact {
	return &fakeArtifact{rows: map[string]*artifactModel.SiteFetch{}, blobs: map[string][]byte{},
		bundles: map[string]*artifactModel.Bundle{}, files: map[int64][]*artifactModel.File{}}
}

func (f *fakeArtifact) GetSiteFetch(_ context.Context, _ int64, u string) (*artifactModel.SiteFetch, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rows[u]
	if !ok {
		return nil, false, nil
	}
	cp := *r
	return &cp, true, nil
}

func (f *fakeArtifact) SaveSiteFetch(_ context.Context, m *artifactModel.SiteFetch) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *m
	cp.FetchedAt = time.Now()
	f.rows[m.Url] = &cp
	return nil
}

func (f *fakeArtifact) FilesOfBundle(_ context.Context, id int64, _ []string) ([]*artifactModel.File, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.files[id], nil
}

func (f *fakeArtifact) StoreFile(_ context.Context, data []byte) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := artifactService.Sha1Hex(data)
	f.blobs[s] = data
	return s, nil
}

func (f *fakeArtifact) ClaimBundle(_ context.Context, nb *artifactModel.NewBundle, _, _ time.Duration) (*artifactModel.Bundle, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if b, ok := f.bundles[nb.Checksum]; ok {
		return b, false, nil
	}
	f.next++
	b := &artifactModel.Bundle{Id: f.next, Checksum: nb.Checksum, Source: nb.Source, State: artifactModel.StateAssembling}
	f.bundles[nb.Checksum] = b
	return b, true, nil
}

func (f *fakeArtifact) CompleteBundle(_ context.Context, id int64, _, _ string, files []*artifactModel.File) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[id] = files
	return nil
}

type site struct {
	srv      *httptest.Server
	jsHits   atomic.Int64
	mapHits  atomic.Int64
	header   atomic.Value
	jsStatus int
	delay    time.Duration
}

func newSite(t *testing.T) *site {
	s := &site{jsStatus: http.StatusOK}
	mux := http.NewServeMux()
	mux.HandleFunc("/assets/a.js", func(w http.ResponseWriter, r *http.Request) {
		s.jsHits.Add(1)
		s.header.Store(r.Header.Get("X-Zorko-Token"))
		time.Sleep(s.delay)
		w.WriteHeader(s.jsStatus)
		_, _ = w.Write([]byte("function a(){throw Error(1)}\n//# sourceMappingURL=a.js.map"))
	})
	mux.HandleFunc("/assets/a.js.map", func(w http.ResponseWriter, r *http.Request) {
		s.mapHits.Add(1)
		_, _ = w.Write([]byte(`{"version":3,"sources":["src/a.js"],"names":[],"mappings":"AAAA"}`))
	})
	mux.HandleFunc("/assets/nomap.js", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("function a(){}"))
	})
	mux.HandleFunc("/assets/redirect.js", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://evil.example.com/a.js", http.StatusFound)
	})
	s.srv = httptest.NewServer(mux)
	t.Cleanup(s.srv.Close)
	return s
}

func (s *site) host() string {
	u, _ := url.Parse(s.srv.URL)
	return u.Host
}

func setup(t *testing.T, conf Config, mutate func(p *projectModel.Main)) (*Service, *site, *fakeArtifact) {
	st := newSite(t)
	p := &projectModel.Main{Id: 1, OrgId: 1, SiteFetchEnabled: true, SiteFetchHost: []string{st.host()}, SiteFetchIntervalMin: 60}
	if mutate != nil {
		mutate(p)
	}
	conf.AllowLoopback = !conf.AllowLoopback // по умолчанию в тестах разрешаем 127.0.0.1
	fa := newFakeArtifact()
	return New(conf, &fakeProject{p: p}, fa), st, fa
}

func TestFetchOkAndMemory(t *testing.T) {
	s, st, fa := setup(t, Config{}, func(p *projectModel.Main) {
		p.SiteFetchHeaderName, p.SiteFetchHeaderValue = "X-Zorko-Token", "secret"
	})
	ctx := context.Background()
	res, err := s.Fetch(ctx, 1, st.srv.URL+"/assets/a.js")
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Empty(t, res.Code)
	assert.Equal(t, "~/assets/a.js.map", res.MapUrl)
	assert.NotEmpty(t, res.JsBlob)
	assert.Contains(t, string(fa.blobs[res.MapBlob]), `"sources":["src/a.js"]`)
	assert.Equal(t, "secret", st.header.Load(), "секретный заголовок")

	// второй раз — из памяти, без запросов
	res2, err := s.Fetch(ctx, 1, st.srv.URL+"/assets/a.js")
	require.NoError(t, err)
	assert.Equal(t, res.MapBlob, res2.MapBlob)
	assert.EqualValues(t, 1, st.jsHits.Load())
	assert.EqualValues(t, 1, st.mapHits.Load())
}

func TestFetchDisabledAndHosts(t *testing.T) {
	s, st, _ := setup(t, Config{}, func(p *projectModel.Main) { p.SiteFetchEnabled = false })
	res, err := s.Fetch(context.Background(), 1, st.srv.URL+"/assets/a.js")
	require.NoError(t, err)
	assert.Nil(t, res, "выключено")
	assert.Zero(t, st.jsHits.Load(), "ни одного запроса")

	s, st, _ = setup(t, Config{}, func(p *projectModel.Main) { p.SiteFetchHost = []string{"admin.mdev.kz"} })
	res, err = s.Fetch(context.Background(), 1, st.srv.URL+"/assets/a.js")
	require.NoError(t, err)
	assert.Equal(t, CodeHostNotAllowed, res.Code)
	assert.Zero(t, st.jsHits.Load())

	s, st, _ = setup(t, Config{}, func(p *projectModel.Main) { p.SiteFetchHost = nil })
	res, _ = s.Fetch(context.Background(), 1, st.srv.URL+"/assets/a.js")
	assert.Equal(t, CodeHostNotAllowed, res.Code, "пустой список — никуда")

	res, _ = s.Fetch(context.Background(), 1, "file:///etc/passwd")
	assert.Equal(t, CodeHostNotAllowed, res.Code)
}

func TestFetchFailuresAreRemembered(t *testing.T) {
	s, st, _ := setup(t, Config{}, nil)
	st.jsStatus = http.StatusNotFound
	for range 3 {
		res, err := s.Fetch(context.Background(), 1, st.srv.URL+"/assets/a.js")
		require.NoError(t, err)
		assert.Equal(t, "site_http_404", res.Code)
	}
	assert.EqualValues(t, 1, st.jsHits.Load(), "неудача тоже помнится до интервала")

	res, _ := s.Fetch(context.Background(), 1, st.srv.URL+"/assets/nomap.js")
	assert.Equal(t, CodeNoSourcemapUrl, res.Code)
}

func TestFetchSingleRequestForMany(t *testing.T) {
	s, st, _ := setup(t, Config{}, nil)
	st.delay = 100 * time.Millisecond
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			res, err := s.Fetch(context.Background(), 1, st.srv.URL+"/assets/a.js")
			assert.NoError(t, err)
			assert.Empty(t, res.Code)
		})
	}
	wg.Wait()
	assert.EqualValues(t, 1, st.jsHits.Load())
}

func TestFetchBlocksInternalAddresses(t *testing.T) {
	// хост в списке, но адрес — 127.0.0.1: соединение запрещено после DNS
	s, st, _ := setup(t, Config{AllowLoopback: true}, nil)
	res, err := s.Fetch(context.Background(), 1, st.srv.URL+"/assets/a.js")
	require.NoError(t, err)
	assert.Equal(t, CodeHostNotAllowed, res.Code)
	assert.Zero(t, st.jsHits.Load())

	for _, addr := range []string{"127.0.0.1:80", "[::1]:80", "169.254.169.254:80", "0.0.0.0:80", "[::ffff:127.0.0.1]:80", "[fe80::1]:80"} {
		assert.ErrorIs(t, s.dialControl("tcp", addr, nil), errBlockedAddress, addr)
	}
	assert.NoError(t, s.dialControl("tcp", "10.0.0.5:443", nil), "частная сеть разрешена")
}

func TestFetchRedirectToForeignHost(t *testing.T) {
	s, st, _ := setup(t, Config{}, nil)
	res, err := s.Fetch(context.Background(), 1, st.srv.URL+"/assets/redirect.js")
	require.NoError(t, err)
	assert.Equal(t, CodeHostNotAllowed, res.Code)
}

func TestFetchTimeoutAndSize(t *testing.T) {
	s, st, _ := setup(t, Config{RequestTimeout: 50 * time.Millisecond}, nil)
	st.delay = 300 * time.Millisecond
	res, err := s.Fetch(context.Background(), 1, st.srv.URL+"/assets/a.js")
	require.NoError(t, err)
	assert.Equal(t, CodeTimeout, res.Code)

	s, st, _ = setup(t, Config{MaxBytes: 10}, nil)
	res, err = s.Fetch(context.Background(), 1, st.srv.URL+"/assets/a.js")
	require.NoError(t, err)
	assert.Equal(t, CodeTooLarge, res.Code)
}

func TestSourceMappingUrl(t *testing.T) {
	for in, want := range map[string]string{
		"a()\n//# sourceMappingURL=a.js.map":     "a.js.map",
		"a()\n//@ sourceMappingURL=a.js.map\n":   "a.js.map",
		"a()\n/*# sourceMappingURL=a.css.map */": "a.css.map",
		"a()":                                    "",
		`var s="sourceMappingURL=x"`:             "",
		"a()\n//# sourceMappingURL=data:application/json;base64,e30=": "data:application/json;base64,e30=",
	} {
		assert.Equal(t, want, sourceMappingUrl([]byte(in)), in)
	}
}
