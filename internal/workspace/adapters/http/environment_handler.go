// Package http is the Workspace & Environment context's REST adapter -
// same "parse, call a use case, map the result" rule as every other
// context's adapters/http package (docs/architecture/18-backend-structure.md §2).
package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"platform-of-platform/internal/platform/httpserver"
	"platform-of-platform/internal/workspace/application"
	"platform-of-platform/internal/workspace/domain"
)

type createEnvironmentRequest struct {
	Name             string `json:"name"`
	PromotionRank    int    `json:"promotion_rank"`
	RequiresApproval bool   `json:"requires_approval"`
}

type environmentResponse struct {
	ID               string `json:"id"`
	OrganizationID   string `json:"organization_id"`
	ProjectID        string `json:"project_id"`
	Name             string `json:"name"`
	PromotionRank    int    `json:"promotion_rank"`
	RequiresApproval bool   `json:"requires_approval"`
	CreatedAt        string `json:"created_at"`
}

func toEnvironmentResponse(e *domain.Environment) environmentResponse {
	return environmentResponse{
		ID:               e.ID,
		OrganizationID:   e.OrganizationID,
		ProjectID:        e.ProjectID,
		Name:             e.Name,
		PromotionRank:    e.PromotionRank,
		RequiresApproval: e.RequiresApproval,
		CreatedAt:        e.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// writeNotFoundOrForbidden maps this context's own sentinel errors -
// shared by every handler in this package, same errors regardless of
// which aggregate (Environment or Workspace) raised them.
func writeNotFoundOrForbidden(w http.ResponseWriter, err error, notFoundTitle string) bool {
	if errors.Is(err, domain.ErrForbidden) {
		httpserver.WriteProblem(w, http.StatusForbidden, "forbidden", "")
		return true
	}
	if errors.Is(err, domain.ErrProjectNotFound) || errors.Is(err, domain.ErrEnvironmentNotFound) || errors.Is(err, domain.ErrWorkspaceNotFound) {
		httpserver.WriteProblem(w, http.StatusNotFound, notFoundTitle, "")
		return true
	}
	return false
}

func CreateEnvironmentHandler(svc *application.CreateEnvironmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		orgID := r.PathValue("id")
		projectID := r.PathValue("projectID")

		var req createEnvironmentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		env, err := svc.Execute(r.Context(), application.CreateEnvironmentInput{
			OrganizationID:   orgID,
			ProjectID:        projectID,
			RequestingUserID: userID,
			Name:             req.Name,
			PromotionRank:    req.PromotionRank,
			RequiresApproval: req.RequiresApproval,
		})
		if err != nil {
			var validationErr *domain.ValidationError
			if errors.As(err, &validationErr) {
				httpserver.WriteProblem(w, http.StatusBadRequest, "validation failed", validationErr.Error())
				return
			}
			if writeNotFoundOrForbidden(w, err, "project not found") {
				return
			}
			httpserver.WriteProblem(w, http.StatusInternalServerError, "failed to create environment", "")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(toEnvironmentResponse(env))
	}
}

func ListEnvironmentsHandler(svc *application.ListEnvironmentsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		orgID := r.PathValue("id")
		projectID := r.PathValue("projectID")

		envs, err := svc.Execute(r.Context(), orgID, projectID, userID)
		if err != nil {
			if writeNotFoundOrForbidden(w, err, "project not found") {
				return
			}
			httpserver.WriteProblem(w, http.StatusInternalServerError, "failed to list environments", "")
			return
		}

		responses := make([]environmentResponse, 0, len(envs))
		for _, e := range envs {
			responses = append(responses, toEnvironmentResponse(e))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"data": responses})
	}
}

func GetEnvironmentHandler(svc *application.GetEnvironmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		orgID := r.PathValue("id")
		projectID := r.PathValue("projectID")
		envID := r.PathValue("envID")

		env, err := svc.Execute(r.Context(), orgID, projectID, envID, userID)
		if err != nil {
			if writeNotFoundOrForbidden(w, err, "environment not found") {
				return
			}
			httpserver.WriteProblem(w, http.StatusInternalServerError, "failed to fetch environment", "")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(toEnvironmentResponse(env))
	}
}
