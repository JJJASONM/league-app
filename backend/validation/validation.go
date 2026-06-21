// Package validation provides shared result and message types for backend domain validators.
// Any domain can use these types to return structured findings to callers or HTTP handlers.
//
// Usage:
//
//	var result validation.Result
//	result.AddError("MY_CODE", "field_name", "Human-readable message")
//	result.AddWarning("MY_WARN", "", "Non-blocking note")
//	if result.HasErrors() { /* block the operation */ }
package validation

// Level is the severity of a validation message.
type Level string

const (
	LevelError   Level = "error"
	LevelWarning Level = "warning"
)

// Message is a single validation finding.
// Code is stable and machine-readable for API consumers.
// Field maps to a UI input name when present; empty means a non-field-specific message.
type Message struct {
	Code    string `json:"code"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
	Level   Level  `json:"level"`
}

// Result collects validation messages from a single validation pass.
// It is JSON-serialisable; handlers may encode it directly on 422 responses.
type Result struct {
	Messages []Message `json:"messages"`
}

// AddError appends an error-level message. Errors mark the operation as invalid.
func (r *Result) AddError(code, field, message string) {
	r.Messages = append(r.Messages, Message{Code: code, Field: field, Message: message, Level: LevelError})
}

// AddWarning appends a warning-level message. Warnings do not invalidate the operation.
func (r *Result) AddWarning(code, field, message string) {
	r.Messages = append(r.Messages, Message{Code: code, Field: field, Message: message, Level: LevelWarning})
}

// HasErrors reports whether any error-level messages are present.
func (r *Result) HasErrors() bool {
	for _, m := range r.Messages {
		if m.Level == LevelError {
			return true
		}
	}
	return false
}

// IsValid reports whether no error-level messages are present.
func (r *Result) IsValid() bool { return !r.HasErrors() }

// HasWarnings reports whether any warning-level messages are present.
// Warnings do not affect IsValid.
func (r *Result) HasWarnings() bool {
	for _, m := range r.Messages {
		if m.Level == LevelWarning {
			return true
		}
	}
	return false
}

// Errors returns only error-level messages in insertion order.
func (r *Result) Errors() []Message {
	return r.byLevel(LevelError)
}

// Warnings returns only warning-level messages in insertion order.
func (r *Result) Warnings() []Message {
	return r.byLevel(LevelWarning)
}

func (r *Result) byLevel(l Level) []Message {
	var out []Message
	for _, m := range r.Messages {
		if m.Level == l {
			out = append(out, m)
		}
	}
	return out
}
