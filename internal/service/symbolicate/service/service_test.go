package service

import (
	"context"
	"crypto/sha1" //nolint:gosec
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"unicode/utf16"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	artifactModel "github.com/rendau/zorko/internal/domain/artifact/model"
	eventModel "github.com/rendau/zorko/internal/domain/event/model"
	groupingService "github.com/rendau/zorko/internal/service/grouping/service"
)

const (
	indexJs  = "index-CC8DwTDE.js"
	pageJs   = "page-6WDFjLaN.js"
	debugIdx = "11111111-1111-4111-8111-111111111111"
	debugPg  = "22222222-2222-4222-8222-222222222222"
)

type fakeArtifact struct {
	mu       sync.Mutex
	files    []*artifactModel.File
	blobs    map[string][]byte
	reads    int
	used     []int64
	failRead bool
}

func sha(b []byte) string { s := sha1.Sum(b); return hex.EncodeToString(s[:]) } //nolint:gosec

func fixture(t *testing.T, name string) []byte {
	b, err := os.ReadFile("../testdata/vite/" + name)
	require.NoError(t, err)
	return b
}

func newFake(t *testing.T) *fakeArtifact {
	f := &fakeArtifact{blobs: map[string][]byte{}}
	add := func(bundle int64, url, typ, debugId, sourcemap string, data []byte) {
		f.blobs[sha(data)] = data
		f.files = append(f.files, &artifactModel.File{BundleId: bundle, Url: url, Type: typ, DebugId: debugId, Sourcemap: sourcemap, Blob: sha(data), Size: int64(len(data))})
	}
	add(1, "~/assets/"+indexJs, artifactModel.FileTypeMinified, debugIdx, indexJs+".map", fixture(t, indexJs))
	add(1, "~/assets/"+indexJs+".map", artifactModel.FileTypeSourceMap, debugIdx, "", fixture(t, indexJs+".map"))
	add(1, "~/assets/"+pageJs, artifactModel.FileTypeMinified, debugPg, pageJs+".map", fixture(t, pageJs))
	add(1, "~/assets/"+pageJs+".map", artifactModel.FileTypeSourceMap, debugPg, "", fixture(t, pageJs+".map"))
	return f
}

func (f *fakeArtifact) FilesByDebugIds(_ context.Context, _ int64, ids []string) ([]*artifactModel.File, error) {
	var out []*artifactModel.File
	for _, file := range f.files {
		for _, id := range ids {
			if file.DebugId == id {
				out = append(out, file)
			}
		}
	}
	return out, nil
}

func (f *fakeArtifact) FilesByReleaseUrls(_ context.Context, _ int64, release, _ string, urls []string) ([]*artifactModel.File, error) {
	if release != "shop@1.0.0" {
		return nil, nil
	}
	var out []*artifactModel.File
	for _, file := range f.files {
		for _, u := range urls {
			if file.Url == u {
				out = append(out, file)
			}
		}
	}
	return out, nil
}

func (f *fakeArtifact) FilesOfBundle(_ context.Context, bundleId int64, urls []string) ([]*artifactModel.File, error) {
	var out []*artifactModel.File
	for _, file := range f.files {
		for _, u := range urls {
			if file.BundleId == bundleId && file.Url == u {
				out = append(out, file)
			}
		}
	}
	return out, nil
}

func (f *fakeArtifact) ReadFile(_ context.Context, blob string) ([]byte, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failRead {
		return nil, false, errors.New("s3 is down")
	}
	f.reads++
	b, ok := f.blobs[blob]
	return b, ok, nil
}

func (f *fakeArtifact) MarkUsed(_ context.Context, ids []int64) error {
	f.used = append(f.used, ids...)
	return nil
}

// frameAt — кадр браузера на месте needle в сжатом файле (колонка с 1, в единицах UTF-16)
func frameAt(t *testing.T, file, needle, fn string) *eventModel.Frame {
	src := string(fixture(t, file))
	off := strings.Index(src, needle)
	require.GreaterOrEqual(t, off, 0, needle)
	lineStart := strings.LastIndex(src[:off], "\n") + 1
	line := strings.Count(src[:off], "\n") + 1
	col := len(utf16.Encode([]rune(src[lineStart:off]))) + 1
	return &eventModel.Frame{
		Filename: "https://shop.mechta.kz/assets/" + file, AbsPath: "https://shop.mechta.kz/assets/" + file,
		Function: fn, Lineno: line, Colno: col, InApp: new(true),
	}
}

func eventWith(t *testing.T, frames []*eventModel.Frame, release string, debug map[string]string) *eventModel.Event {
	jsonFrames := make([]map[string]any, 0, len(frames))
	for _, f := range frames {
		jsonFrames = append(jsonFrames, map[string]any{"filename": f.Filename, "abs_path": f.AbsPath, "function": f.Function, "lineno": f.Lineno, "colno": f.Colno, "in_app": true})
	}
	data, err := json.Marshal(map[string]any{
		"exception": map[string]any{"values": []any{map[string]any{"type": "RangeError", "value": "x", "stacktrace": map[string]any{"frames": jsonFrames}}}},
	})
	require.NoError(t, err)
	return &eventModel.Event{
		ProjectId: 1, Release: release, DebugImages: debug, Data: data,
		Exceptions: []*eventModel.Exception{{Type: "RangeError", Frames: frames}},
	}
}

func stackFrames() func(t *testing.T) []*eventModel.Frame {
	return func(t *testing.T) []*eventModel.Frame {
		// старые кадры первыми, место падения — последним (как у SDK)
		return []*eventModel.Frame{
			frameAt(t, pageJs, "n.applyCode({total", "r"),
			frameAt(t, indexJs, "t(this.rules.find", "e.applyCode"),
			frameAt(t, indexJs, "throw RangeError", "t"),
		}
	}
}

func debugImages() map[string]string {
	return map[string]string{
		"https://shop.mechta.kz/assets/" + indexJs: debugIdx,
		"https://shop.mechta.kz/assets/" + pageJs:  debugPg,
	}
}

func TestSymbolicateByDebugId(t *testing.T) {
	fake := newFake(t)
	s := New(Config{}, fake, nil)
	ev := eventWith(t, stackFrames()(t), "", debugImages())
	s.Symbolicate(context.Background(), []*eventModel.Event{ev})

	frames := ev.Exceptions[0].Frames
	top := frames[2]
	assert.Equal(t, "src/promo.js", top.Filename)
	assert.Equal(t, 10, top.Lineno)
	assert.Equal(t, "validateRule", top.Function, "имя объемлющей функции по карте")
	assert.True(t, *top.InApp)

	assert.Equal(t, "src/promo.js", frames[1].Filename)
	assert.Equal(t, 5, frames[1].Lineno)
	assert.Equal(t, "PromoEngine.applyCode", frames[1].Function, "метод безымянного класса в переменной")

	assert.Contains(t, frames[0].Filename, "src/page.js")
	assert.Equal(t, 7, frames[0].Lineno)
	assert.Equal(t, "openCart", frames[0].Function, "export default function")

	// JSON для интерфейса
	var tree map[string]any
	require.NoError(t, json.Unmarshal(ev.Data, &tree))
	ex := exceptionValues(tree)[0].(map[string]any)
	jf := ex["stacktrace"].(map[string]any)["frames"].([]any)[2].(map[string]any)
	assert.Equal(t, "src/promo.js", jf["filename"])
	assert.Contains(t, jf["context_line"], "throw new RangeError")
	assert.Len(t, jf["pre_context"], 5)
	assert.Equal(t, true, jf["data"].(map[string]any)["symbolicated"])
	assert.Equal(t, "debug_id", jf["data"].(map[string]any)["resolved_with"])
	raw := ex["raw_stacktrace"].(map[string]any)["frames"].([]any)[2].(map[string]any)
	assert.Equal(t, "https://shop.mechta.kz/assets/"+indexJs, raw["filename"], "исходный сжатый стек сохранён")

	// второй раз — из памяти, без чтения S3
	reads := fake.reads
	ev2 := eventWith(t, stackFrames()(t), "", debugImages())
	s.Symbolicate(context.Background(), []*eventModel.Event{ev2})
	assert.Equal(t, reads, fake.reads)
	assert.Equal(t, "validateRule", ev2.Exceptions[0].Frames[2].Function)

	s.flushUsed(context.Background())
	assert.Equal(t, []int64{1}, fake.used)
}

func TestSymbolicateByRelease(t *testing.T) {
	s := New(Config{}, newFake(t), nil)
	ev := eventWith(t, stackFrames()(t), "shop@1.0.0", nil)
	s.Symbolicate(context.Background(), []*eventModel.Event{ev})
	top := ev.Exceptions[0].Frames[2]
	assert.Equal(t, "src/promo.js", top.Filename)
	assert.Equal(t, "validateRule", top.Function)

	var tree map[string]any
	require.NoError(t, json.Unmarshal(ev.Data, &tree))
	jf := exceptionValues(tree)[0].(map[string]any)["stacktrace"].(map[string]any)["frames"].([]any)[2].(map[string]any)
	assert.Equal(t, "release", jf["data"].(map[string]any)["resolved_with"])
}

func TestSymbolicateNotFound(t *testing.T) {
	cases := map[string]struct {
		release string
		debug   map[string]string
		code    string
	}{
		"no debug id, no release": {code: CodeNoDebugIdNoRelease},
		"unknown debug id":        {debug: map[string]string{"https://shop.mechta.kz/assets/" + indexJs: "33333333-3333-4333-8333-333333333333"}, code: CodeMapNotFound},
		"unknown release":         {release: "shop@9", code: CodeMapNotFound},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			s := New(Config{}, newFake(t), nil)
			ev := eventWith(t, []*eventModel.Frame{frameAt(t, indexJs, "throw RangeError", "t")}, tc.release, tc.debug)
			s.Symbolicate(context.Background(), []*eventModel.Event{ev})
			f := ev.Exceptions[0].Frames[0]
			assert.Equal(t, "t", f.Function, "кадр не тронут")
			assert.Contains(t, f.Filename, indexJs)

			var tree map[string]any
			require.NoError(t, json.Unmarshal(ev.Data, &tree))
			ex := exceptionValues(tree)[0].(map[string]any)
			assert.NotContains(t, ex, "raw_stacktrace")
			data := ex["stacktrace"].(map[string]any)["frames"].([]any)[0].(map[string]any)["data"].(map[string]any)
			assert.Equal(t, tc.code, data["zorko_error"])
			assert.Equal(t, CodeSiteDisabled, data["zorko_site_error"])
		})
	}
}

func TestSymbolicateStorageDown(t *testing.T) {
	fake := newFake(t)
	fake.failRead = true
	s := New(Config{}, fake, nil)
	ev := eventWith(t, []*eventModel.Frame{frameAt(t, indexJs, "throw RangeError", "t")}, "", debugImages())
	s.Symbolicate(context.Background(), []*eventModel.Event{ev})
	assert.Equal(t, "t", ev.Exceptions[0].Frames[0].Function, "сбой S3 — кадр как есть, приём идёт")

	var tree map[string]any
	require.NoError(t, json.Unmarshal(ev.Data, &tree))
	data := exceptionValues(tree)[0].(map[string]any)["stacktrace"].(map[string]any)["frames"].([]any)[0].(map[string]any)["data"].(map[string]any)
	assert.Equal(t, CodeStorageUnavailable, data["zorko_error"])

	// S3 вернулось — следующая пачка переводится (сбой не запоминается)
	fake.failRead = false
	ev = eventWith(t, []*eventModel.Frame{frameAt(t, indexJs, "throw RangeError", "t")}, "", debugImages())
	s.Symbolicate(context.Background(), []*eventModel.Event{ev})
	assert.Equal(t, "validateRule", ev.Exceptions[0].Frames[0].Function)
}

func TestSymbolicateSkipsFramesWithoutPosition(t *testing.T) {
	fake := newFake(t)
	s := New(Config{}, fake, nil)
	ev := eventWith(t, []*eventModel.Frame{{Filename: "/app/src/x.js", Function: "f", Lineno: 3}}, "", nil)
	before := string(ev.Data)
	s.Symbolicate(context.Background(), []*eventModel.Event{ev})
	assert.Equal(t, before, string(ev.Data))
	assert.Zero(t, fake.reads)
}

func TestPaths(t *testing.T) {
	for in, want := range map[string]string{
		"../../src/promo.js":                    "src/promo.js",
		"webpack://shop/./src/a.js":             "src/a.js",
		"webpack:///src/a.js":                   "src/a.js",
		"/@fs/Users/x/app/src/App.vue?vue&lang": "Users/x/app/src/App.vue",
		"https://cdn.kz/static/js/a.js":         "static/js/a.js",
		"../../node_modules/vue/dist/vue.js":    "node_modules/vue/dist/vue.js",
	} {
		assert.Equal(t, want, cleanSource(in), in)
	}
	assert.False(t, inApp("node_modules/vue/dist/vue.js"))
	assert.True(t, inApp("src/a.js"))

	assert.Equal(t, []string{"~/assets/a.js?v=1", "~/assets/a.js", "https://s.kz/assets/a.js?v=1", "https://s.kz/assets/a.js"},
		urlCandidates("https://s.kz/assets/a.js?v=1"))
	assert.Equal(t, "~/assets/a.js.map", resolveRef("~/assets/a.js", "a.js.map"))
	assert.Equal(t, "~/maps/a.js.map", resolveRef("~/assets/a.js", "../maps/a.js.map"))
	assert.Equal(t, "~/x/a.map", resolveRef("~/assets/a.js", "https://cdn.kz/x/a.map"))
}

func TestClip(t *testing.T) {
	long := strings.Repeat("a", 1000) + "XYZ" + strings.Repeat("b", 1000)
	out := clip(long, 1000)
	assert.Contains(t, out, "XYZ")
	assert.LessOrEqual(t, len([]rune(out)), maxContextLine+2)
	assert.Equal(t, "short", clip("short", 3))
}

// Одна ошибка в двух сборках (разные сжатые имена и файлы) после перевода — один отпечаток
func TestSameIssueAcrossBuilds(t *testing.T) {
	s := New(Config{}, newFake(t), nil)
	grouper := groupingService.New()

	build := func(file, fn string) *eventModel.Event {
		f := frameAt(t, indexJs, "throw RangeError", fn)
		f.Filename = "https://shop.mechta.kz/assets/" + file
		f.AbsPath = f.Filename
		ev := eventWith(t, []*eventModel.Frame{f}, "", map[string]string{f.AbsPath: debugIdx})
		return ev
	}
	a, b := build("index-CC8DwTDE.js", "t"), build("index-Zq81LmN0.js", "q")
	assert.NotEqual(t, grouper.Group(a).Hash, grouper.Group(b).Hash, "до перевода — разные проблемы")

	s.Symbolicate(context.Background(), []*eventModel.Event{a, b})
	ga, gb := grouper.Group(a), grouper.Group(b)
	assert.Equal(t, ga.Hash, gb.Hash, "после перевода — одна проблема")
	assert.Equal(t, "src/promo.js в validateRule", ga.Culprit)
}
