package service

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha1" //nolint:gosec
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	artifactModel "github.com/rendau/zorko/internal/domain/artifact/model"
	"github.com/rendau/zorko/internal/service/assemble/model"
)

// fakeArtifact — архивы и куски в памяти
type fakeArtifact struct {
	mu      sync.Mutex
	chunks  map[string][]byte
	files   map[string][]byte
	bundles map[string]*artifactModel.Bundle
	stored  map[int64][]*artifactModel.File
	nextId  int64
}

func newFake() *fakeArtifact {
	return &fakeArtifact{chunks: map[string][]byte{}, files: map[string][]byte{}, bundles: map[string]*artifactModel.Bundle{}, stored: map[int64][]*artifactModel.File{}}
}

func sha(b []byte) string { s := sha1.Sum(b); return hex.EncodeToString(s[:]) } //nolint:gosec

func (f *fakeArtifact) put(b []byte) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.chunks[sha(b)] = b
	return sha(b)
}

func (f *fakeArtifact) MissingChunks(_ context.Context, cs []string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var m []string
	for _, c := range cs {
		if _, ok := f.chunks[c]; !ok {
			m = append(m, c)
		}
	}
	return m, nil
}

func (f *fakeArtifact) OpenChunk(_ context.Context, c string) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return io.NopCloser(bytes.NewReader(f.chunks[c])), nil
}

func (f *fakeArtifact) DeleteChunks(context.Context, []string) error { return nil }

func (f *fakeArtifact) StoreFile(_ context.Context, data []byte) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[sha(data)] = data
	return sha(data), nil
}

func (f *fakeArtifact) GetBundleByChecksum(_ context.Context, _ int64, c string) (*artifactModel.Bundle, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.bundles[c]
	if !ok {
		return nil, false, nil
	}
	cp := *b
	return &cp, true, nil
}

func (f *fakeArtifact) AddBundleProjects(context.Context, int64, []int64) error { return nil }

func (f *fakeArtifact) ClaimBundle(_ context.Context, nb *artifactModel.NewBundle, _, _ time.Duration) (*artifactModel.Bundle, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if b, ok := f.bundles[nb.Checksum]; ok && b.State != artifactModel.StateError {
		cp := *b
		return &cp, false, nil
	}
	f.nextId++
	b := &artifactModel.Bundle{Id: f.nextId, Checksum: nb.Checksum, State: artifactModel.StateAssembling, CreatedAt: time.Now()}
	f.bundles[nb.Checksum] = b
	cp := *b
	return &cp, true, nil
}

func (f *fakeArtifact) byId(id int64) *artifactModel.Bundle {
	for _, b := range f.bundles {
		if b.Id == id {
			return b
		}
	}
	return nil
}

func (f *fakeArtifact) CompleteBundle(_ context.Context, id int64, release, _ string, files []*artifactModel.File) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	b := f.byId(id)
	b.State, b.Release = artifactModel.StateOk, release
	f.stored[id] = files
	return nil
}

func (f *fakeArtifact) FailBundle(_ context.Context, id int64, detail string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	b := f.byId(id)
	b.State, b.Detail, b.AssembledAt = artifactModel.StateError, detail, new(time.Now())
	return nil
}

type entry struct {
	path, url, typ string
	content        []byte
}

func zipOf(t *testing.T, entries []entry, extraManifestFiles int) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]any{}
	for _, e := range entries {
		w, err := zw.Create(e.path)
		require.NoError(t, err)
		_, _ = w.Write(e.content)
		files[e.path] = map[string]any{"url": e.url, "type": e.typ, "headers": map[string]string{"Debug-Id": "AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE"}}
	}
	for i := 0; i < extraManifestFiles; i++ {
		files[fmt.Sprintf("x/%d", i)] = map[string]any{"url": "~/x", "type": "source"}
	}
	mf, _ := json.Marshal(map[string]any{"files": files, "release": "r1"})
	w, _ := zw.Create("manifest.json")
	_, _ = w.Write(mf)
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func run(t *testing.T, s *Service, fake *fakeArtifact, archive []byte) *artifactModel.Bundle {
	c := fake.put(archive)
	res, err := s.Assemble(context.Background(), &model.Request{OrgId: 1, Checksum: sha(archive), Chunks: []string{c}})
	require.NoError(t, err)
	require.Equal(t, model.StateCreated, res.State)
	s.Wait()
	b, _, _ := fake.GetBundleByChecksum(context.Background(), 1, sha(archive))
	return b
}

func TestAssembleOk(t *testing.T) {
	fake := newFake()
	s := New(Config{TmpDir: t.TempDir()}, fake)
	b := run(t, s, fake, zipOf(t, []entry{
		{"a.js", "~/a.js", "minified_source", []byte("var a")},
		{"a.js.map", "~/a.js.map", "source_map", []byte("{}")},
		{"x.bin", "~/x.bin", "indexed_ram_bundle", []byte("skip")},
	}, 0))
	require.Equal(t, artifactModel.StateOk, b.State, b.Detail)
	assert.Equal(t, "r1", b.Release)
	files := fake.stored[b.Id]
	require.Len(t, files, 2, "неизвестные виды файлов пропускаются")
	assert.Equal(t, "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", files[0].DebugId, "метка в нижнем регистре, заголовок без учёта регистра")

	// повтор — сразу ok
	res, err := s.Assemble(context.Background(), &model.Request{OrgId: 1, Checksum: b.Checksum, Chunks: []string{sha([]byte("gone"))}})
	require.NoError(t, err)
	assert.Equal(t, model.StateOk, res.State)
}

func TestAssembleLimits(t *testing.T) {
	cases := map[string]struct {
		conf    Config
		archive func(t *testing.T) []byte
		detail  string
	}{
		"bomb": {
			conf: Config{MaxFileBytes: 1 << 20},
			archive: func(t *testing.T) []byte {
				return zipOf(t, []entry{{"big.js", "~/big.js", "minified_source", bytes.Repeat([]byte("0"), 2<<20)}}, 0)
			},
			detail: "is larger than",
		},
		"unpacked": {
			conf: Config{MaxUnpackedBytes: 10},
			archive: func(t *testing.T) []byte {
				return zipOf(t, []entry{{"a.js", "~/a.js", "source", []byte("0123456789")}, {"b.js", "~/b.js", "source", []byte("0123456789")}}, 0)
			},
			detail: "unpacks to more than",
		},
		"too many files": {
			conf:    Config{MaxFiles: 5},
			archive: func(t *testing.T) []byte { return zipOf(t, nil, 6) },
			detail:  "too many files",
		},
		"bundle too large": {
			conf: Config{MaxBundleBytes: 100},
			archive: func(t *testing.T) []byte {
				return zipOf(t, []entry{{"a.js", "~/a.js", "source", bytes.Repeat([]byte("x"), 500)}}, 0)
			},
			detail: "bundle is larger than",
		},
		"file missing from zip": {
			archive: func(t *testing.T) []byte { return zipOf(t, nil, 1) },
			detail:  "is missing in the archive",
		},
		"no manifest": {
			archive: func(t *testing.T) []byte {
				var buf bytes.Buffer
				zw := zip.NewWriter(&buf)
				w, _ := zw.Create("a.js")
				_, _ = w.Write([]byte("x"))
				_ = zw.Close()
				return buf.Bytes()
			},
			detail: "manifest.json not found",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fake := newFake()
			tc.conf.TmpDir = t.TempDir()
			s := New(tc.conf, fake)
			b := run(t, s, fake, tc.archive(t))
			assert.Equal(t, artifactModel.StateError, b.State)
			assert.True(t, strings.Contains(b.Detail, tc.detail), b.Detail)
		})
	}
}

func TestAssembleValidation(t *testing.T) {
	s := New(Config{}, newFake())
	_, err := s.Assemble(context.Background(), &model.Request{Checksum: "nope", Chunks: []string{sha([]byte("a"))}})
	assert.Error(t, err)
	_, err = s.Assemble(context.Background(), &model.Request{Checksum: sha([]byte("a"))})
	assert.Error(t, err)

	res, err := s.Assemble(context.Background(), &model.Request{Checksum: sha([]byte("a")), Chunks: []string{sha([]byte("b"))}})
	require.NoError(t, err)
	assert.Equal(t, model.StateNotFound, res.State)
	assert.Equal(t, []string{sha([]byte("b"))}, res.MissingChunks)
}
