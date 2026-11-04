package service

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strings"

	eventModel "github.com/rendau/zorko/internal/domain/event/model"
)

// Version — версия алгоритма отпечатка (ADR-004): хранится в issue_hash
const Version = "v1"

const (
	maxTitleLen = 200
	maxValueLen = 1000
	untitled    = "<без заголовка>"
)

type Service struct{}

func New() *Service {
	return &Service{}
}

func (s *Service) Group(ev *eventModel.Event) eventModel.Grouping {
	g := eventModel.Grouping{Version: Version}

	if top := topException(ev); top != nil {
		g.Type = top.Type
		g.Value = truncateRunes(firstLine(top.Value), maxValueLen)
	}
	g.Title = title(ev, g.Type, g.Value)
	g.Culprit = culprit(ev)
	g.Hash = hashParts(fingerprintParts(ev))

	return g
}

// fingerprintParts — отпечаток SDK (с {{ default }}) либо отпечаток по умолчанию
func fingerprintParts(ev *eventModel.Event) []string {
	if len(ev.Fingerprint) == 0 {
		return defaultParts(ev)
	}

	parts := []string{"custom"}
	for _, fp := range ev.Fingerprint {
		if isDefaultPlaceholder(fp) {
			parts = append(parts, defaultParts(ev)...)
			continue
		}
		parts = append(parts, "fp:"+fp)
	}
	return parts
}

func isDefaultPlaceholder(fp string) bool {
	return strings.ReplaceAll(fp, " ", "") == "{{default}}"
}

// defaultParts: исключения — тип + кадры своего кода (без строк и колонок), а без стека — тип и
// нормализованный текст; без исключений — нормализованный текст сообщения
func defaultParts(ev *eventModel.Event) []string {
	parts := make([]string, 0, 16)

	for _, exc := range ev.Exceptions {
		if !exc.Mechanism.Synthetic {
			parts = append(parts, "type:"+exc.Type)
		}

		frameParts := framesParts(exc.Frames)
		if len(frameParts) > 0 {
			parts = append(parts, frameParts...)
			continue
		}

		text := exc.Value
		if exc.Mechanism.Synthetic {
			text = ev.MessageTemplate
		}
		parts = append(parts, "msg:"+normalizeText(text))
	}

	if len(parts) > 0 {
		return parts
	}

	if ev.MessageTemplate != "" {
		return []string{"msg:" + normalizeText(ev.MessageTemplate)}
	}

	// ни исключения, ни сообщения: такие события склеиваются в одну проблему проекта
	return []string{"empty"}
}

// framesParts — кадры своего кода; если своих нет совсем — все кадры
func framesParts(frames []*eventModel.Frame) []string {
	selected := make([]*eventModel.Frame, 0, len(frames))
	for _, f := range frames {
		if isInApp(f) {
			selected = append(selected, f)
		}
	}
	if len(selected) == 0 {
		selected = frames
	}

	parts := make([]string, 0, len(selected))
	for _, f := range selected {
		location := frameLocation(f)
		function := frameFunction(f)
		if location == "" && function == "" {
			continue
		}
		parts = append(parts, "frame:"+location+"|"+function)
	}
	return parts
}

// isInApp — in_app от SDK, иначе по пути: библиотеки, расширения браузера и встроенный код — не свои
func isInApp(f *eventModel.Frame) bool {
	if f.InApp != nil {
		return *f.InApp
	}

	path := f.AbsPath
	if path == "" {
		path = f.Filename
	}

	switch {
	case path == "", path == "<anonymous>", path == "[native code]":
		return false
	case strings.Contains(path, "node_modules"),
		strings.HasPrefix(path, "node:"),
		strings.HasPrefix(path, "chrome-extension://"),
		strings.HasPrefix(path, "moz-extension://"),
		strings.HasPrefix(path, "safari-extension://"),
		strings.HasPrefix(path, "safari-web-extension://"):
		return false
	}
	return true
}

// frameLocation — module, иначе нормализованное имя файла
func frameLocation(f *eventModel.Frame) string {
	if f.Module != "" {
		return f.Module
	}
	path := f.Filename
	if path == "" {
		path = f.AbsPath
	}
	return normalizeFilename(path)
}

func frameFunction(f *eventModel.Frame) string {
	switch f.Function {
	case "?", "<anonymous>", "<unknown>":
		return ""
	}
	return f.Function
}

// normalizeFilename: без схемы и хоста, без query и #…, без хеша сборки в имени файла
func normalizeFilename(path string) string {
	if path == "" {
		return ""
	}

	if i := strings.Index(path, "://"); i > 0 {
		if u, err := url.Parse(path); err == nil && u.Scheme != "" {
			path = u.Path
			if path == "" {
				path = u.Opaque
			}
		} else {
			path = path[i+3:]
		}
	}

	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}

	dir, base := "", path
	if i := strings.LastIndex(path, "/"); i >= 0 {
		dir, base = path[:i+1], path[i+1:]
	}

	return dir + stripBuildHash(base)
}

// stripBuildHash: `app.3f9a1c.js` → `app.js`, `index-BhK3xyz1.js` → `index.js`, `chunk-vendors.js` — как есть.
// Хешем считается кусок от 6 символов перед расширением, в котором есть цифра или смешан регистр.
func stripBuildHash(base string) string {
	ext := ""
	if i := strings.LastIndex(base, "."); i > 0 {
		base, ext = base[:i], base[i:]
	}

	i := strings.LastIndexAny(base, ".-")
	if i <= 0 {
		return base + ext
	}

	if candidate := base[i+1:]; looksLikeHash(candidate) {
		return base[:i] + ext
	}
	return base + ext
}

func looksLikeHash(s string) bool {
	if len(s) < 6 {
		return false
	}
	hasDigit, hasUpper, hasLower := false, false, false
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
			hasDigit = true
		case c >= 'A' && c <= 'Z':
			hasUpper = true
		case c >= 'a' && c <= 'z':
			hasLower = true
		case c == '_':
		default:
			return false
		}
	}
	return hasDigit || (hasUpper && hasLower)
}

func topException(ev *eventModel.Event) *eventModel.Exception {
	if len(ev.Exceptions) == 0 {
		return nil
	}
	top := ev.Exceptions[len(ev.Exceptions)-1]
	if top.Mechanism.Synthetic {
		return nil
	}
	return top
}

func title(ev *eventModel.Event, excType, excValue string) string {
	var t string
	switch {
	case excType != "" && excValue != "":
		t = excType + ": " + excValue
	case excType != "" || excValue != "":
		t = excType + excValue
	case ev.Message != "":
		t = firstLine(ev.Message)
	case ev.MessageTemplate != "":
		t = firstLine(ev.MessageTemplate)
	default:
		t = untitled
	}
	return truncateRunes(t, maxTitleLen)
}

// culprit — где упало: transaction события, иначе верхний свой кадр, иначе путь страницы
func culprit(ev *eventModel.Event) string {
	if ev.Transaction != "" {
		return ev.Transaction
	}

	for i := len(ev.Exceptions) - 1; i >= 0; i-- {
		// у synthetic-исключения стек искусственный: сверху кадры самого SDK, а не место ошибки
		if ev.Exceptions[i].Mechanism.Synthetic {
			continue
		}
		frames := ev.Exceptions[i].Frames
		for j := len(frames) - 1; j >= 0; j-- {
			f := frames[j]
			if !isInApp(f) {
				continue
			}
			location, function := frameLocation(f), frameFunction(f)
			switch {
			case location != "" && function != "":
				return location + " в " + function
			case location != "":
				return location
			case function != "":
				return function
			}
		}
	}

	if ev.RequestUrl != "" {
		if u, err := url.Parse(ev.RequestUrl); err == nil && u.Path != "" {
			return u.Path
		}
	}

	return ""
}

func hashParts(parts []string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])[:32]
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(line)
}

func truncateRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}
