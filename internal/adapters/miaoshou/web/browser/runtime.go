package browser

import (
	"context"
	"fmt"
	"net/url"
)

type Target struct {
	API     *url.URL
	Desktop *url.URL
	Token   string
}
type Runtime interface {
	Resolve(context.Context, string, bool) (Target, error)
	Remote() bool
}
type Local struct {
	Port  int
	Token string
}

func (l Local) Remote() bool { return false }
func (l Local) Resolve(_ context.Context, _ string, _ bool) (Target, error) {
	u, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", l.Port))
	return Target{API: u, Token: l.Token}, nil
}

// RuntimeError is a safe, actionable message authored by the runtime, never
// raw Docker output, filesystem errors, credentials, or remote response bodies.
type RuntimeError struct {
	Code    string
	Message string
}

func (e *RuntimeError) Error() string         { return e.Message }
func runtimeError(code, message string) error { return &RuntimeError{Code: code, Message: message} }
