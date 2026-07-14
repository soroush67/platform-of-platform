// Package http is the Audit context's REST adapter - read-only by
// construction, matching the aggregate's own append-only, no-write-API
// nature (see the postgres adapter's own comment).
package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"platform-of-platform/internal/audit/application"
	"platform-of-platform/internal/audit/domain"
	"platform-of-platform/internal/platform/httpserver"
)

type auditEntryResponse struct {
	ID         string         `json:"id"`
	Actor      string         `json:"actor"`
	Action     string         `json:"action"`
	TargetType string         `json:"target_type"`
	TargetID   string         `json:"target_id"`
	Metadata   map[string]any `json:"metadata"`
	CreatedAt  string         `json:"created_at"`
}

func toAuditEntryResponse(e *domain.Entry) auditEntryResponse {
	return auditEntryResponse{
		ID:         e.ID,
		Actor:      e.Actor,
		Action:     e.Action,
		TargetType: e.TargetType,
		TargetID:   e.TargetID,
		Metadata:   e.Metadata,
		CreatedAt:  e.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// ListAuditLogHandler implements GET /api/v1/orgs/{id}/audit-log.
func ListAuditLogHandler(svc *application.ListAuditEntriesService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		entries, err := svc.Execute(r.Context(), r.PathValue("id"), userID)
		if err != nil {
			if errors.Is(err, domain.ErrForbidden) {
				httpserver.WriteProblem(w, http.StatusForbidden, "forbidden", "")
				return
			}
			httpserver.WriteProblem(w, http.StatusInternalServerError, "failed to fetch audit log", "")
			return
		}

		responses := make([]auditEntryResponse, 0, len(entries))
		for _, e := range entries {
			responses = append(responses, toAuditEntryResponse(e))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"data": responses})
	}
}
