// Package jsscope — границы функций в сжатом JS и позиции их имён (ADR-011): по ним карта даёт
// настоящее имя функции вокруг места ошибки
package jsscope

import (
	"errors"
	"reflect"
	"sort"

	"github.com/dop251/goja/ast"
	"github.com/dop251/goja/parser"
)

// Name — имя в сжатом коде: позиция (байты от начала файла) и текст
type Name struct {
	Pos  int
	Text string
}

// Scope — функция: [Start, End) в байтах; Name — её имя или имя, к которому она привязана
// (const x = () => …, x.y = function…, ключ свойства или метода); Owner — класс или объект-владелец
type Scope struct {
	Start int
	End   int
	Name  *Name
	Owner *Name
}

// обёртка: await на верхнем уровне модуля разбирается только внутри async-функции
const wrapPrefix = "async function _z(){"

// Parse — функции файла, отсортированные по началу
func Parse(src []byte) ([]Scope, error) {
	prepared, ok := demodule(src)
	if !ok {
		return nil, errors.New("jsscope: lexer failed")
	}
	code := wrapPrefix + string(prepared) + "\n}"
	prog, err := parser.ParseFile(nil, "", code, 0, parser.WithDisableSourceMaps)
	if err != nil {
		return nil, err
	}

	w := &walker{seen: map[uintptr]bool{}, shift: len(wrapPrefix) + 1, names: map[ast.Node]*Name{}, owners: map[ast.Node]*Name{}}
	w.walk(reflect.ValueOf(prog))

	sort.Slice(w.scopes, func(i, j int) bool { return w.scopes[i].Start < w.scopes[j].Start })
	return w.scopes, nil
}

// Innermost — самая внутренняя функция, содержащая позицию off; нет — nil
func Innermost(scopes []Scope, off int) *Scope {
	// функции не пересекаются частично: подходящая с наибольшим Start — самая внутренняя
	i := sort.Search(len(scopes), func(i int) bool { return scopes[i].Start > off })
	for j := i - 1; j >= 0; j-- {
		if scopes[j].Start <= off && off < scopes[j].End {
			return &scopes[j]
		}
	}
	return nil
}

type walker struct {
	seen   map[uintptr]bool // узел может встретиться дважды (DeclarationList функции)
	shift  int              // Idx goja — с 1 и с учётом обёртки
	names  map[ast.Node]*Name
	owners map[ast.Node]*Name
	scopes []Scope
}

var nodeType = reflect.TypeOf((*ast.Node)(nil)).Elem()

func (w *walker) pos(idx int) int { return idx - w.shift }

func (w *walker) ident(id *ast.Identifier) *Name {
	if id == nil {
		return nil
	}
	return &Name{Pos: w.pos(int(id.Idx)), Text: id.Name.String()}
}

// keyName — ключ свойства, метода или поля класса (не вычисляемый)
func (w *walker) keyName(key ast.Expression) *Name {
	switch k := key.(type) {
	case *ast.StringLiteral:
		return &Name{Pos: w.pos(int(k.Idx)), Text: k.Value.String()}
	case *ast.Identifier:
		return w.ident(k)
	case *ast.PrivateIdentifier:
		return &Name{Pos: w.pos(int(k.Idx)), Text: "#" + k.Name.String()}
	}
	return nil
}

func isFunc(n ast.Node) bool {
	switch n.(type) {
	case *ast.FunctionLiteral, *ast.ArrowFunctionLiteral:
		return true
	}
	return false
}

// hint — имена и владельцы для функций-потомков: записываются до обхода детей
func (w *walker) hint(n ast.Node) {
	switch v := n.(type) {
	case *ast.Binding:
		if id, ok := v.Target.(*ast.Identifier); ok && v.Initializer != nil {
			name := w.ident(id)
			if name.Text == DefaultPlaceholder {
				return
			}
			if isFunc(v.Initializer) {
				w.names[v.Initializer] = name
			}
			if obj, ok := v.Initializer.(*ast.ObjectLiteral); ok {
				w.owners[obj] = name
			}
			if cls, ok := v.Initializer.(*ast.ClassLiteral); ok && cls.Name == nil {
				w.owners[cls] = name
			}
		}
	case *ast.AssignExpression:
		if v.ParenthesizedLeft || !isFunc(v.Right) {
			return
		}
		switch left := v.Left.(type) {
		case *ast.Identifier:
			w.names[v.Right] = w.ident(left)
		case *ast.DotExpression:
			w.names[v.Right] = &Name{Pos: w.pos(int(left.Identifier.Idx)), Text: left.Identifier.Name.String()}
		}
	case *ast.ObjectLiteral:
		owner := w.owners[v]
		for _, p := range v.Value {
			if pk, ok := p.(*ast.PropertyKeyed); ok && !pk.Computed && isFunc(pk.Value) {
				if name := w.keyName(pk.Key); name != nil {
					w.names[pk.Value] = name
					if owner != nil {
						w.owners[pk.Value] = owner
					}
				}
			}
		}
	case *ast.ClassLiteral:
		owner := w.ident(v.Name)
		if owner == nil {
			owner = w.owners[v]
		}
		for _, el := range v.Body {
			switch m := el.(type) {
			case *ast.MethodDefinition:
				if m.Body != nil && !m.Computed {
					if name := w.keyName(m.Key); name != nil {
						w.names[m.Body] = name
					}
					if owner != nil {
						w.owners[m.Body] = owner
					}
				}
			case *ast.FieldDefinition:
				if m.Initializer != nil && !m.Computed && isFunc(m.Initializer) {
					if name := w.keyName(m.Key); name != nil {
						w.names[m.Initializer] = name
					}
					if owner != nil {
						w.owners[m.Initializer] = owner
					}
				}
			}
		}
	}
}

func (w *walker) visit(n ast.Node) {
	w.hint(n)
	switch v := n.(type) {
	case *ast.FunctionLiteral:
		s := Scope{Start: w.pos(int(v.Idx0())), End: w.pos(int(v.Idx1())), Name: w.ident(v.Name), Owner: w.owners[n]}
		if s.Name == nil {
			s.Name = w.names[n]
		}
		if s.Start >= 0 {
			w.scopes = append(w.scopes, s)
		}
	case *ast.ArrowFunctionLiteral:
		s := Scope{Start: w.pos(int(v.Idx0())), End: w.pos(int(v.Idx1())), Name: w.names[n], Owner: w.owners[n]}
		if s.Start >= 0 {
			w.scopes = append(w.scopes, s)
		}
	}
}

// walk — обход дерева goja по reflect (своего обходчика в goja нет); узлы — в прямом порядке
func (w *walker) walk(v reflect.Value) {
	switch v.Kind() {
	case reflect.Interface:
		if !v.IsNil() {
			w.walk(v.Elem())
		}
	case reflect.Pointer:
		if v.IsNil() || w.seen[v.Pointer()] {
			return
		}
		w.seen[v.Pointer()] = true
		if v.Type().Implements(nodeType) {
			w.visit(v.Interface().(ast.Node))
		}
		w.walk(v.Elem())
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				w.walk(v.Field(i))
			}
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			w.walk(v.Index(i))
		}
	}
}
