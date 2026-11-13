package service

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha1" //nolint:gosec
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/crc32"
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

// zipRaw — архив с одним файлом bad.js, у которого размер и контрольная сумма в оглавлении заданы
// вручную (и могут не совпадать с содержимым)
func zipRaw(t *testing.T, content []byte, declaredSize uint64, crc uint32) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.CreateRaw(&zip.FileHeader{
		Name: "bad.js", Method: zip.Store, CRC32: crc,
		CompressedSize64: uint64(len(content)), UncompressedSize64: declaredSize,
	})
	require.NoError(t, err)
	_, err = w.Write(content)
	require.NoError(t, err)

	mf, _ := json.Marshal(map[string]any{"files": map[string]any{"bad.js": map[string]any{"url": "~/bad.js", "type": "source"}}})
	w, err = zw.Create("manifest.json")
	require.NoError(t, err)
	_, _ = w.Write(mf)
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// размер файла берётся из оглавления архива до чтения — архив, где он не совпадает с настоящим
// (или не сходится контрольная сумма), не собирается
func TestAssembleSizeFromHeader(t *testing.T) {
	content := bytes.Repeat([]byte("x"), 100)
	sum := crc32.ChecksumIEEE(content)

	cases := map[string]struct {
		declared uint64
		crc      uint32
		state    string
	}{
		"honest":          {declared: 100, crc: sum, state: artifactModel.StateOk},
		"longer than set": {declared: 10, crc: sum, state: artifactModel.StateError},
		"shorter than":    {declared: 5000, crc: sum, state: artifactModel.StateError},
		"bad checksum":    {declared: 100, crc: sum + 1, state: artifactModel.StateError},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fake := newFake()
			s := New(Config{TmpDir: t.TempDir()}, fake)
			b := run(t, s, fake, zipRaw(t, content, tc.declared, tc.crc))
			require.Equal(t, tc.state, b.State, b.Detail)
			if tc.state == artifactModel.StateError {
				assert.Contains(t, b.Detail, "file bad.js", b.Detail)
				assert.Empty(t, fake.files, "негодный файл в хранилище не попал")
				return
			}
			require.Len(t, fake.stored[b.Id], 1)
			assert.Equal(t, int64(100), fake.stored[b.Id][0].Size)
			assert.Equal(t, content, fake.files[sha(content)])
		})
	}
}

// inFlightArtifact — считает байты файлов, одновременно отданных на запись
type inFlightArtifact struct {
	*fakeArtifact
	limit int64

	tmu      sync.Mutex
	cur      int64
	count    int
	maxCur   int64
	maxCount int
	crowded  bool // сверх предела в работе был не один файл
}

func (a *inFlightArtifact) StoreFile(ctx context.Context, data []byte) (string, error) {
	a.tmu.Lock()
	a.cur += int64(len(data))
	a.count++
	a.maxCur, a.maxCount = max(a.maxCur, a.cur), max(a.maxCount, a.count)
	if a.cur > a.limit && a.count > 1 {
		a.crowded = true
	}
	a.tmu.Unlock()

	time.Sleep(3 * time.Millisecond)
	defer func() {
		a.tmu.Lock()
		a.cur -= int64(len(data))
		a.count--
		a.tmu.Unlock()
	}()
	return a.fakeArtifact.StoreFile(ctx, data)
}

func TestAssembleInFlightLimit(t *testing.T) {
	small := func(n int) []entry {
		list := make([]entry, 0, n)
		for i := range n {
			list = append(list, entry{fmt.Sprintf("s%d.js", i), fmt.Sprintf("~/s%d.js", i), "source", bytes.Repeat([]byte{byte('a' + i)}, 100)})
		}
		return list
	}

	t.Run("sum of files in work is limited", func(t *testing.T) {
		fake := &inFlightArtifact{fakeArtifact: newFake(), limit: 250}
		s := New(Config{TmpDir: t.TempDir(), MaxInFlightBytes: 250}, fake)
		b := run(t, s, fake.fakeArtifact, zipOf(t, small(12), 0))
		require.Equal(t, artifactModel.StateOk, b.State, b.Detail)
		assert.Len(t, fake.stored[b.Id], 12)
		assert.LessOrEqual(t, fake.maxCur, int64(250))
		assert.Equal(t, 2, fake.maxCount, "по 100 байт под предел 250 — два файла сразу, не один")
	})

	t.Run("file over the limit goes alone", func(t *testing.T) {
		fake := &inFlightArtifact{fakeArtifact: newFake(), limit: 250}
		s := New(Config{TmpDir: t.TempDir(), MaxInFlightBytes: 250}, fake)
		entries := append(small(6), entry{"big.js", "~/big.js", "source", bytes.Repeat([]byte("B"), 1000)})
		b := run(t, s, fake.fakeArtifact, zipOf(t, entries, 0))
		require.Equal(t, artifactModel.StateOk, b.State, b.Detail)
		assert.Len(t, fake.stored[b.Id], 7, "файл больше предела собран, а не завис")
		assert.Equal(t, int64(1000), fake.maxCur)
		assert.False(t, fake.crowded, "файл больше предела был в работе один")
	})

	t.Run("limit is free after a failed bundle", func(t *testing.T) {
		fake := &inFlightArtifact{fakeArtifact: newFake(), limit: 100}
		s := New(Config{TmpDir: t.TempDir(), MaxInFlightBytes: 100, MaxFileBytes: 500}, fake)

		// предел размера проверен до чтения: ни один файл архива не записан
		entries := append(small(6), entry{"big.js", "~/big.js", "source", bytes.Repeat([]byte("B"), 1000)})
		b := run(t, s, fake.fakeArtifact, zipOf(t, entries, 0))
		assert.Equal(t, artifactModel.StateError, b.State)
		assert.Contains(t, b.Detail, "is larger than")
		assert.Empty(t, fake.files)

		// файл занял весь предел и не прошёл сверку при чтении
		content := bytes.Repeat([]byte("x"), 100)
		b = run(t, s, fake.fakeArtifact, zipRaw(t, content, 100, crc32.ChecksumIEEE(content)+1))
		assert.Equal(t, artifactModel.StateError, b.State)
		assert.Contains(t, b.Detail, "checksum")

		// следующая сборка не ждёт вечно
		b = run(t, s, fake.fakeArtifact, zipOf(t, small(3), 0))
		require.Equal(t, artifactModel.StateOk, b.State, b.Detail)
	})
}
