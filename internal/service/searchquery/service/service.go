package service

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/rendau/zorko/internal/errs"
	searchqueryModel "github.com/rendau/zorko/internal/service/searchquery/model"
)

const (
	maxConds    = 20
	maxWords    = 10
	maxValueLen = 200
)

// ignoredKeys — привычные пользователям Sentry ключи, которым в zorko пока нечего соответствовать:
// `is:unresolved` и т. п. (статусы — шаг 4) не сужают выдачу и не дают ошибку
var ignoredKeys = map[string]bool{"is": true}

type Service struct{}

func New() *Service {
	return &Service{}
}

func (s *Service) Parse(q string) (*searchqueryModel.Query, error) {
	result := &searchqueryModel.Query{}
	runes := []rune(q)

	for i := 0; i < len(runes); {
		if unicode.IsSpace(runes[i]) {
			i++
			continue
		}

		start := i
		negate := false
		if runes[i] == '!' {
			negate = true
			i++
		}

		// ключ — до двоеточия, если до него только допустимые символы
		keyEnd := i
		for keyEnd < len(runes) && isKeyRune(runes[keyEnd]) {
			keyEnd++
		}

		// `https://…` — адрес, а не ключ https
		isUrl := keyEnd+2 < len(runes) && runes[keyEnd+1] == '/' && runes[keyEnd+2] == '/'

		if keyEnd > i && keyEnd < len(runes) && runes[keyEnd] == ':' && !isUrl {
			key := strings.ToLower(string(runes[i:keyEnd]))
			value, next, err := readValue(runes, keyEnd+1)
			if err != nil {
				return nil, err
			}
			if value == "" {
				return nil, invalid("empty value", start)
			}
			if len(value) > maxValueLen {
				return nil, invalid("value too long", start)
			}
			i = next

			if ignoredKeys[key] {
				continue
			}
			result.Conds = append(result.Conds, buildCond(key, value, negate))
			if len(result.Conds) > maxConds {
				return nil, invalid("too many conditions", start)
			}
			continue
		}

		// слово (или фраза в кавычках)
		i = start
		word, next, err := readValue(runes, i)
		if err != nil {
			return nil, err
		}
		i = next
		if word = strings.TrimSpace(word); word != "" {
			result.Words = append(result.Words, word)
			if len(result.Words) > maxWords {
				return nil, invalid("too many words", start)
			}
		}
	}

	return result, nil
}

// readValue — значение с позиции i: "в кавычках" (с \" внутри) или до пробела
func readValue(runes []rune, i int) (string, int, error) {
	if i < len(runes) && runes[i] == '"' {
		var b strings.Builder
		for j := i + 1; j < len(runes); j++ {
			switch {
			case runes[j] == '\\' && j+1 < len(runes):
				j++
				b.WriteRune(runes[j])
			case runes[j] == '"':
				return b.String(), j + 1, nil
			default:
				b.WriteRune(runes[j])
			}
		}
		return "", 0, invalid("unclosed quote", i)
	}

	j := i
	for j < len(runes) && !unicode.IsSpace(runes[j]) {
		j++
	}
	return string(runes[i:j]), j, nil
}

// buildCond: `*` в значении — «содержит / начинается / кончается» (LIKE), иначе точное равенство
func buildCond(key, value string, negate bool) searchqueryModel.Cond {
	if !strings.Contains(value, "*") {
		return searchqueryModel.Cond{Key: key, Op: map[bool]searchqueryModel.Op{false: searchqueryModel.OpEq, true: searchqueryModel.OpNe}[negate], Value: value}
	}

	parts := strings.Split(value, "*")
	for i, p := range parts {
		parts[i] = escapeLike(p)
	}
	return searchqueryModel.Cond{
		Key:   key,
		Op:    map[bool]searchqueryModel.Op{false: searchqueryModel.OpLike, true: searchqueryModel.OpNotLike}[negate],
		Value: strings.Join(parts, "%"),
	}
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func isKeyRune(r rune) bool {
	return r == '.' || r == '_' || r == '-' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

func invalid(desc string, pos int) error {
	return errs.ErrFull{
		Err:    errs.InvalidQuery,
		Desc:   desc + " at position " + strconv.Itoa(pos+1),
		Fields: map[string]string{"q": strconv.Itoa(pos + 1)},
	}
}
