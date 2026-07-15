package http

import (
	"encoding/json"
	"net/http"

	"platform-of-platform/internal/platform/httpserver"
	"platform-of-platform/internal/tenancy/application"
)

// ListMyOrganizationsHandler implements GET /api/v1/orgs - list-wrapped
// {"data": [...]}, same envelope shape as every other list endpoint in
// this codebase (e.g. ListProjectsHandler in project_handler.go). Reuses
// organizationResponse/toOrganizationResponse (handler.go) unchanged.
func ListMyOrganizationsHandler(svc *application.ListMyOrganizationsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		orgs, err := svc.Execute(r.Context(), userID)
		if err != nil {
			httpserver.WriteProblem(w, http.StatusInternalServerError, "failed to list organizations", "")
			return
		}

		responses := make([]organizationResponse, 0, len(orgs))
		for _, o := range orgs {
			responses = append(responses, toOrganizationResponse(o))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"data": responses})
	}
}
