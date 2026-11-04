package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func adminDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"index.html":                "<html>index</html>",
		"_nuxt/entry.abc123.js":     "console.log('app')",
		"_nuxt/entry.abc123.js.br":  "BR",
		"_nuxt/entry.abc123.js.gz":  "GZ",
		"_nuxt/entry.abc123.css":    "body{}",
		"_nuxt/entry.abc123.css.gz": "GZCSS",
		"favicon.svg":               "<svg/>",
	}
	for name, body := range files {
		p := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	return dir
}

func get(h http.Handler, path, acceptEncoding string, headers ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, path, nil)
	if acceptEncoding != "" {
		r.Header.Set("Accept-Encoding", acceptEncoding)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		r.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestAdminEncodings(t *testing.T) {
	h := newAdminHandler(adminDir(t), "https://api.zorko.test")

	w := get(h, "/_nuxt/entry.abc123.js", "gzip, deflate, br")
	assert.Equal(t, "br", w.Header().Get("Content-Encoding"), "br предпочтительнее")
	assert.Equal(t, "BR", w.Body.String())
	assert.Contains(t, w.Header().Get("Content-Type"), "javascript", "тип — от оригинала")
	assert.Equal(t, "Accept-Encoding", w.Header().Get("Vary"))

	w = get(h, "/_nuxt/entry.abc123.js", "gzip")
	assert.Equal(t, "gzip", w.Header().Get("Content-Encoding"))

	w = get(h, "/_nuxt/entry.abc123.js", "br;q=0, gzip")
	assert.Equal(t, "gzip", w.Header().Get("Content-Encoding"), "q=0 — нельзя")

	w = get(h, "/_nuxt/entry.abc123.js", "")
	assert.Empty(t, w.Header().Get("Content-Encoding"))
	assert.Equal(t, "console.log('app')", w.Body.String())

	w = get(h, "/_nuxt/entry.abc123.css", "br, gzip")
	assert.Equal(t, "gzip", w.Header().Get("Content-Encoding"), "нет .br — берётся .gz")
}

func TestAdminCacheAndFallback(t *testing.T) {
	h := newAdminHandler(adminDir(t), "https://api.zorko.test")

	w := get(h, "/_nuxt/entry.abc123.js", "br")
	assert.Equal(t, "public, max-age=86400", w.Header().Get("Cache-Control"), "файлы сборки — сутки")

	for _, path := range []string{"/", "/index.html", "/issues/42", "/settings"} {
		w = get(h, path, "")
		assert.Equal(t, http.StatusOK, w.Code, path)
		assert.Equal(t, "<html>index</html>", w.Body.String(), path+": адрес SPA → index.html")
		assert.Equal(t, "no-cache", w.Header().Get("Cache-Control"), path)
	}

	last := get(h, "/", "").Header().Get("Last-Modified")
	require.NotEmpty(t, last)
	w = get(h, "/", "", "If-Modified-Since", last)
	assert.Equal(t, http.StatusNotModified, w.Code, "проверка без тела")

	w = get(h, "/../../etc/passwd", "")
	assert.NotContains(t, w.Body.String(), "root:", "выход за каталог не работает")
}

func TestAdminConfigJs(t *testing.T) {
	h := newAdminHandler(adminDir(t), `https://api.zorko.test/"x`)
	w := get(h, "/config.js", "")
	assert.Equal(t, `window.__APP_CONFIG__ = {"apiBaseUrl":"https://api.zorko.test/\"x"};`+"\n", w.Body.String(), "значение экранировано как JSON")
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
}

func TestAdminMethodsAndDisabled(t *testing.T) {
	h := newAdminHandler(adminDir(t), "")
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	assert.Nil(t, AdminHttpServerCreate("0", t.TempDir(), ""), "нет сборки — сервера нет")
}
