package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rendau/zorko/internal/errs"
	m "github.com/rendau/zorko/internal/service/searchquery/model"
)

func TestParse(t *testing.T) {
	cases := []struct {
		q     string
		conds []m.Cond
		words []string
	}{
		{"", nil, nil},
		{"корзина", nil, []string{"корзина"}},
		{"browser.name:Chrome", []m.Cond{c("browser.name", m.OpEq, "Chrome")}, nil},
		{"Browser.Name:Chrome", []m.Cond{c("browser.name", m.OpEq, "Chrome")}, nil},
		{"!level:info", []m.Cond{c("level", m.OpNe, "info")}, nil},
		{`page:"корзина оплаты"`, []m.Cond{c("page", m.OpEq, "корзина оплаты")}, nil},
		{`msg:"say \"hi\""`, []m.Cond{c("msg", m.OpEq, `say "hi"`)}, nil},
		{"url:*cart*", []m.Cond{c("url", m.OpLike, "%cart%")}, nil},
		{"release:shop@1.*", []m.Cond{c("release", m.OpLike, "shop@1.%")}, nil},
		{"!url:*admin*", []m.Cond{c("url", m.OpNotLike, "%admin%")}, nil},
		{"url:*100%_off*", []m.Cond{c("url", m.OpLike, `%100\%\_off%`)}, nil},
		{
			"  browser.name:Safari   release:shop@1.4.2 корзина  TypeError ",
			[]m.Cond{c("browser.name", m.OpEq, "Safari"), c("release", m.OpEq, "shop@1.4.2")},
			[]string{"корзина", "TypeError"},
		},
		{"is:unresolved level:error", []m.Cond{c("level", m.OpEq, "error")}, nil},
		{"https://shop/cart", nil, []string{"https://shop/cart"}},
		{`"cannot read" x`, nil, []string{"cannot read", "x"}},
	}
	for _, c := range cases {
		t.Run(c.q, func(t *testing.T) {
			got, err := New().Parse(c.q)
			require.NoError(t, err)
			assert.Equal(t, c.conds, got.Conds)
			assert.Equal(t, c.words, got.Words)
		})
	}
}

func TestParseErrors(t *testing.T) {
	cases := map[string]string{
		`page:"незакрытая`:              "6",
		"level:":                        "1",
		"x:" + strings.Repeat("a", 201): "1",
		strings.Repeat("k:v ", 21):      "81",
	}
	for q, pos := range cases {
		_, err := New().Parse(q)
		full, ok := errs.AsErrFull(err)
		require.True(t, ok, q)
		assert.True(t, errors.Is(full.Err, errs.InvalidQuery), q)
		assert.Equal(t, pos, full.Fields["q"], q)
	}
}

func c(key string, op m.Op, value string) m.Cond {
	return m.Cond{Key: key, Op: op, Value: value}
}
