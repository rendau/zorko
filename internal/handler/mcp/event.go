package mcp

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/samber/lo"
)

// Разбор исходного JSON события Sentry (EventDetail.Data) в короткий вид для агента: цепочка
// исключений со стеком и строками кода, последние действия перед ошибкой, теги, окружение.
// То же, что карточка события в интерфейсе (admin/app/utils/event.ts), но с пределами размера:
// событие бывает до мегабайта, а агенту нужен ответ в несколько килобайт.
// Формат: develop.sentry.dev/sdk/data-model/event-payloads

const (
	eventMaxFrames      = 50   // кадров стека на исключение
	eventContextLines   = 5    // строк кода до и после места в кадре
	eventMaxBreadcrumbs = 30   // последних действий перед ошибкой
	eventMaxValueLen    = 2000 // символов в одном значении
	eventMaxItems       = 50   // ключей в одной группе (теги, контекст, extra)
)

type FrameOut struct {
	File     string   `json:"file,omitempty"`
	Function string   `json:"function,omitempty"`
	Line     int      `json:"line,omitempty"`
	Column   int      `json:"column,omitempty"`
	InApp    bool     `json:"in_app" jsonschema:"кадр из кода самого приложения, а не библиотеки"`
	Code     []string `json:"code,omitempty" jsonschema:"строки исходника вокруг места; строка с ошибкой помечена '>'"`
	// почему кадр не переведён по картам исходников (код причины); пусто — переведён или карты не нужны
	SourcemapError string `json:"sourcemap_error,omitempty"`
}

type ExceptionOut struct {
	Type      string     `json:"type,omitempty"`
	Value     string     `json:"value,omitempty"`
	Mechanism string     `json:"mechanism,omitempty"`
	Handled   *bool      `json:"handled,omitempty" jsonschema:"false — исключение никто не поймал"`
	Frames    []FrameOut `json:"frames" jsonschema:"стек: место падения первым"`
	// сколько кадров не поместилось (отброшены самые дальние от места падения)
	FramesOmitted int `json:"frames_omitted,omitempty"`
}

type BreadcrumbOut struct {
	Timestamp string            `json:"timestamp,omitempty"`
	Category  string            `json:"category,omitempty"`
	Type      string            `json:"type,omitempty"`
	Level     string            `json:"level,omitempty"`
	Message   string            `json:"message,omitempty"`
	Data      map[string]string `json:"data,omitempty"`
}

type EventParsedOut struct {
	Message     string                       `json:"message,omitempty"`
	Exceptions  []ExceptionOut               `json:"exceptions" jsonschema:"цепочка исключений: пойманное снаружи — первым"`
	Breadcrumbs []BreadcrumbOut              `json:"breadcrumbs" jsonschema:"последние действия перед ошибкой (клики, переходы, запросы, консоль), старые первыми"`
	Tags        map[string]string            `json:"tags,omitempty"`
	Contexts    map[string]map[string]string `json:"contexts,omitempty" jsonschema:"user, request, browser, os, device, runtime, app, sdk и др."`
	Extra       map[string]string            `json:"extra,omitempty"`
}

// parseEvent — битый JSON даёт пустой разбор: сводка события (EventSummaryOut) остаётся
func parseEvent(raw string) EventParsedOut {
	out := EventParsedOut{Exceptions: []ExceptionOut{}, Breadcrumbs: []BreadcrumbOut{}}

	var ev map[string]any
	if err := json.Unmarshal([]byte(raw), &ev); err != nil {
		return out
	}

	// пойманное снаружи исключение — последнее в цепочке SDK
	exceptions := lo.FilterMap(valuesOf(ev["exception"]), func(v any, _ int) (ExceptionOut, bool) {
		e, ok := v.(map[string]any)
		if !ok {
			return ExceptionOut{}, false
		}
		return exceptionOut(e), true
	})
	slices.Reverse(exceptions)
	out.Exceptions = exceptions

	crumbs := lo.FilterMap(valuesOf(ev["breadcrumbs"]), func(v any, _ int) (BreadcrumbOut, bool) {
		b, ok := v.(map[string]any)
		if !ok {
			return BreadcrumbOut{}, false
		}
		return breadcrumbOut(b), true
	})
	if len(crumbs) > eventMaxBreadcrumbs {
		crumbs = crumbs[len(crumbs)-eventMaxBreadcrumbs:]
	}
	out.Breadcrumbs = crumbs

	out.Tags = entriesOf(ev["tags"])
	out.Extra = entriesOf(ev["extra"])

	contexts := map[string]map[string]string{}
	add := func(key string, items map[string]string) {
		if len(items) > 0 {
			contexts[key] = items
		}
	}
	add("user", entriesOf(ev["user"]))
	if r, ok := ev["request"].(map[string]any); ok {
		items := map[string]string{}
		for _, key := range []string{"url", "method", "query_string"} {
			if v := stringOf(r[key]); v != "" {
				items[key] = clip(v)
			}
		}
		for k, v := range entriesOf(r["headers"]) {
			items["header: "+k] = v
		}
		add("request", items)
	}
	if all, ok := ev["contexts"].(map[string]any); ok {
		for key, value := range all {
			items := entriesOf(value)
			delete(items, "type")
			add(key, items)
		}
	}
	if sdk, ok := ev["sdk"].(map[string]any); ok {
		add("sdk", entriesOf(map[string]any{"name": sdk["name"], "version": sdk["version"]}))
	}
	if len(contexts) > 0 {
		out.Contexts = contexts
	}

	if logentry, ok := ev["logentry"].(map[string]any); ok {
		out.Message = lo.CoalesceOrEmpty(stringOf(logentry["formatted"]), stringOf(logentry["message"]))
	}
	if out.Message == "" {
		if m, ok := ev["message"].(map[string]any); ok {
			out.Message = lo.CoalesceOrEmpty(stringOf(m["formatted"]), stringOf(m["message"]))
		} else {
			out.Message = stringOf(ev["message"])
		}
	}
	out.Message = clip(out.Message)

	return out
}

func exceptionOut(e map[string]any) ExceptionOut {
	out := ExceptionOut{
		Type:   clip(stringOf(e["type"])),
		Value:  clip(stringOf(e["value"])),
		Frames: []FrameOut{},
	}
	if mechanism, ok := e["mechanism"].(map[string]any); ok {
		out.Mechanism = stringOf(mechanism["type"])
		if handled, ok := mechanism["handled"].(bool); ok {
			out.Handled = &handled
		}
	}

	var frames []any
	if stack, ok := e["stacktrace"].(map[string]any); ok {
		frames, _ = stack["frames"].([]any)
	}
	// в событии место падения — последний кадр; агенту — первым
	for i := len(frames) - 1; i >= 0; i-- {
		f, ok := frames[i].(map[string]any)
		if !ok {
			continue
		}
		if len(out.Frames) == eventMaxFrames {
			out.FramesOmitted = i + 1
			break
		}
		out.Frames = append(out.Frames, frameOut(f))
	}

	return out
}

func frameOut(f map[string]any) FrameOut {
	out := FrameOut{
		File:     lo.CoalesceOrEmpty(stringOf(f["filename"]), stringOf(f["abs_path"])),
		Function: stringOf(f["function"]),
		Line:     intOf(f["lineno"]),
		Column:   intOf(f["colno"]),
		InApp:    frameInApp(f),
	}
	if data, ok := f["data"].(map[string]any); ok {
		out.SourcemapError = lo.CoalesceOrEmpty(stringOf(data["zorko_error"]), stringOf(data["zorko_site_error"]))
	}

	// строки кода — только у кадров приложения: в библиотечных они агенту не помогают, а место занимают
	line, ok := f["context_line"].(string)
	if !ok || !out.InApp {
		return out
	}
	pre, post := stringsOf(f["pre_context"]), stringsOf(f["post_context"])
	if len(pre) > eventContextLines {
		pre = pre[len(pre)-eventContextLines:]
	}
	if len(post) > eventContextLines {
		post = post[:eventContextLines]
	}
	for i, text := range pre {
		out.Code = append(out.Code, codeLine(out.Line-len(pre)+i, text, false))
	}
	out.Code = append(out.Code, codeLine(out.Line, line, true))
	for i, text := range post {
		out.Code = append(out.Code, codeLine(out.Line+1+i, text, false))
	}

	return out
}

func codeLine(n int, text string, current bool) string {
	return fmt.Sprintf("%s %d: %s", lo.Ternary(current, ">", " "), n, clipTo(text, 300))
}

// frameInApp — in_app от SDK, иначе по пути, как на сервере (service/grouping) и в интерфейсе
func frameInApp(f map[string]any) bool {
	if v, ok := f["in_app"].(bool); ok {
		return v
	}
	path := lo.CoalesceOrEmpty(stringOf(f["abs_path"]), stringOf(f["filename"]))
	if path == "" || path == "<anonymous>" || path == "[native code]" {
		return false
	}
	if strings.Contains(path, "node_modules") || strings.HasPrefix(path, "node:") {
		return false
	}
	for _, scheme := range []string{"chrome-extension://", "moz-extension://", "safari-extension://", "safari-web-extension://"} {
		if strings.HasPrefix(path, scheme) {
			return false
		}
	}
	return true
}

func breadcrumbOut(b map[string]any) BreadcrumbOut {
	return BreadcrumbOut{
		Timestamp: isoTimeOf(b["timestamp"]),
		Category:  stringOf(b["category"]),
		Type:      stringOf(b["type"]),
		Level:     stringOf(b["level"]),
		Message:   clip(stringOf(b["message"])),
		Data:      entriesOf(b["data"]),
	}
}

// valuesOf — {values: [...]} или сразу [...]
func valuesOf(v any) []any {
	switch t := v.(type) {
	case []any:
		return t
	case map[string]any:
		items, _ := t["values"].([]any)
		return items
	}
	return nil
}

// entriesOf — объект или список пар [ключ, значение] как «ключ → строка»; не больше eventMaxItems
// ключей (по алфавиту), значения обрезаются
func entriesOf(v any) map[string]string {
	pairs := map[string]string{}
	switch t := v.(type) {
	case map[string]any:
		for k, x := range t {
			pairs[k] = stringOf(x)
		}
	case []any:
		for _, item := range t {
			if p, ok := item.([]any); ok && len(p) == 2 {
				pairs[stringOf(p[0])] = stringOf(p[1])
			}
		}
	}
	if len(pairs) == 0 {
		return nil
	}

	keys := lo.Keys(pairs)
	slices.Sort(keys)
	if len(keys) > eventMaxItems {
		keys = keys[:eventMaxItems]
	}
	return lo.SliceToMap(keys, func(k string) (string, string) { return k, clip(pairs[k]) })
}

func stringsOf(v any) []string {
	items, _ := v.([]any)
	return lo.Map(items, func(x any, _ int) string { return stringOf(x) })
}

func stringOf(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(raw)
}

func intOf(v any) int {
	n, _ := v.(float64)
	return int(n)
}

// isoTimeOf — секунды Unix (число) или строка времени → RFC 3339 в поясе сервиса, как остальные
// времена ответа; не разобрать — как есть
func isoTimeOf(v any) string {
	switch t := v.(type) {
	case float64:
		return time.UnixMilli(int64(t * 1000)).In(time.Local).Format(time.RFC3339Nano)
	case string:
		if parsed, err := time.Parse(time.RFC3339Nano, t); err == nil {
			return parsed.In(time.Local).Format(time.RFC3339Nano)
		}
		return t
	}
	return ""
}

func clip(s string) string {
	return clipTo(s, eventMaxValueLen)
}

func clipTo(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "…"
}
