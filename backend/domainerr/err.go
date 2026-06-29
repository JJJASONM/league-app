// Package domainerr provides a shared domain error type used by domain services.
// HTTP handlers map domainerr.Category to HTTP status codes.
// SQLite adapters must not import this package; they return plain wrapped errors.
package domainerr

import "errors"

// Category classifies a domain error for HTTP status mapping.
type Category int

const (
	NotFound     Category = iota
	InvalidInput          // reserved for future phases; unused in Phase A
	Internal
)

// Err is a domain error. Error() returns only the safe Message field so that
// an unguarded err.Error() call cannot expose infrastructure details to HTTP clients.
// The internal Cause is available through Unwrap() for logging and error-chain inspection.
type Err struct {
	Code     string
	Category Category
	Message  string // safe for HTTP response bodies
	Cause    error  // internal; never written to HTTP responses
}

// Error returns only the safe Message. Cause is intentionally omitted.
func (e *Err) Error() string { return e.Message }

// Unwrap returns the internal Cause for errors.As/Is chain traversal and logging.
func (e *Err) Unwrap() error { return e.Cause }

// New creates a domain error without a wrapped cause.
func New(code string, cat Category, msg string) *Err {
	return &Err{Code: code, Category: cat, Message: msg}
}

// Wrap creates a domain error wrapping an infrastructure cause.
func Wrap(code string, cat Category, msg string, cause error) *Err {
	return &Err{Code: code, Category: cat, Message: msg, Cause: cause}
}

// IsCategory reports whether err or any error in its chain is a *Err with the given category.
func IsCategory(err error, cat Category) bool {
	var de *Err
	return errors.As(err, &de) && de.Category == cat
}
