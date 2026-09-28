package tool

import "errors"

var (
	ErrMissingBuildInfo   = errors.New("missing or unreadable build info")
	ErrMissingPackagePath = errors.New("missing main package path")
)
