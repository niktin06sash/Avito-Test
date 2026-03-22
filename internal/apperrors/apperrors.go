package apperrors

import "errors"

var (
	ErrFooNotFound     = errors.New("not found")
	ErrFooConflict     = errors.New("conflict")
	ErrFooForbidden    = errors.New("forbidden")
	ErrFooBadRequest   = errors.New("bad request")
	ErrFooUnauthorized = errors.New("unauthorized")
)
