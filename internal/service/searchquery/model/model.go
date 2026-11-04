package model

type Op string

const (
	OpEq      Op = "eq"       // key:value
	OpNe      Op = "ne"       // !key:value
	OpLike    Op = "like"     // key:*часть* — Value уже шаблон LIKE
	OpNotLike Op = "not_like" // !key:*часть*
)

// Cond — условие на тег события
type Cond struct {
	Key   string
	Op    Op
	Value string
}

// Query — разобранная строка: условия на теги и слова для поиска по заголовку и месту падения
type Query struct {
	Conds []Cond
	Words []string
}
