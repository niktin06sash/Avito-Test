package apperrors

import "errors"

var (
	NotFound     = errors.New("not found")
	Conflict     = errors.New("conflict")
	Forbidden    = errors.New("forbidden")
	BadRequest   = errors.New("bad request")
	Unauthorized = errors.New("unauthorized")
)
