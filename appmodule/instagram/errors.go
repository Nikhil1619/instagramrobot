package instagram

import "errors"

var (
	ErrAllMethodsFailed   = errors.New("all methods failed")
	ErrGQLJSONNotFound    = errors.New("failed to find JSON in response")
	ErrGQLContextNotFound = errors.New("failed to find context in response")
	ErrGQLContextMismatch = errors.New("context is not a string")
	ErrGQLNilResponse     = errors.New("GQL data is nil")
	ErrGQLNilMedia        = errors.New("GQL media is nil")
)