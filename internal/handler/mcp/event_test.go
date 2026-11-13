package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleEvent = `{
  "event_id": "9f0c",
  "message": "ignored when logentry is present",
  "logentry": {"formatted": "Корзина не открылась"},
  "exception": {"values": [
    {"type": "Error", "value": "inner", "stacktrace": {"frames": [
      {"filename": "node_modules/vue/dist/vue.js", "function": "callHook", "lineno": 10, "colno": 2,
       "context_line": "hook()", "pre_context": ["a"], "post_context": ["b"]}
    ]}},
    {"type": "TypeError", "value": "Cannot read properties of undefined (reading 'items')",
     "mechanism": {"type": "onerror", "handled": false},
     "stacktrace": {"frames": [
       {"filename": "app:///vendor.js", "function": "run", "lineno": 1, "colno": 100, "in_app": false,
        "data": {"zorko_error": "missing_sourcemap"}},
       {"filename": "src/pages/cart.vue", "function": "loadCart", "lineno": 42, "colno": 17, "in_app": true,
        "pre_context": ["l36", "l37", "l38", "l39", "l40", "l41"],
        "context_line": "  return cart.items.length",
        "post_context": ["l43"],
        "data": {"symbolicated": true}}
     ]}}
  ]},
  "breadcrumbs": {"values": [
    {"timestamp": 1731400000.5, "category": "ui.click", "message": "button.buy"},
    {"timestamp": "2026-11-12T10:00:01Z", "category": "fetch", "type": "http", "level": "error",
     "data": {"url": "/api/cart", "status_code": 500}}
  ]},
  "tags": [["environment", "production"], ["browser.name", "Chrome"]],
  "user": {"id": "42", "email": "u@example.com"},
  "request": {"url": "https://shop.example/cart", "method": "GET", "headers": {"User-Agent": "UA"}},
  "contexts": {"browser": {"name": "Chrome", "version": "130", "type": "browser"}, "broken": "x"},
  "sdk": {"name": "sentry.javascript.vue", "version": "9.1.0", "packages": []},
  "extra": {"cart_id": 7, "nested": {"a": 1}}
}`

func TestParseEvent(t *testing.T) {
	almaty := time.FixedZone("+05", 5*60*60)
	defer func(prev *time.Location) { time.Local = prev }(time.Local)
	time.Local = almaty

	out := parseEvent(sampleEvent)

	assert.Equal(t, "Корзина не открылась", out.Message)

	// пойманное снаружи исключение — первым; в нём место падения — первым кадром
	require.Len(t, out.Exceptions, 2)
	top := out.Exceptions[0]
	assert.Equal(t, "TypeError", top.Type)
	assert.Equal(t, "onerror", top.Mechanism)
	require.NotNil(t, top.Handled)
	assert.False(t, *top.Handled)

	require.Len(t, top.Frames, 2)
	crash := top.Frames[0]
	assert.Equal(t, "src/pages/cart.vue", crash.File)
	assert.Equal(t, "loadCart", crash.Function)
	assert.Equal(t, 42, crash.Line)
	assert.True(t, crash.InApp)
	assert.Empty(t, crash.SourcemapError)
	// не больше пяти строк до места, строка с ошибкой помечена
	assert.Equal(t, []string{
		"  37: l37", "  38: l38", "  39: l39", "  40: l40", "  41: l41",
		"> 42:   return cart.items.length",
		"  43: l43",
	}, crash.Code)

	vendor := top.Frames[1]
	assert.False(t, vendor.InApp)
	assert.Equal(t, "missing_sourcemap", vendor.SourcemapError)

	// у библиотечного кадра строк кода нет, in_app выводится по пути
	lib := out.Exceptions[1].Frames[0]
	assert.False(t, lib.InApp)
	assert.Empty(t, lib.Code)
	assert.Nil(t, out.Exceptions[1].Handled)

	require.Len(t, out.Breadcrumbs, 2)
	assert.Equal(t, "ui.click", out.Breadcrumbs[0].Category)
	// время — в поясе сервиса, каким бы SDK его ни прислал: числом или строкой
	assert.Equal(t, "2024-11-12T13:26:40.5+05:00", out.Breadcrumbs[0].Timestamp)
	assert.Equal(t, "2026-11-12T15:00:01+05:00", out.Breadcrumbs[1].Timestamp)
	assert.Equal(t, map[string]string{"url": "/api/cart", "status_code": "500"}, out.Breadcrumbs[1].Data)

	assert.Equal(t, map[string]string{"environment": "production", "browser.name": "Chrome"}, out.Tags)
	assert.Equal(t, map[string]string{"cart_id": "7", "nested": `{"a":1}`}, out.Extra)
	assert.Equal(t, map[string]map[string]string{
		"user":    {"id": "42", "email": "u@example.com"},
		"request": {"url": "https://shop.example/cart", "method": "GET", "header: User-Agent": "UA"},
		"browser": {"name": "Chrome", "version": "130"},
		"sdk":     {"name": "sentry.javascript.vue", "version": "9.1.0"},
	}, out.Contexts)
}

func TestParseEventBroken(t *testing.T) {
	for _, raw := range []string{"", "not json", "[]", `{"exception": 5, "breadcrumbs": "x", "tags": 1}`} {
		out := parseEvent(raw)
		assert.Empty(t, out.Exceptions, raw)
		assert.Empty(t, out.Breadcrumbs, raw)

		// пустые списки — списками, а не null: ответ проходит схему инструмента
		encoded, err := json.Marshal(out)
		require.NoError(t, err)
		assert.Contains(t, string(encoded), `"exceptions":[]`)
		assert.Contains(t, string(encoded), `"breadcrumbs":[]`)
	}
}

func TestParseEventLimits(t *testing.T) {
	frames := lo.Times(eventMaxFrames+20, func(i int) string {
		return fmt.Sprintf(`{"filename": "src/f%d.ts", "lineno": %d}`, i, i+1)
	})
	crumbs := lo.Times(eventMaxBreadcrumbs+15, func(i int) string {
		return fmt.Sprintf(`{"message": "crumb %d"}`, i)
	})
	raw := fmt.Sprintf(`{"exception": [{"type": "E", "value": %q, "stacktrace": {"frames": [%s]}}], "breadcrumbs": [%s]}`,
		strings.Repeat("я", eventMaxValueLen+500), strings.Join(frames, ","), strings.Join(crumbs, ","))

	out := parseEvent(raw)

	require.Len(t, out.Exceptions, 1)
	e := out.Exceptions[0]
	assert.Len(t, []rune(e.Value), eventMaxValueLen+1, "длинное значение обрезано по символам, не по байтам")
	// остаются кадры, ближние к месту падения
	require.Len(t, e.Frames, eventMaxFrames)
	assert.Equal(t, fmt.Sprintf("src/f%d.ts", eventMaxFrames+19), e.Frames[0].File)
	assert.Equal(t, 20, e.FramesOmitted)

	// остаются последние действия
	require.Len(t, out.Breadcrumbs, eventMaxBreadcrumbs)
	assert.Equal(t, fmt.Sprintf("crumb %d", eventMaxBreadcrumbs+14), out.Breadcrumbs[eventMaxBreadcrumbs-1].Message)
}
