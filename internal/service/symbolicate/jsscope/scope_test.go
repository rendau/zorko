package jsscope

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readFixture(t *testing.T, name string) []byte {
	b, err := os.ReadFile("../testdata/vite/" + name)
	require.NoError(t, err)
	return b
}

func TestDemoduleKeepsOffsets(t *testing.T) {
	src := []byte(`import{a as e}from"./x.js";import"./y.css";const s="import x from 'z'",u=import.meta.url,r=/export{a}/.test(s);` +
		"const tpl=`${e(`import`)}export`;const m=()=>import(\"./a.js\");export default function(){return 1};export{e as b};export const q=1;export*from\"./z.js\";")
	out, ok := demodule(src)
	require.True(t, ok)
	require.Len(t, out, len(src))
	got := string(out)
	assert.NotContains(t, got, `import{a as e}`)
	assert.Contains(t, got, `"import x from 'z'"`, "строки не трогаются")
	assert.Contains(t, got, `/export{a}/`, "регулярные выражения не трогаются")
	assert.Contains(t, got, "`${e(`import`)}export`", "шаблоны не трогаются")
	assert.Contains(t, got, `_mport.meta.url`)
	assert.Contains(t, got, `_mport("./a.js")`)
	assert.Contains(t, got, `var _z_d_____= function(){return 1}`)
	assert.NotContains(t, got, `export`[:6]+`{e as b}`)
	assert.Contains(t, got, `       const q=1`)

	_, err := Parse(src)
	require.NoError(t, err)
}

func TestParseViteChunks(t *testing.T) {
	for _, name := range []string{"index-CC8DwTDE.js", "page-6WDFjLaN.js"} {
		scopes, err := Parse(readFixture(t, name))
		require.NoError(t, err, name)
		assert.NotEmpty(t, scopes, name)
	}
}

func TestInnermostAndNames(t *testing.T) {
	src := `var vv=class{constructor(e){this.rules=e}applyCode(e,t){return yv(this.rules.find(e=>e.code===t),e)}};` +
		`function yv(e,t){if(e.minTotal>t.total)throw RangeError("Сумма "+e.minTotal);return e.discount}` +
		`var bv=e=>e.toFixed(2),xv={totalOf(e){return e.reduce((e,t)=>e+t.price*t.qty,0)}};o.p=function(){};`
	scopes, err := Parse([]byte(src))
	require.NoError(t, err)

	find := func(needle string) *Scope {
		off := strings.Index(src, needle)
		require.GreaterOrEqual(t, off, 0, needle)
		return Innermost(scopes, off)
	}

	s := find("throw RangeError")
	require.NotNil(t, s.Name)
	assert.Equal(t, "yv", s.Name.Text)
	assert.Equal(t, strings.Index(src, "yv(e,t)"), s.Name.Pos, "позиция имени — в байтах исходника")

	s = find("toFixed")
	require.NotNil(t, s.Name)
	assert.Equal(t, "bv", s.Name.Text, "стрелочная функция в переменной")

	s = find("this.rules.find")
	require.NotNil(t, s.Name)
	assert.Equal(t, "applyCode", s.Name.Text, "метод класса")
	require.NotNil(t, s.Owner)
	assert.Equal(t, "vv", s.Owner.Text, "безымянный класс — имя переменной")

	s = find("e.code===t")
	assert.Nil(t, s.Name, "анонимный колбэк")

	s = find("e.reduce")
	require.NotNil(t, s.Name)
	assert.Equal(t, "totalOf", s.Name.Text)
	require.NotNil(t, s.Owner)
	assert.Equal(t, "xv", s.Owner.Text, "метод объекта — владелец объект")

	s = find("function(){}")
	require.NotNil(t, s.Name)
	assert.Equal(t, "p", s.Name.Text, "o.p = function")

	assert.Nil(t, Innermost(scopes, 0), "вне функций")
}
