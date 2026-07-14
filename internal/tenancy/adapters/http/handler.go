// Package http is the Tenancy context's REST adapter - parses the
// request, calls an /application use case, maps the result to a response
// DTO. No business logic lives here (docs/architecture/18-backend-structure.md §2).
package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"platform-of-platform/internal/platform/httpserver"
	"platform-of-platform/internal/tenancy/application"
	"platform-of-platform/internal/tenancy/domain"
)

type createOrganizationRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type organizationResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	CreatedAt string `json:"created_at"`
}

// CreateOrganizationHandler implements POST /api/v1/orgs
// (docs/architecture/04-api-design.md §1). Deliberately unauthenticated
// for now - see the use case's own comment on why, and the
// bootstrap-only "how does the very first org ever get created" framing
// in docs/architecture/21-deployment.md §4 step 3.
func CreateOrganizationHandler(svc *application.CreateOrganizationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createOrganizationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		org, err := svc.Execute(r.Context(), application.CreateOrganizationInput{
			Name: req.Name,
			Slug: req.Slug,
		})
		if err != nil {
			var validationErr *domain.ValidationError
			if errors.As(err, &validationErr) {
				httpserver.WriteProblem(w, http.StatusBadRequest, "validation failed", validationErr.Error())
				return
			}
			httpserver.WriteProblem(w, http.StatusInternalServerError, "failed to create organization", "")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(organizationResponse{
			ID:        org.ID,
			Name:      org.Name,
			Slug:      org.Slug,
			CreatedAt: org.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
}
