package errs

import "errors"

type Err string

func (e Err) Error() string {
	return string(e)
}

// common errors
const (
	ServiceNA         = Err("service_not_available")
	NotImplemented    = Err("not_implemented")
	InvalidConfig     = Err("invalid_config")
	NoPermission      = Err("no_permission")
	ObjectNotFound    = Err("object_not_found")
	NoRows            = Err("err_no_rows")
	NotAuthorized     = Err("not_authorized")
	InvalidRequest    = Err("invalid_request")
	IncorrectPageSize = Err("incorrect_page_size")
	IdRequired        = Err("id_required")
)

// zorko errors
const (
	SlugExists      = Err("slug_exists")
	OrgNotFound     = Err("org_not_found")
	ProjectNotFound = Err("project_not_found")

	UsrUsernameExists = Err("usr_username_exists")
	LoginRateLimited  = Err("login_rate_limited")
	UsrSelfChange     = Err("usr_self_change")
	InvalidQuery      = Err("invalid_query")

	IngestKeyInvalid   = Err("ingest_key_invalid")
	IngestOriginDenied = Err("ingest_origin_denied")

	// карты исходников (шаг 3)
	TokenInvalid       = Err("token_invalid")
	SourcemapsDisabled = Err("sourcemaps_disabled")
	ChunkInvalid       = Err("chunk_invalid")
	BundleInvalid      = Err("bundle_invalid")
	ReleaseNotFound    = Err("release_not_found")

	// запись сессий
	ReplayDisabled = Err("replay_disabled") // S3 не настроен: запись не включить
	ReplayNotFound = Err("replay_not_found")
)

type ErrFull struct {
	Err    error
	Desc   string
	Fields map[string]string
}

func (e ErrFull) Error() string {
	return e.Err.Error() + ", desc: " + e.Desc
}

// AsErr extracts an Err from the error chain regardless of whether it was
// returned as a value (Err) or a pointer (*Err).
func AsErr(err error) (Err, bool) {
	if e, ok := errors.AsType[Err](err); ok {
		return e, true
	}
	if e, ok := errors.AsType[*Err](err); ok && e != nil {
		return *e, true
	}
	return "", false
}

// AsErrFull extracts an ErrFull from the error chain regardless of whether it
// was returned as a value (ErrFull) or a pointer (*ErrFull).
func AsErrFull(err error) (ErrFull, bool) {
	if e, ok := errors.AsType[ErrFull](err); ok {
		return e, true
	}
	if e, ok := errors.AsType[*ErrFull](err); ok && e != nil {
		return *e, true
	}
	return ErrFull{}, false
}

// Code — код ошибки ответа для метрик и логов: errs.Err сервиса (в том числе внутри ErrFull), для
// прочих ошибок — service_not_available. Набор кодов конечен — годится в лейбл метрики.
func Code(err error) string {
	if fullErr, ok := AsErrFull(err); ok {
		err = fullErr.Err
	}
	if baseErr, ok := AsErr(err); ok {
		return baseErr.Error()
	}
	return ServiceNA.Error()
}

// IsFailure — код значит сбой сервиса (упала зависимость, неожиданная ошибка, не настроен), а не
// отказ по делу (не найдено, неверный запрос, нет прав). По нему метрики отделяют сбои от
// отказов по делу (метка code в request_total).
func IsFailure(code string) bool {
	switch Err(code) {
	case ServiceNA, NotImplemented, InvalidConfig:
		return true
	}
	return false
}
