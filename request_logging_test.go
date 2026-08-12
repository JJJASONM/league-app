package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithRequestLogging_NoIncomingHeader_GeneratesID(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	wrapped := withRequestLogging(inner)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	got := w.Header().Get(requestIDHeader)
	if got == "" {
		t.Fatal("want a generated X-Request-Id response header, got empty")
	}
}

func TestWithRequestLogging_IncomingHeader_IsEchoedBack(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	wrapped := withRequestLogging(inner)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set(requestIDHeader, "caller-supplied-id-123")
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if got := w.Header().Get(requestIDHeader); got != "caller-supplied-id-123" {
		t.Errorf("want upstream request ID echoed back, got %q", got)
	}
}

func TestWithRequestLogging_PassesThroughStatusCode(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	wrapped := withRequestLogging(inner)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("want wrapped handler's status code to pass through, got %d", w.Code)
	}
}

func TestGenerateRequestID_ReturnsDistinctValues(t *testing.T) {
	a := generateRequestID()
	b := generateRequestID()
	if a == "" || b == "" {
		t.Fatal("want non-empty request IDs")
	}
	if a == b {
		t.Errorf("want distinct IDs across calls, got %q twice", a)
	}
}
