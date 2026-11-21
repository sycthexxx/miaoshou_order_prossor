package browser

import (
	"context"
	"net/url"
)

type Target struct {
	API     *url.URL
	Desktop *url.URL
	Token   string
}
type Runtime interface {
	Resolve(context.Context, string, bool) (Target, error)
}

// RuntimeError is a safe, actionable message authored by the runtime, never
// raw Docker output, filesystem errors, credentials, or remote response bodies.
type RuntimeError struct {
	Code    string
	Message string
}

func (e *RuntimeError) Error() string         { return e.Message }
func runtimeError(code, message string) error { return &RuntimeError{Code: code, Message: message} }
