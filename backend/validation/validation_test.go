package validation_test

import (
	"encoding/json"
	"league_app/backend/validation"
	"testing"
)

func TestResultEmpty(t *testing.T) {
	var r validation.Result
	if r.HasErrors() {
		t.Error("empty result should have no errors")
	}
	if !r.IsValid() {
		t.Error("empty result should be valid")
	}
	if len(r.Errors()) != 0 {
		t.Errorf("expected 0 errors, got %d", len(r.Errors()))
	}
	if len(r.Warnings()) != 0 {
		t.Errorf("expected 0 warnings, got %d", len(r.Warnings()))
	}
}

func TestAddError(t *testing.T) {
	var r validation.Result
	r.AddError("CODE_A", "field_a", "something wrong")
	if !r.HasErrors() {
		t.Error("expected HasErrors = true")
	}
	if r.IsValid() {
		t.Error("expected IsValid = false")
	}
	if len(r.Errors()) != 1 {
		t.Errorf("expected 1 error, got %d", len(r.Errors()))
	}
	if len(r.Warnings()) != 0 {
		t.Errorf("expected 0 warnings, got %d", len(r.Warnings()))
	}
	e := r.Errors()[0]
	if e.Code != "CODE_A" || e.Field != "field_a" || e.Level != validation.LevelError {
		t.Errorf("unexpected error fields: %+v", e)
	}
}

func TestAddWarning(t *testing.T) {
	var r validation.Result
	r.AddWarning("WARN_A", "", "heads-up")
	if r.HasErrors() {
		t.Error("warning alone should not trigger HasErrors")
	}
	if !r.IsValid() {
		t.Error("warning alone should not invalidate")
	}
	if len(r.Warnings()) != 1 {
		t.Errorf("expected 1 warning, got %d", len(r.Warnings()))
	}
	if len(r.Errors()) != 0 {
		t.Errorf("expected 0 errors, got %d", len(r.Errors()))
	}
	w := r.Warnings()[0]
	if w.Code != "WARN_A" || w.Level != validation.LevelWarning {
		t.Errorf("unexpected warning fields: %+v", w)
	}
}

func TestMixedMessages(t *testing.T) {
	var r validation.Result
	r.AddWarning("W", "", "w")
	r.AddError("E", "f", "e")
	if !r.HasErrors() {
		t.Error("expected HasErrors = true with a mix of messages")
	}
	if len(r.Messages) != 2 {
		t.Errorf("expected 2 total messages, got %d", len(r.Messages))
	}
	if len(r.Errors()) != 1 {
		t.Errorf("expected 1 error, got %d", len(r.Errors()))
	}
	if len(r.Warnings()) != 1 {
		t.Errorf("expected 1 warning, got %d", len(r.Warnings()))
	}
}

func TestHasWarnings_Empty(t *testing.T) {
	var r validation.Result
	if r.HasWarnings() {
		t.Error("empty result should have no warnings")
	}
}

func TestHasWarnings_WarningOnly(t *testing.T) {
	var r validation.Result
	r.AddWarning("W", "", "w")
	if !r.HasWarnings() {
		t.Error("expected HasWarnings = true")
	}
	if !r.IsValid() {
		t.Error("warning alone must not affect IsValid")
	}
}

func TestHasWarnings_Mixed(t *testing.T) {
	var r validation.Result
	r.AddError("E", "f", "e")
	r.AddWarning("W", "", "w")
	if !r.HasWarnings() {
		t.Error("expected HasWarnings = true when both error and warning present")
	}
	if !r.HasErrors() {
		t.Error("expected HasErrors = true")
	}
}

func TestJSONSerialization(t *testing.T) {
	var r validation.Result
	r.AddError("ERR_CODE", "field_a", "something is wrong")
	r.AddWarning("WARN_CODE", "", "heads-up")

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var out struct {
		Messages []struct {
			Code    string `json:"code"`
			Field   string `json:"field"`
			Message string `json:"message"`
			Level   string `json:"level"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if len(out.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(out.Messages))
	}

	// Error message
	e := out.Messages[0]
	if e.Code != "ERR_CODE" {
		t.Errorf("error code: got %q, want %q", e.Code, "ERR_CODE")
	}
	if e.Field != "field_a" {
		t.Errorf("error field: got %q, want %q", e.Field, "field_a")
	}
	if e.Level != "error" {
		t.Errorf("error level: got %q, want %q", e.Level, "error")
	}

	// Warning message
	w := out.Messages[1]
	if w.Code != "WARN_CODE" {
		t.Errorf("warning code: got %q, want %q", w.Code, "WARN_CODE")
	}
	if w.Field != "" {
		t.Errorf("warning field should be omitted/empty, got %q", w.Field)
	}
	if w.Level != "warning" {
		t.Errorf("warning level: got %q, want %q", w.Level, "warning")
	}
}
