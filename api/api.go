package api

import (
	_ "embed"
)

//go:embed api.yaml
var OpenapiSpec []byte
