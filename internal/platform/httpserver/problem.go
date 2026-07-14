// Package httpserver holds cross-cutting HTTP helpers shared by every
// context's /adapters/http package, per docs/architecture/18-backend-structure.md §1.
package httpserver

import (
	"encoding/json"
	"net/http"
)

// Problem is the RFC 7807 application/problem+json shape every error
// response uses (docs/architecture/04-api-design.md §7).
type Problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
}

func WriteProblem(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(Problem{
		Type:   "about:blank",
		Title:  title,
		Status: status,
		Detail: detail,
	})
}
