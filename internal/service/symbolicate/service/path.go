package service

import (
	"net/url"
	"path"
	"strings"
)

// cleanSource — исходный путь из карты в понятном виде: webpack://shop/./src/a.js, ../../src/a.js,
// /@fs/…/src/a.js?vue&type=script → src/a.js
func cleanSource(src string) string {
	s := src
	if rest, ok := strings.CutPrefix(s, "webpack://"); ok {
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			rest = rest[i:]
		}
		s = rest
	} else if u, err := url.Parse(s); err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
		s = u.Path
	}
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimPrefix(s, "/@fs")
	s = path.Clean("/" + s)
	s = strings.TrimPrefix(s, "/")
	for strings.HasPrefix(s, "../") {
		s = s[3:]
	}
	if s == "." || s == ".." {
		return src
	}
	return s
}

// inApp — свой код или библиотека, по исходному пути
func inApp(source string) bool {
	switch {
	case strings.Contains(source, "node_modules/"),
		strings.HasPrefix(source, "\x00"),
		strings.Contains(source, "/@vite/"), strings.HasPrefix(source, "@vite/"),
		strings.HasPrefix(source, "vite/"),
		strings.HasPrefix(source, "(webpack)"), strings.HasPrefix(source, "webpack/"):
		return false
	}
	return true
}

// urlCandidates — пути файла в архиве для старого способа (по релизу), как ищет Sentry:
// ~/path?query, ~/path, полный адрес, полный адрес без query
func urlCandidates(absPath string) []string {
	u, err := url.Parse(absPath)
	if err != nil || u.Path == "" {
		return []string{absPath}
	}
	result := make([]string, 0, 4)
	add := func(v string) {
		for _, x := range result {
			if x == v {
				return
			}
		}
		result = append(result, v)
	}
	p := u.Path
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if u.RawQuery != "" {
		add("~" + p + "?" + u.RawQuery)
	}
	add("~" + p)
	if u.Scheme != "" {
		add(absPath)
		noQuery := *u
		noQuery.RawQuery, noQuery.Fragment = "", ""
		add(noQuery.String())
	} else {
		add(absPath)
	}
	return result
}

// resolveRef — адрес ref относительно файла base (оба в виде ~/путь или полного адреса); data: — как есть
func resolveRef(base, ref string) string {
	if ref == "" || strings.HasPrefix(ref, "data:") {
		return ref
	}
	if u, err := url.Parse(ref); err == nil && u.Scheme != "" && u.Host != "" {
		return "~" + u.Path
	}
	if strings.HasPrefix(ref, "~/") {
		return ref
	}
	b := strings.TrimPrefix(base, "~")
	if u, err := url.Parse(base); err == nil && u.Scheme != "" && u.Host != "" {
		b = u.Path
	}
	var p string
	if strings.HasPrefix(ref, "/") {
		p = path.Clean(ref)
	} else {
		p = path.Join(path.Dir(b), ref)
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return "~" + p
}
