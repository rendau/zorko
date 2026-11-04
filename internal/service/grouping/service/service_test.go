package service

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventModel "github.com/rendau/zorko/internal/domain/event/model"
	normalizeService "github.com/rendau/zorko/internal/service/normalize/service"
)

func frame(file, function string, line int, inApp *bool) *eventModel.Frame {
	return &eventModel.Frame{Filename: file, AbsPath: file, Function: function, Lineno: line, Colno: line * 3, InApp: inApp}
}

func typeError(frames ...*eventModel.Frame) *eventModel.Event {
	return &eventModel.Event{Exceptions: []*eventModel.Exception{{
		Type:   "TypeError",
		Value:  "Cannot read properties of undefined (reading 'price')",
		Frames: frames,
	}}}
}

func group(ev *eventModel.Event) eventModel.Grouping {
	return New().Group(ev)
}

func TestSameErrorDifferentPlaces(t *testing.T) {
	a := typeError(frame("https://shop.mechta.kz/assets/vendor-Ab12Cd34.js", "callWithErrorHandling", 10, nil),
		frame("https://shop.mechta.kz/assets/CartPage-Xy98Zw76.js", "addItem", 120, nil))
	b := typeError(frame("https://shop.mechta.kz/assets/vendor-Ab12Cd34.js", "callWithErrorHandling", 10, nil),
		frame("https://shop.mechta.kz/assets/Checkout-Qw12Er34.js", "submit", 40, nil))

	assert.NotEqual(t, group(a).Hash, group(b).Hash, "одна ошибка из двух мест — две проблемы")
}

func TestSameStackDifferentBuilds(t *testing.T) {
	a := typeError(frame("https://shop.mechta.kz/assets/CartPage-Xy98Zw76.js?v=1", "addItem", 120, nil))
	b := typeError(frame("https://cdn.mechta.kz/assets/CartPage-Lm55Nb21.js#x", "addItem", 133, nil))

	assert.Equal(t, group(a).Hash, group(b).Hash, "хеш сборки, хост, query и номер строки не участвуют")
}

func TestOnlyInAppFrames(t *testing.T) {
	no, yes := new(false), new(true)
	a := typeError(frame("/app/node_modules/vue/runtime.js", "callHook", 1, no), frame("/app/src/cart.js", "addItem", 5, yes))
	b := typeError(frame("/app/node_modules/vue/runtime.js", "callHookV2", 9, no), frame("/app/src/cart.js", "addItem", 5, yes))
	assert.Equal(t, group(a).Hash, group(b).Hash, "кадры библиотек не участвуют, когда есть свои")

	c := typeError(frame("/app/node_modules/vue/runtime.js", "callHook", 1, no))
	d := typeError(frame("/app/node_modules/vue/runtime.js", "otherHook", 1, no))
	assert.NotEqual(t, group(c).Hash, group(d).Hash, "своих кадров нет — берутся все")
}

func TestInAppHeuristics(t *testing.T) {
	cases := map[string]bool{
		"https://shop.mechta.kz/assets/app.js":     true,
		"/srv/app/node_modules/axios/lib/core.js":  false,
		"chrome-extension://abcdef/content.js":     false,
		"node:internal/process/task_queues":        false,
		"<anonymous>":                              false,
		"[native code]":                            false,
		"webpack:///./src/components/CartPage.vue": true,
	}
	for path, want := range cases {
		assert.Equal(t, want, isInApp(&eventModel.Frame{Filename: path}), path)
	}
}

func TestNormalizeFilename(t *testing.T) {
	cases := map[string]string{
		"https://shop.mechta.kz/assets/app.3f9a1c.js":     "/assets/app.js",
		"https://shop.mechta.kz/assets/index-BhK3xyz1.js": "/assets/index.js",
		"/assets/chunk-vendors.js":                        "/assets/chunk-vendors.js",
		"/assets/index-component.js":                      "/assets/index-component.js",
		"app:///main.jsbundle":                            "/main.jsbundle",
		"https://shop.mechta.kz/assets/app.js?v=12#L3":    "/assets/app.js",
		"webpack:///./src/App.vue":                        "/./src/App.vue",
	}
	for in, want := range cases {
		assert.Equal(t, want, normalizeFilename(in), in)
	}
}

func TestMessageNormalization(t *testing.T) {
	a := &eventModel.Event{MessageTemplate: "Order 81723 not found for user ivan@mechta.kz"}
	b := &eventModel.Event{MessageTemplate: "Order 90011 not found for user olga@mechta.kz"}
	c := &eventModel.Event{MessageTemplate: "Payment 81723 failed"}

	assert.Equal(t, group(a).Hash, group(b).Hash)
	assert.NotEqual(t, group(a).Hash, group(c).Hash)

	assert.Equal(t, "Request <uuid> to <url> from <ip> failed after <int> ms, trace <hex>, deadline",
		normalizeText("Request 9172448878484ab3b8d683bbe4c8c0de to https://api.mdev.kz/x?y=1 from 10.0.0.1 failed after 350 ms, trace 7f3a9c0b12, deadline"))
}

func TestExceptionWithoutStack(t *testing.T) {
	a := &eventModel.Event{Exceptions: []*eventModel.Exception{{Type: "ApiError", Value: "HTTP 502 on /order/81723"}}}
	b := &eventModel.Event{Exceptions: []*eventModel.Exception{{Type: "ApiError", Value: "HTTP 502 on /order/90011"}}}
	c := &eventModel.Event{Exceptions: []*eventModel.Exception{{Type: "OtherError", Value: "HTTP 502 on /order/90011"}}}

	assert.Equal(t, group(a).Hash, group(b).Hash)
	assert.NotEqual(t, group(a).Hash, group(c).Hash, "тип участвует")
}

func TestCustomFingerprint(t *testing.T) {
	base := typeError(frame("/src/a.js", "f", 1, nil))

	custom := typeError(frame("/src/a.js", "f", 1, nil))
	custom.Fingerprint = []string{"payment-gateway"}
	other := typeError(frame("/src/b.js", "g", 1, nil))
	other.Fingerprint = []string{"payment-gateway"}

	assert.Equal(t, group(custom).Hash, group(other).Hash, "свой отпечаток SDK заменяет стек")
	assert.NotEqual(t, group(base).Hash, group(custom).Hash)

	withDefault := typeError(frame("/src/a.js", "f", 1, nil))
	withDefault.Fingerprint = []string{"{{ default }}", "tenant-1"}
	withDefault2 := typeError(frame("/src/a.js", "f", 1, nil))
	withDefault2.Fingerprint = []string{"{{default}}", "tenant-2"}
	assert.NotEqual(t, group(withDefault).Hash, group(withDefault2).Hash, "{{ default }} + своё различие")
	assert.NotEqual(t, group(withDefault).Hash, group(base).Hash)
}

func TestTitleAndCulprit(t *testing.T) {
	ev := typeError(frame("/node_modules/vue/x.js", "callHook", 1, nil),
		frame("https://shop.mechta.kz/assets/CartPage-Xy98Zw76.js", "addItem", 120, nil))
	g := group(ev)
	assert.Equal(t, "TypeError: Cannot read properties of undefined (reading 'price')", g.Title)
	assert.Equal(t, "/assets/CartPage.js в addItem", g.Culprit)
	assert.Equal(t, "TypeError", g.Type)
	assert.Equal(t, Version, g.Version)
	assert.Len(t, g.Hash, 32)

	ev.Transaction = "/cart"
	assert.Equal(t, "/cart", group(ev).Culprit, "transaction важнее кадра")

	assert.Equal(t, "Order 1 not found", group(&eventModel.Event{Message: "Order 1 not found\nstack…"}).Title)
	assert.Equal(t, untitled, group(&eventModel.Event{}).Title)
	assert.Equal(t, "/checkout", group(&eventModel.Event{Message: "x", RequestUrl: "https://shop/checkout?a=1"}).Culprit)
}

// captureMessage в SDK 11 приходит synthetic-исключением со стеком: склеиваем по стеку, заголовок — сообщение
func TestNodeSdkSyntheticMessage(t *testing.T) {
	payload, err := os.ReadFile("../../normalize/service/testdata/node-message.json")
	require.NoError(t, err)

	normalizer := normalizeService.New()
	ev, err := normalizer.Normalize(&eventModel.RawEvent{ProjectId: 1, ReceivedAt: time.Now(), Payload: payload})
	require.NoError(t, err)

	g := group(ev)
	assert.Equal(t, "Order 81723 not found", g.Title)
	assert.Empty(t, g.Type, "тип synthetic-исключения не настоящий")
	assert.Empty(t, g.Culprit, "кадры synthetic-стека — это SDK, не место ошибки")

	ev2 := *ev
	ev2.Message, ev2.MessageTemplate = "Order 90011 not found", "Order 90011 not found"
	assert.Equal(t, g.Hash, group(&ev2).Hash, "то же место в коде — та же проблема")
}

func TestNodeSdkException(t *testing.T) {
	payload, err := os.ReadFile("../../normalize/service/testdata/node-exception.json")
	require.NoError(t, err)

	ev, err := normalizeService.New().Normalize(&eventModel.RawEvent{ProjectId: 1, ReceivedAt: time.Now(), Payload: payload})
	require.NoError(t, err)

	g := group(ev)
	assert.Equal(t, "TypeError: Cannot read properties of undefined (reading 'price')", g.Title)
	assert.Equal(t, "send в addItem", g.Culprit, "у node-кадров есть module")
}
