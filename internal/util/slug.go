package util

import (
	"regexp"

	"github.com/rendau/zorko/internal/errs"
)

var slugRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,48}[a-z0-9])?$`)

// ValidateSlug — slug: латиница в нижнем регистре, цифры и дефис, до 50 символов, без дефиса по краям.
func ValidateSlug(slug *string, required bool) error {
	if slug == nil {
		if required {
			return errs.ErrFull{Err: errs.InvalidRequest, Desc: "slug is required", Fields: map[string]string{"slug": "required"}}
		}
		return nil
	}
	if !slugRe.MatchString(*slug) {
		return errs.ErrFull{Err: errs.InvalidRequest, Desc: "slug: a-z, 0-9 and '-', up to 50 chars", Fields: map[string]string{"slug": "invalid"}}
	}
	return nil
}
