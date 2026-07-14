// Package http is the Execution context's REST adapter.
package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"platform-of-platform/internal/execution/application"
	"platform-of-platform/internal/execution/domain"
	"platform-of-platform/internal/platform/httpserver"
)

type runResponse struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organization_id"`
	WorkspaceID    string  `json:"workspace_id"`
	Trigger        string  `json:"trigger"`
	TriggeredBy    string  `json:"triggered_by"`
	Status         string  `json:"status"`
	CreatedAt      string  `json:"created_at"`
	FinishedAt     *string `json:"finished_at,omitempty"`
	// ApplyOutputRef: the real captured job output (docker-compose
	// stdout+stderr, or the failure error text if the job never
	// produced output) - see application.WorkerReportService.HandleReport
	// for why this is inline text rather than a real object storage
	// pointer. Was never surfaced via the API at all until now - the
	// only place it existed was the Worker's own stdout.
	ApplyOutputRef *string `json:"apply_output_ref,omitempty"`
}

func toRunResponse(r *domain.Run) runResponse {
	resp := runResponse{
		ID:             r.ID,
		OrganizationID: r.OrganizationID,
		WorkspaceID:    r.WorkspaceID,
		Trigger:        string(r.Trigger),
		TriggeredBy:    r.TriggeredBy,
		Status:         string(r.Status),
		CreatedAt:      r.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		ApplyOutputRef: r.ApplyOutputRef,
	}
	if r.FinishedAt != nil {
		formatted := r.FinishedAt.Format("2006-01-02T15:04:05Z07:00")
		resp.FinishedAt = &formatted
	}
	return resp
}

// writeExecutionError - this context's own error-mapping helper, same
// pattern as the Workspace context's writeNotFoundOrForbidden.
func writeExecutionError(w http.ResponseWriter, err error, notFoundTitle string) {
	switch {
	case errors.Is(err, domain.ErrForbidden):
		httpserver.WriteProblem(w, http.StatusForbidden, "forbidden", "")
	case errors.Is(err, domain.ErrWorkspaceNotFound), errors.Is(err, domain.ErrRunNotFound):
		httpserver.WriteProblem(w, http.StatusNotFound, notFoundTitle, "")
	case errors.Is(err, domain.ErrWorkspaceLocked):
		httpserver.WriteProblem(w, http.StatusConflict, "workspace is locked", "another run is already in progress on this workspace")
	case errors.Is(err, domain.ErrRunAlreadyTerminal):
		httpserver.WriteProblem(w, http.StatusConflict, "run already terminal", "")
	case errors.Is(err, domain.ErrOrganizationArchived):
		httpserver.WriteProblem(w, http.StatusConflict, "organization is archived", "")
	default:
		var validationErr *domain.ValidationError
		if errors.As(err, &validationErr) {
			httpserver.WriteProblem(w, http.StatusBadRequest, "validation failed", validationErr.Error())
			return
		}
		httpserver.WriteProblem(w, http.StatusInternalServerError, "internal error", "")
	}
}

func TriggerRunHandler(svc *application.TriggerRunService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		run, err := svc.Execute(r.Context(), application.TriggerRunInput{
			OrganizationID:   r.PathValue("id"),
			ProjectID:        r.PathValue("projectID"),
			WorkspaceID:      r.PathValue("workspaceID"),
			RequestingUserID: userID,
		})
		if err != nil {
			writeExecutionError(w, err, "workspace not found")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(toRunResponse(run))
	}
}

func CancelRunHandler(svc *application.CancelRunService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		run, err := svc.Execute(r.Context(), r.PathValue("id"), r.PathValue("projectID"), r.PathValue("workspaceID"), r.PathValue("runID"), userID)
		if err != nil {
			writeExecutionError(w, err, "run not found")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(toRunResponse(run))
	}
}

func ListRunsHandler(svc *application.ListRunsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		runs, err := svc.Execute(r.Context(), r.PathValue("id"), r.PathValue("projectID"), r.PathValue("workspaceID"), userID)
		if err != nil {
			writeExecutionError(w, err, "workspace not found")
			return
		}

		responses := make([]runResponse, 0, len(runs))
		for _, run := range runs {
			responses = append(responses, toRunResponse(run))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"data": responses})
	}
}

func GetRunHandler(svc *application.GetRunService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		run, err := svc.Execute(r.Context(), r.PathValue("id"), r.PathValue("projectID"), r.PathValue("workspaceID"), r.PathValue("runID"), userID)
		if err != nil {
			writeExecutionError(w, err, "run not found")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(toRunResponse(run))
	}
}
