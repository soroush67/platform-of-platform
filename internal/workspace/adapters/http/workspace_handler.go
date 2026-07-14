package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"platform-of-platform/internal/platform/httpserver"
	"platform-of-platform/internal/workspace/application"
	"platform-of-platform/internal/workspace/domain"
)

type createWorkspaceRequest struct {
	Name            string  `json:"name"`
	ExecutionEngine string  `json:"execution_engine"`
	EnvironmentID   *string `json:"environment_id"`
}

type workspaceResponse struct {
	ID                    string  `json:"id"`
	OrganizationID        string  `json:"organization_id"`
	ProjectID             string  `json:"project_id"`
	EnvironmentID         *string `json:"environment_id,omitempty"`
	Name                  string  `json:"name"`
	ExecutionEngine       string  `json:"execution_engine"`
	Locked                bool    `json:"locked"`
	CreatedAt             string  `json:"created_at"`
}

func toWorkspaceResponse(w *domain.Workspace) workspaceResponse {
	return workspaceResponse{
		ID:              w.ID,
		OrganizationID:  w.OrganizationID,
		ProjectID:       w.ProjectID,
		EnvironmentID:   w.EnvironmentID,
		Name:            w.Name,
		ExecutionEngine: string(w.ExecutionEngine),
		Locked:          w.Locked,
		CreatedAt:       w.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func CreateWorkspaceHandler(svc *application.CreateWorkspaceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		orgID := r.PathValue("id")
		projectID := r.PathValue("projectID")

		var req createWorkspaceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		ws, err := svc.Execute(r.Context(), application.CreateWorkspaceInput{
			OrganizationID:   orgID,
			ProjectID:        projectID,
			RequestingUserID: userID,
			Name:             req.Name,
			ExecutionEngine:  domain.ExecutionEngine(req.ExecutionEngine),
			EnvironmentID:    req.EnvironmentID,
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
			httpserver.WriteProblem(w, http.StatusInternalServerError, "failed to create workspace", "")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(toWorkspaceResponse(ws))
	}
}

func ListWorkspacesHandler(svc *application.ListWorkspacesService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		orgID := r.PathValue("id")
		projectID := r.PathValue("projectID")

		workspaces, err := svc.Execute(r.Context(), orgID, projectID, userID)
		if err != nil {
			if writeNotFoundOrForbidden(w, err, "project not found") {
				return
			}
			httpserver.WriteProblem(w, http.StatusInternalServerError, "failed to list workspaces", "")
			return
		}

		responses := make([]workspaceResponse, 0, len(workspaces))
		for _, ws := range workspaces {
			responses = append(responses, toWorkspaceResponse(ws))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"data": responses})
	}
}

func GetWorkspaceHandler(svc *application.GetWorkspaceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		orgID := r.PathValue("id")
		projectID := r.PathValue("projectID")
		wsID := r.PathValue("workspaceID")

		ws, err := svc.Execute(r.Context(), orgID, projectID, wsID, userID)
		if err != nil {
			if writeNotFoundOrForbidden(w, err, "workspace not found") {
				return
			}
			httpserver.WriteProblem(w, http.StatusInternalServerError, "failed to fetch workspace", "")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(toWorkspaceResponse(ws))
	}
}
