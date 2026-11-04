package jsscope

import (
	"bytes"

	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/js"
)

// Парсер goja не знает синтаксиса модулей (import/export, import.meta, import(), await на верхнем уровне),
// а сборки Vite — модули. demodule заменяет эти конструкции кодом той же длины в байтах, поэтому позиции
// функций в исходном файле не сдвигаются:
//   - import{…}from"x"; import"x"; export{…}; export{…}from"x"; export*from"x" — пробелами;
//   - export const|let|var|function|class|async — пробелами вместо export;
//   - export default X → var _z_d_____=X (14 байт, как у «export default»);
//   - import.meta → _mport.meta, import( → _mport(.
//
// Слова находятся лексером (строки, комментарии и шаблоны он пропускает), регулярные выражения
// различаются по предыдущему слову — в сжатом коде этого достаточно.

const defaultPlaceholder = "var _z_d_____="

// DefaultPlaceholder — имя, которое demodule подставляет для export default (у функции не настоящее имя)
const DefaultPlaceholder = "_z_d_____"

type token struct {
	tt    js.TokenType
	start int
	end   int
	text  []byte
}

var regexKeywords = map[string]bool{
	"return": true, "typeof": true, "instanceof": true, "in": true, "of": true, "new": true, "delete": true,
	"void": true, "throw": true, "case": true, "do": true, "else": true, "yield": true, "await": true,
}

func regexAllowed(prev *token) bool {
	if prev == nil {
		return true
	}
	switch prev.tt {
	case js.CloseParenToken, js.CloseBracketToken, js.CloseBraceToken:
		return false
	case js.TemplateStartToken, js.TemplateMiddleToken:
		return true
	}
	if js.IsPunctuator(prev.tt) || js.IsOperator(prev.tt) {
		return true
	}
	if js.IsIdentifierName(prev.tt) {
		return regexKeywords[string(prev.text)]
	}
	return false
}

// tokenize — значимые слова (без пробелов и комментариев) со смещениями в байтах
func tokenize(src []byte) ([]token, bool) {
	l := js.NewLexer(parse.NewInputBytes(src))
	tokens := make([]token, 0, len(src)/4)
	off := 0
	var prev *token
	for {
		tt, b := l.Next()
		if tt == js.ErrorToken {
			return tokens, l.Err() == nil || l.Err().Error() == "EOF"
		}
		if (tt == js.DivToken || tt == js.DivEqToken) && regexAllowed(prev) {
			tt, b = l.RegExp()
			if tt == js.ErrorToken {
				return tokens, false
			}
		}
		t := token{tt: tt, start: off, end: off + len(b), text: b}
		off += len(b)
		switch tt {
		case js.WhitespaceToken, js.LineTerminatorToken, js.CommentToken, js.CommentLineTerminatorToken:
			continue
		}
		tokens = append(tokens, t)
		prev = &tokens[len(tokens)-1]
	}
}

func is(t *token, s string) bool {
	return t != nil && string(t.text) == s
}

// demodule — копия src с заменёнными конструкциями модулей; ok=false — лексер не справился
func demodule(src []byte) ([]byte, bool) {
	tokens, ok := tokenize(src)
	if !ok {
		return nil, false
	}
	out := bytes.Clone(src)
	blank := func(from, to int) {
		for i := from; i < to; i++ {
			if out[i] != '\n' && out[i] != '\r' {
				out[i] = ' '
			}
		}
	}
	at := func(i int) *token {
		if i < len(tokens) {
			return &tokens[i]
		}
		return nil
	}

	depth := 0
	for i := 0; i < len(tokens); i++ {
		t := &tokens[i]
		switch t.tt {
		case js.OpenBraceToken, js.TemplateStartToken:
			depth++
			continue
		case js.CloseBraceToken, js.TemplateEndToken:
			depth--
			continue
		}
		if !js.IsIdentifierName(t.tt) {
			continue
		}
		next := at(i + 1)
		prevDot := i > 0 && tokens[i-1].tt == js.DotToken

		switch string(t.text) {
		case "import":
			if prevDot {
				continue
			}
			if next != nil && (next.tt == js.DotToken || next.tt == js.OpenParenToken) {
				copy(out[t.start:t.end], "_mport")
				continue
			}
			if depth != 0 {
				continue
			}
			// import … from"x" | import"x": до строки после from (или сразу после import)
			j := i + 1
			for ; j < len(tokens); j++ {
				if tokens[j].tt == js.StringToken && (j == i+1 || is(&tokens[j-1], "from")) {
					break
				}
			}
			if j == len(tokens) {
				return nil, false
			}
			end := tokens[j].end
			if n := at(j + 1); n != nil && n.tt == js.SemicolonToken {
				end = n.end
				j++
			}
			blank(t.start, end)
			i = j
		case "export":
			if prevDot || depth != 0 || next == nil {
				continue
			}
			switch {
			case next.tt == js.OpenBraceToken || is(next, "*"):
				j := i + 1
				if next.tt == js.OpenBraceToken {
					for ; j < len(tokens) && tokens[j].tt != js.CloseBraceToken; j++ {
					}
				}
				if n := at(j + 1); is(n, "from") || (next.tt != js.OpenBraceToken) {
					for ; j < len(tokens) && tokens[j].tt != js.StringToken; j++ {
					}
				}
				if j >= len(tokens) {
					return nil, false
				}
				end := tokens[j].end
				if n := at(j + 1); n != nil && n.tt == js.SemicolonToken {
					end = n.end
					j++
				}
				blank(t.start, end)
				i = j
			case is(next, "default"):
				if next.end-t.start != len(defaultPlaceholder) {
					// не «export default» через один пробел — просто убираем слова
					blank(t.start, next.end)
				} else {
					copy(out[t.start:next.end], defaultPlaceholder)
				}
				i++
			default:
				blank(t.start, t.end)
			}
		}
	}
	return out, true
}
