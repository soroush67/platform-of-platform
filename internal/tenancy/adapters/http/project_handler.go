package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"platform-of-platform/internal/platform/httpserver"
	"platform-of-platform/internal/tenancy/application"
	"platform-of-platform/internal/tenancy/domain"
)

type createProjectRequest struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
}

type projectResponse struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	Name           string `json:"name"`
	Slug           string `json:"slug"`
	Description    string `json:"description"`
	CreatedAt      string `json:"created_at"`
}

func toProjectResponse(p *domain.Project) projectResponse {
	return projectResponse{
		ID:             p.ID,
		OrganizationID: p.OrganizationID,
		Name:           p.Name,
		Slug:           p.Slug,
		Description:    p.Description,
		CreatedAt:      p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// CreateProjectHandler implements POST /api/v1/orgs/{id}/projects.
// Registered behind httpserver.RequireAuth in main.go - the use case
// checks organization:manage, not this handler (same shape as
// AddMemberHandler).
func CreateProjectHandler(svc *application.CreateProjectService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		orgID := r.PathValue("id")

		var req createProjectRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		project, err := svc.Execute(r.Context(), application.CreateProjectInput{
			OrganizationID:   orgID,
			RequestingUserID: userID,
			Name:             req.Name,
			Slug:             req.Slug,
			Description:      req.Description,
		})
		if err != nil {
			var validationErr *domain.ValidationError
			if errors.As(err, &validationErr) {
				httpserver.WriteProblem(w, http.StatusBadRequest, "validation failed", validationErr.Error())
				return
			}
			if errors.Is(err, domain.ErrForbidden) {
				httpserver.WriteProblem(w, http.StatusForbidden, "forbidden", "requires organization:manage")
				return
			}
			if errors.Is(err, domain.ErrOrganizationNotFound) {
				httpserver.WriteProblem(w, http.StatusNotFound, "organization not found", "")
				return
			}
			httpserver.WriteProblem(w, http.StatusInternalServerError, "failed to create project", "")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(toProjectResponse(project))
	}
}

// ListProjectsHandler implements GET /api/v1/orgs/{id}/projects -
// membership-gated only, any role.
func ListProjectsHandler(svc *application.ListProjectsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		orgID := r.PathValue("id")

		projects, err := svc.Execute(r.Context(), orgID, userID)
		if err != nil {
			if errors.Is(err, domain.ErrOrganizationNotFound) {
				httpserver.WriteProblem(w, http.StatusNotFound, "organization not found", "")
				return
			}
			httpserver.WriteProblem(w, http.StatusInternalServerError, "failed to list projects", "")
			return
		}

		responses := make([]projectResponse, 0, len(projects))
		for _, p := range projects {
			responses = append(responses, toProjectResponse(p))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"data": responses})
	}
}

// GetProjectHandler implements GET /api/v1/orgs/{id}/projects/{projectID}.
func GetProjectHandler(svc *application.GetProjectService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		orgID := r.PathValue("id")
		projectID := r.PathValue("projectID")

		project, err := svc.Execute(r.Context(), orgID, projectID, userID)
		if err != nil {
			if errors.Is(err, domain.ErrOrganizationNotFound) || errors.Is(err, domain.ErrProjectNotFound) {
				httpserver.WriteProblem(w, http.StatusNotFound, "project not found", "")
				return
			}
			httpserver.WriteProblem(w, http.StatusInternalServerError, "failed to fetch project", "")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(toProjectResponse(project))
	}
}
