package main

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"time"
)

// requestIDHeader is the header used both to accept an upstream-assigned
// request ID and to echo the ID (generated or upstream) back to the caller.
const requestIDHeader = "X-Request-Id"

// statusRecorder wraps http.ResponseWriter to capture the status code
// written, since http.ResponseWriter has no getter for it.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// withRequestLogging wraps next with request-ID assignment and basic access
// logging (method, path, status, duration, request ID). It never logs
// Authorization headers, other request headers, query strings, or request
// bodies. The request ID is read from the incoming X-Request-Id header when
// present (so an upstream proxy's ID is preserved), otherwise generated; it
// is always echoed back on the response header so a user-reported error can
// be matched to a server log line.
func withRequestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get(requestIDHeader)
		if reqID == "" {
			reqID = generateRequestID()
		}
		w.Header().Set(requestIDHeader, reqID)

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rec, r)
		duration := time.Since(start)

		log.Printf("request_id=%s method=%s path=%s status=%d duration=%s",
			reqID, r.Method, r.URL.Path, rec.status, duration)
	})
}

// generateRequestID returns a random 16-byte hex-encoded ID. Uses crypto/rand
// directly rather than adding a UUID dependency; a random 32-hex-char string
// is sufficient for correlating a request across logs.
func generateRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Extremely unlikely; fall back to a timestamp-based ID rather than
		// failing the request over an inability to generate a trace ID.
		return hex.EncodeToString([]byte(time.Now().Format("20060102150405.000000000")))
	}
	return hex.EncodeToString(b)
}
