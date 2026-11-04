package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	artifactModel "github.com/rendau/zorko/internal/domain/artifact/model"
	eventModel "github.com/rendau/zorko/internal/domain/event/model"
)

type Config struct {
	CacheBytes    int64         // разобранные файлы в памяти
	BatchBudget   time.Duration // время на перевод пачки
	FileTimeout   time.Duration // чтение одного файла из S3
	LookupTTL     time.Duration // память результатов поиска карт
	MarkUsedEvery time.Duration // отметка «пригодился» пачкой
}

type Service struct {
	conf     Config
	artifact artifactServiceI
	site     SiteFetcherI

	files   *lru
	lookups *ttlCache[*resolved]

	usedMu sync.Mutex
	used   map[int64]bool

	wg sync.WaitGroup
}

func New(conf Config, artifact artifactServiceI, site SiteFetcherI) *Service {
	if conf.CacheBytes == 0 {
		conf.CacheBytes = 256 << 20
	}
	if conf.BatchBudget == 0 {
		conf.BatchBudget = 10 * time.Second
	}
	if conf.FileTimeout == 0 {
		conf.FileTimeout = 5 * time.Second
	}
	if conf.LookupTTL == 0 {
		conf.LookupTTL = time.Minute
	}
	if conf.MarkUsedEvery == 0 {
		conf.MarkUsedEvery = time.Minute
	}
	return &Service{
		conf: conf, artifact: artifact, site: site,
		files:   newLru(conf.CacheBytes),
		lookups: newTtlCache[*resolved](conf.LookupTTL, 50000),
		used:    map[int64]bool{},
	}
}

// Start — отметка «пригодился» раз в MarkUsedEvery; при остановке — последняя отметка
func (s *Service) Start(ctx context.Context) {
	s.wg.Go(func() {
		ticker := time.NewTicker(s.conf.MarkUsedEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				flushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
				s.flushUsed(flushCtx)
				cancel()
				return
			case <-ticker.C:
				s.flushUsed(ctx)
			}
		}
	})
}

func (s *Service) Wait() {
	s.wg.Wait()
}

func (s *Service) markUsed(bundleId int64) {
	s.usedMu.Lock()
	s.used[bundleId] = true
	s.usedMu.Unlock()
}

func (s *Service) flushUsed(ctx context.Context) {
	s.usedMu.Lock()
	ids := make([]int64, 0, len(s.used))
	for id := range s.used {
		ids = append(ids, id)
	}
	s.used = map[int64]bool{}
	s.usedMu.Unlock()
	if len(ids) == 0 {
		return
	}
	if err := s.artifact.MarkUsed(ctx, ids); err != nil {
		slog.Warn("symbolicate: MarkUsed", "error", err)
	}
}

// target — что искать для кадра
type target struct {
	projectId int64
	debugId   string
	release   string
	dist      string
	absPath   string
}

func (t target) id() string {
	return fmt.Sprintf("%d|%s|%s|%s|%s", t.projectId, t.debugId, t.release, t.dist, t.absPath)
}

func targetOf(ev *eventModel.Event, absPath string) target {
	return target{projectId: ev.ProjectId, debugId: ev.DebugImages[absPath], release: ev.Release, dist: ev.Dist, absPath: absPath}
}

// resolved — найденные файлы для кадра или причины, почему нет
type resolved struct {
	bundleId int64
	jsBlob   string
	mapBlob  string
	mapUrl   string
	inline   bool // карта внутри сжатого JS (data:)
	via      string
	ciCode   string
	siteCode string
	retry    bool // сбой хранилища или нехватка времени: не запоминать
}

func (r *resolved) found() bool { return r.mapBlob != "" || r.inline }

// candidate — кадр, который можно перевести: есть путь, строка и колонка
func candidate(f *eventModel.Frame) (string, bool) {
	p := f.AbsPath
	if p == "" {
		p = f.Filename
	}
	return p, p != "" && f.Lineno > 0 && f.Colno > 0
}

func (s *Service) Symbolicate(ctx context.Context, events []*eventModel.Event) {
	targets := map[string]target{}
	for _, ev := range events {
		for _, ex := range ev.Exceptions {
			for _, f := range ex.Frames {
				if p, ok := candidate(f); ok {
					t := targetOf(ev, p)
					targets[t.id()] = t
				}
			}
		}
	}
	if len(targets) == 0 {
		return
	}

	start := time.Now()
	defer func() { metricBatchDuration.Observe(time.Since(start).Seconds()) }()

	ctx, cancel := context.WithTimeout(ctx, s.conf.BatchBudget)
	defer cancel()

	results := s.resolveAll(ctx, targets)
	files := s.loadAll(ctx, results)

	for _, ev := range events {
		s.apply(ev, results, files)
	}
}

func (s *Service) resolveAll(ctx context.Context, targets map[string]target) map[string]*resolved {
	results := make(map[string]*resolved, len(targets))
	var mu sync.Mutex
	g := errgroup.Group{}
	g.SetLimit(8)
	for id, t := range targets {
		if r, ok := s.lookups.get(id); ok {
			results[id] = r
			continue
		}
		g.Go(func() error {
			r := s.resolve(ctx, t)
			if !r.retry {
				s.lookups.put(id, r)
			}
			mu.Lock()
			results[id] = r
			mu.Unlock()
			return nil
		})
	}
	_ = g.Wait()
	return results
}

func failCode(ctx context.Context) string {
	if ctx.Err() != nil {
		return CodeTimeBudgetExceeded
	}
	return CodeStorageUnavailable
}

func (s *Service) resolve(ctx context.Context, t target) *resolved {
	r := &resolved{}
	fail := func(err error) *resolved {
		r.ciCode, r.retry = failCode(ctx), true
		slog.Warn("symbolicate: lookup", "project_id", t.projectId, "error", err)
		return r
	}

	if t.debugId != "" {
		files, err := s.artifact.FilesByDebugIds(ctx, t.projectId, []string{t.debugId})
		if err != nil {
			return fail(err)
		}
		if m := firstOfType(files, artifactModel.FileTypeSourceMap, 0); m != nil {
			r.bundleId, r.mapBlob, r.mapUrl, r.via = m.BundleId, m.Blob, m.Url, ViaDebugId
			if js := firstOfType(files, artifactModel.FileTypeMinified, m.BundleId); js != nil {
				r.jsBlob = js.Blob
			}
			return r
		}
	}

	if t.release != "" {
		cands := urlCandidates(t.absPath)
		files, err := s.artifact.FilesByReleaseUrls(ctx, t.projectId, t.release, t.dist, cands)
		if err != nil {
			return fail(err)
		}
		if js := firstByCandidates(files, cands); js != nil {
			ref := js.Sourcemap
			if ref == "" {
				ref = js.Url[strings.LastIndex(js.Url, "/")+1:] + ".map"
			}
			mapUrl := resolveRef(js.Url, ref)
			if strings.HasPrefix(mapUrl, "data:") {
				r.bundleId, r.jsBlob, r.inline, r.via = js.BundleId, js.Blob, true, ViaRelease
				return r
			}
			maps, err := s.artifact.FilesOfBundle(ctx, js.BundleId, []string{mapUrl})
			if err != nil {
				return fail(err)
			}
			if m := firstOfType(maps, artifactModel.FileTypeSourceMap, js.BundleId); m != nil {
				r.bundleId, r.jsBlob, r.mapBlob, r.mapUrl, r.via = js.BundleId, js.Blob, m.Blob, m.Url, ViaRelease
				return r
			}
		}
	}

	r.ciCode = CodeMapNotFound
	if t.debugId == "" && t.release == "" {
		r.ciCode = CodeNoDebugIdNoRelease
	}

	if s.site == nil {
		r.siteCode = CodeSiteDisabled
		return r
	}
	res, err := s.site.Fetch(ctx, t.projectId, t.absPath)
	switch {
	case err != nil:
		r.siteCode, r.retry = CodeSiteUnavailable, true
		if ctx.Err() != nil {
			r.siteCode = CodeTimeBudgetExceeded
		}
		slog.Warn("symbolicate: site fetch", "project_id", t.projectId, "url", t.absPath, "error", err)
	case res == nil:
		r.siteCode = CodeSiteDisabled
	case res.Code != "":
		r.siteCode = res.Code
	default:
		r.bundleId, r.jsBlob, r.mapBlob, r.mapUrl, r.via = res.BundleId, res.JsBlob, res.MapBlob, res.MapUrl, ViaSite
		r.ciCode = ""
	}
	return r
}

// firstOfType — первый файл вида typ (в архиве bundleId, если задан); файлы уже по свежести архива
func firstOfType(files []*artifactModel.File, typ string, bundleId int64) *artifactModel.File {
	for _, f := range files {
		if f.Type == typ && (bundleId == 0 || f.BundleId == bundleId) {
			return f
		}
	}
	return nil
}

// firstByCandidates — сжатый JS по порядку кандидатов пути
func firstByCandidates(files []*artifactModel.File, cands []string) *artifactModel.File {
	for _, c := range cands {
		for _, f := range files {
			if f.Url == c && f.Type == artifactModel.FileTypeMinified {
				return f
			}
		}
	}
	return nil
}

// loadedFiles — разобранные файлы пачки и причины, по которым не удалось
type loadedFiles struct {
	maps  map[string]*mapFile // по mapRef
	js    map[string]*jsFile  // по SHA-1
	codes map[string]string   // mapRef → код причины
}

// mapRef — SHA-1 карты или "inline:"+SHA-1 сжатого JS (карта внутри него)
func mapRef(r *resolved) string {
	if r.inline {
		return "inline:" + r.jsBlob
	}
	return r.mapBlob
}

func (s *Service) loadAll(ctx context.Context, results map[string]*resolved) *loadedFiles {
	out := &loadedFiles{maps: map[string]*mapFile{}, js: map[string]*jsFile{}, codes: map[string]string{}}
	jsBlobs, refs := map[string]bool{}, map[string]*resolved{}
	for _, r := range results {
		if !r.found() {
			continue
		}
		if r.jsBlob != "" {
			jsBlobs[r.jsBlob] = true
		}
		refs[mapRef(r)] = r
	}

	var mu sync.Mutex
	g := errgroup.Group{}
	g.SetLimit(8)
	for blob := range jsBlobs {
		g.Go(func() error {
			f, err := s.loadJs(ctx, blob)
			mu.Lock()
			defer mu.Unlock()
			if err == nil && f != nil {
				out.js[blob] = f
			}
			return nil
		})
	}
	_ = g.Wait()

	for ref, r := range refs {
		g.Go(func() error {
			var inlineJs *jsFile
			if r.inline {
				mu.Lock()
				inlineJs = out.js[r.jsBlob]
				mu.Unlock()
			}
			m, code := s.loadMap(ctx, r, inlineJs)
			mu.Lock()
			defer mu.Unlock()
			if m != nil {
				out.maps[ref] = m
			} else {
				out.codes[ref] = code
			}
			return nil
		})
	}
	_ = g.Wait()
	return out
}

func (s *Service) readFile(ctx context.Context, blob string) ([]byte, bool, error) {
	fctx, cancel := context.WithTimeout(ctx, s.conf.FileTimeout)
	defer cancel()
	return s.artifact.ReadFile(fctx, blob)
}

func (s *Service) loadJs(ctx context.Context, blob string) (*jsFile, error) {
	if v, ok := s.files.get("js:" + blob); ok {
		return v.(*jsFile), nil
	}
	data, found, err := s.readFile(ctx, blob)
	if err != nil || !found {
		return nil, err
	}
	f := newJsFile(data)
	s.files.put("js:"+blob, f, int64(len(data))*2)
	return f, nil
}

func (s *Service) loadMap(ctx context.Context, r *resolved, inlineJs *jsFile) (*mapFile, string) {
	ref := mapRef(r)
	if v, ok := s.files.get("map:" + ref); ok {
		return v.(*mapFile), ""
	}
	var data []byte
	if r.inline {
		if inlineJs == nil {
			return nil, failCode(ctx)
		}
		if data = inlineJs.inlineMap(); data == nil {
			return nil, CodeMapInvalid
		}
	} else {
		var found bool
		var err error
		data, found, err = s.readFile(ctx, r.mapBlob)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				slog.Warn("symbolicate: read map", "blob", r.mapBlob, "error", err)
			}
			return nil, failCode(ctx)
		}
		if !found {
			return nil, CodeMapNotFound
		}
	}
	m, err := parseMap(data)
	if err != nil {
		return nil, CodeMapInvalid
	}
	s.files.put("map:"+ref, m, int64(len(data))*3)
	return m, ""
}
