package service

import (
	"encoding/json"
	"log/slog"
	"unicode/utf8"

	eventModel "github.com/rendau/zorko/internal/domain/event/model"
	"github.com/rendau/zorko/internal/service/symbolicate/jsscope"
)

const (
	contextLines   = 5
	maxContextLine = 200
)

// frameResult — итог перевода одного кадра
type frameResult struct {
	candidate bool
	ok        bool
	ciCode    string
	siteCode  string

	source    string
	line, col int    // исходные: строка с 1, колонка с 0
	token     string // имя из карты в позиции кадра (для имени функции вызываемого кадра)
	function  string
	pre, post []string
	ctxLine   string
	hasCtx    bool
	mapUrl    string
	via       string
	bundleId  int64
}

func (s *Service) apply(ev *eventModel.Event, results map[string]*resolved, files *loadedFiles) {
	type exResult struct {
		frames  []*frameResult
		changed bool
		any     bool
	}
	exResults := make([]exResult, len(ev.Exceptions))
	anyCandidate := false

	for ei, ex := range ev.Exceptions {
		frs := make([]*frameResult, len(ex.Frames))
		for fi, f := range ex.Frames {
			fr := &frameResult{}
			frs[fi] = fr
			p, ok := candidate(f)
			if !ok {
				continue
			}
			fr.candidate = true
			anyCandidate = true
			exResults[ei].any = true
			r := results[targetOf(ev, p).id()]
			if r == nil {
				fr.ciCode = CodeTimeBudgetExceeded
				continue
			}
			if !r.found() {
				fr.ciCode, fr.siteCode = r.ciCode, r.siteCode
				continue
			}
			ref := mapRef(r)
			m := files.maps[ref]
			if m == nil {
				fr.ciCode = files.codes[ref]
				if fr.ciCode == "" {
					fr.ciCode = CodeTimeBudgetExceeded
				}
				continue
			}
			s.translate(f, fr, m, files.js[r.jsBlob])
			if fr.ok {
				fr.mapUrl, fr.via, fr.bundleId = r.mapUrl, r.via, r.bundleId
				exResults[ei].changed = true
			}
		}

		// имя по месту вызова: кадр ниже по стеку (старше) вызывает этот — его токен и есть имя функции
		for fi := 1; fi < len(frs); fi++ {
			if fr := frs[fi]; fr.ok && fr.function == "" && frs[fi-1].ok && frs[fi-1].token != "" {
				fr.function = frs[fi-1].token
			}
		}
		exResults[ei].frames = frs
	}
	if !anyCandidate {
		return
	}

	// модель: её кадры видит группировка
	for ei, ex := range ev.Exceptions {
		for fi, f := range ex.Frames {
			fr := exResults[ei].frames[fi]
			switch {
			case fr.ok:
				f.Filename, f.AbsPath = fr.source, fr.source
				f.Lineno, f.Colno = fr.line, fr.col+1
				f.InApp = new(inApp(fr.source))
				if fr.function != "" {
					f.Function = fr.function
				}
				s.markUsed(fr.bundleId)
				metricFrameTotal.WithLabelValues("ok", fr.via).Inc()
			case fr.candidate:
				metricFrameTotal.WithLabelValues(fr.ciCode, "none").Inc()
			}
		}
	}

	// JSON события: его видит интерфейс
	var tree map[string]any
	if err := json.Unmarshal(ev.Data, &tree); err != nil {
		return
	}
	values := exceptionValues(tree)
	if len(values) != len(ev.Exceptions) {
		slog.Warn("symbolicate: exceptions mismatch", "event_id", ev.EventId)
		return
	}
	for ei, raw := range values {
		if !exResults[ei].any {
			continue
		}
		exMap, _ := raw.(map[string]any)
		st, _ := exMap["stacktrace"].(map[string]any)
		frames, _ := st["frames"].([]any)
		if len(frames) != len(exResults[ei].frames) {
			continue
		}
		if exResults[ei].changed {
			exMap["raw_stacktrace"] = rawCopy(st)
		}
		for fi, rawFrame := range frames {
			fm, ok := rawFrame.(map[string]any)
			fr := exResults[ei].frames[fi]
			if !ok || !fr.candidate {
				continue
			}
			writeFrame(fm, fr, ev.Exceptions[ei].Frames[fi])
		}
	}
	if data, err := json.Marshal(tree); err == nil {
		ev.Data = data
	}
}

func (s *Service) translate(f *eventModel.Frame, fr *frameResult, m *mapFile, js *jsFile) {
	genLine, genCol := f.Lineno, f.Colno-1
	source, token, line, col, ok := m.c.Source(genLine, genCol)
	if !ok || source == "" {
		fr.ciCode = CodePositionNotInMap
		return
	}
	fr.ok = true
	fr.source = cleanSource(source)
	fr.line, fr.col, fr.token = line, col, token

	if js != nil {
		if off := js.offset(genLine, genCol); off >= 0 {
			if sc := jsscope.Innermost(js.getScopes(), off); sc != nil && sc.Name != nil {
				fr.function = originalName(m, js, sc.Name)
				if sc.Owner != nil {
					if owner := originalName(m, js, sc.Owner); owner != "" && fr.function != "" {
						fr.function = owner + "." + fr.function
					}
				}
			}
		}
	}

	if lines := m.sourceLines(source); line >= 1 && line <= len(lines) {
		idx := line - 1
		fr.hasCtx = true
		fr.ctxLine = clip(lines[idx], col)
		for i := max(0, idx-contextLines); i < idx; i++ {
			fr.pre = append(fr.pre, clip(lines[i], 0))
		}
		for i := idx + 1; i < min(len(lines), idx+1+contextLines); i++ {
			fr.post = append(fr.post, clip(lines[i], 0))
		}
	}
}

// originalName — имя из карты в позиции имени в сжатом коде; карта молчит — текст как есть
// (имена свойств и методов сжатие не трогает)
func originalName(m *mapFile, js *jsFile, n *jsscope.Name) string {
	line, col := js.position(n.Pos)
	if _, name, _, _, ok := m.c.Source(line, col); ok && name != "" {
		return name
	}
	return n.Text
}

// clip — строка не длиннее maxContextLine символов, окном вокруг колонки col
func clip(s string, col int) string {
	if utf8.RuneCountInString(s) <= maxContextLine {
		return s
	}
	runes := []rune(s)
	start := max(0, col-maxContextLine/2)
	end := min(len(runes), start+maxContextLine)
	start = max(0, end-maxContextLine)
	out := string(runes[start:end])
	if start > 0 {
		out = "…" + out
	}
	if end < len(runes) {
		out += "…"
	}
	return out
}

// exceptionValues — исключения JSON события: {values: [...]} или сразу [...]
func exceptionValues(tree map[string]any) []any {
	switch v := tree["exception"].(type) {
	case map[string]any:
		values, _ := v["values"].([]any)
		return values
	case []any:
		return v
	}
	return nil
}

// rawCopy — исходный стек для переключателя «сжатый вид»; длинные строки кода — окном вокруг колонки
func rawCopy(st map[string]any) map[string]any {
	b, err := json.Marshal(st)
	if err != nil {
		return nil
	}
	var cp map[string]any
	if json.Unmarshal(b, &cp) != nil {
		return nil
	}
	frames, _ := cp["frames"].([]any)
	for _, raw := range frames {
		fm, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		col := 0
		if c, ok := fm["colno"].(float64); ok {
			col = int(c) - 1
		}
		if line, ok := fm["context_line"].(string); ok {
			fm["context_line"] = clip(line, col)
		}
		for _, k := range []string{"pre_context", "post_context"} {
			if arr, ok := fm[k].([]any); ok {
				for i, l := range arr {
					if ls, ok := l.(string); ok {
						arr[i] = clip(ls, 0)
					}
				}
			}
		}
	}
	return cp
}

func writeFrame(fm map[string]any, fr *frameResult, f *eventModel.Frame) {
	data, _ := fm["data"].(map[string]any)
	if data == nil {
		data = map[string]any{}
	}
	if !fr.ok {
		data["zorko_error"] = fr.ciCode
		if fr.siteCode != "" {
			data["zorko_site_error"] = fr.siteCode
		}
		fm["data"] = data
		return
	}
	fm["filename"] = f.Filename
	fm["abs_path"] = f.AbsPath
	fm["lineno"] = f.Lineno
	fm["colno"] = f.Colno
	fm["in_app"] = *f.InApp
	if f.Function != "" {
		fm["function"] = f.Function
	}
	delete(fm, "pre_context")
	delete(fm, "context_line")
	delete(fm, "post_context")
	if fr.hasCtx {
		fm["pre_context"] = nonNil(fr.pre)
		fm["context_line"] = fr.ctxLine
		fm["post_context"] = nonNil(fr.post)
	}
	data["sourcemap"] = fr.mapUrl
	data["resolved_with"] = fr.via
	data["symbolicated"] = true
	delete(data, "zorko_error")
	delete(data, "zorko_site_error")
	fm["data"] = data
}

func nonNil(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}
