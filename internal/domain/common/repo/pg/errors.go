package common

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// IsUniqueViolation — ошибка нарушения уникальности (23505); constraint — имя ограничения или "" для любого.
func IsUniqueViolation(err error, constraint string) bool {
	return isPgError(err, "23505", constraint)
}

// IsForeignKeyViolation — ошибка ссылки на несуществующую запись (23503); constraint — имя или "" для любого.
func IsForeignKeyViolation(err error, constraint string) bool {
	return isPgError(err, "23503", constraint)
}

func isPgError(err error, code, constraint string) bool {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	if !ok {
		return false
	}
	return pgErr.Code == code && (constraint == "" || pgErr.ConstraintName == constraint)
}
