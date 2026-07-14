// Package httpserver holds cross-cutting HTTP helpers shared by every
// context's /adapters/http package, per docs/architecture/18-backend-structure.md §1.
package httpserver

import (
	"encoding/json"
	"net/http"
)

// Problem is the RFC 7807 application/problem+json shape every error
// response uses (docs/architecture/04-api-design.md §7). RequestID is
// only populated for 5xx responses (see WriteProblem below) - a 400/403/
// 404 is a normal, expected outcome of a bad or unauthorized request,
// nothing to correlate against server-side logs for; a 5xx is exactly
// the case an operator needs "which request was this" to go find in
// logs/traces.
type Problem struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Detail    string `json:"detail,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// WriteProblem reads the request id straight back off the
// ResponseWriter's own headers - RequestID (request_id.go) sets it
// there before any handler runs, so this needs no context/request
// parameter and none of its ~25 existing call sites across every
// context's HTTP adapter had to change.
func WriteProblem(w http.ResponseWriter, status int, title, detail string) {
	p := Problem{
		Type:   "about:blank",
		Title:  title,
		Status: status,
		Detail: detail,
	}
	if status >= 500 {
		p.RequestID = w.Header().Get("X-Request-ID")
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(p)
}
