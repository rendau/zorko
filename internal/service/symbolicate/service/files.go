package service

import (
	"bytes"
	"encoding/base64"
	"net/url"
	"strings"
	"sync"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/go-sourcemap/sourcemap"

	"github.com/rendau/zorko/internal/service/symbolicate/jsscope"
)

// mapFile — разобранная карта
type mapFile struct {
	c *sourcemap.Consumer

	mu    sync.Mutex
	lines map[string][]string // исходники, разбитые на строки (по мере надобности)
}

func parseMap(data []byte) (*mapFile, error) {
	c, err := sourcemap.Parse("", data)
	if err != nil {
		return nil, err
	}
	return &mapFile{c: c, lines: map[string][]string{}}, nil
}

// sourceLines — строки исходника из sourcesContent; нет — nil
func (m *mapFile) sourceLines(source string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if l, ok := m.lines[source]; ok {
		return l
	}
	content := m.c.SourceContent(source)
	var l []string
	if content != "" {
		l = strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	}
	m.lines[source] = l
	return l
}

// jsFile — сжатый JS: начала строк (для перевода колонок) и функции (для имён), функции — лениво
type jsFile struct {
	src        []byte
	lineStarts []int

	once   sync.Once
	scopes []jsscope.Scope
}

func newJsFile(src []byte) *jsFile {
	starts := []int{0}
	for i, b := range src {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	return &jsFile{src: src, lineStarts: starts}
}

func (f *jsFile) getScopes() []jsscope.Scope {
	f.once.Do(func() {
		if s, err := jsscope.Parse(f.src); err == nil {
			f.scopes = s
		}
	})
	return f.scopes
}

func (f *jsFile) line(n int) []byte {
	if n < 1 || n > len(f.lineStarts) {
		return nil
	}
	start := f.lineStarts[n-1]
	end := len(f.src)
	if n < len(f.lineStarts) {
		end = f.lineStarts[n] - 1
	}
	return f.src[start:end]
}

// offset — байтовое смещение для строки (с 1) и колонки в единицах UTF-16 (с 0); -1 — вне файла
func (f *jsFile) offset(line, col16 int) int {
	l := f.line(line)
	if l == nil {
		return -1
	}
	b, units := 0, 0
	for b < len(l) && units < col16 {
		r, size := utf8.DecodeRune(l[b:])
		units += runeUnits(r)
		b += size
	}
	return f.lineStarts[line-1] + b
}

// position — строка (с 1) и колонка UTF-16 (с 0) для байтового смещения
func (f *jsFile) position(off int) (int, int) {
	lo, hi := 0, len(f.lineStarts)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if f.lineStarts[mid] <= off {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	units := 0
	for _, r := range string(f.src[f.lineStarts[lo]:off]) {
		units += runeUnits(r)
	}
	return lo + 1, units
}

// runeUnits — сколько единиц UTF-16 занимает символ (колонки браузеров и карт считаются в них)
func runeUnits(r rune) int {
	if n := utf16.RuneLen(r); n > 0 {
		return n
	}
	return 1
}

// inlineMap — карта из комментария //# sourceMappingURL=data:…;base64,… в конце файла
func (f *jsFile) inlineMap() []byte {
	idx := bytes.LastIndex(f.src, []byte("sourceMappingURL=data:"))
	if idx < 0 {
		return nil
	}
	ref := string(f.src[idx+len("sourceMappingURL="):])
	if i := strings.IndexAny(ref, "\n\r \t*"); i >= 0 {
		ref = ref[:i]
	}
	return decodeDataUrl(ref)
}

// decodeDataUrl — содержимое data:-адреса (base64 или процентная кодировка)
func decodeDataUrl(ref string) []byte {
	meta, data, ok := strings.Cut(strings.TrimPrefix(ref, "data:"), ",")
	if !ok {
		return nil
	}
	if strings.HasSuffix(meta, ";base64") {
		b, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			b, err = base64.RawStdEncoding.DecodeString(data)
		}
		if err != nil {
			return nil
		}
		return b
	}
	s, err := url.PathUnescape(data)
	if err != nil {
		return nil
	}
	return []byte(s)
}
