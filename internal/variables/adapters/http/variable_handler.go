// Package http is the Variables context's REST adapter.
package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"platform-of-platform/internal/platform/httpserver"
	"platform-of-platform/internal/variables/application"
	"platform-of-platform/internal/variables/domain"
)

type createVariableRequest struct {
	ScopeType   string `json:"scope_type"`
	ScopeID     string `json:"scope_id"`
	Key         string `json:"key"`
	Category    string `json:"category"`
	Sensitivity string `json:"sensitivity"`
	Value       string `json:"value"`
}

type variableResponse struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organization_id"`
	ScopeType      string  `json:"scope_type"`
	ScopeID        string  `json:"scope_id"`
	Key            string  `json:"key"`
	Category       string  `json:"category"`
	Sensitivity    string  `json:"sensitivity"`
	// Value is a pointer so a sensitive variable serializes it as JSON
	// null instead of an empty string - distinguishable from "the value
	// genuinely is empty," which a plain "" would hide. Same masking
	// posture this operator's own compose-platform already established
	// this session for sensitive values.
	Value *string `json:"value"`
	CreatedAt string `json:"created_at"`
}

func toVariableResponse(v *domain.Variable) variableResponse {
	resp := variableResponse{
		ID:             v.ID,
		OrganizationID: v.OrganizationID,
		ScopeType:      string(v.ScopeType),
		ScopeID:        v.ScopeID,
		Key:            v.Key,
		Category:       string(v.Category),
		Sensitivity:    string(v.Sensitivity),
		CreatedAt:      v.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if v.Sensitivity != domain.SensitivitySensitive {
		value := v.Value
		resp.Value = &value
	}
	return resp
}

func writeVariablesError(w http.ResponseWriter, err error, notFoundTitle string) {
	switch {
	case errors.Is(err, domain.ErrForbidden):
		httpserver.WriteProblem(w, http.StatusForbidden, "forbidden", "")
	case errors.Is(err, domain.ErrScopeNotFound):
		httpserver.WriteProblem(w, http.StatusNotFound, "scope not found", "")
	case errors.Is(err, domain.ErrVariableNotFound):
		httpserver.WriteProblem(w, http.StatusNotFound, notFoundTitle, "")
	default:
		var validationErr *domain.ValidationError
		if errors.As(err, &validationErr) {
			httpserver.WriteProblem(w, http.StatusBadRequest, "validation failed", validationErr.Error())
			return
		}
		httpserver.WriteProblem(w, http.StatusInternalServerError, "internal error", "")
	}
}

// CreateVariableHandler implements POST /api/v1/orgs/{id}/variables -
// one generic endpoint for all scope types, see CreateVariableInput's
// own comment.
func CreateVariableHandler(svc *application.CreateVariableService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		var req createVariableRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		v, err := svc.Execute(r.Context(), application.CreateVariableInput{
			OrganizationID:   r.PathValue("id"),
			RequestingUserID: userID,
			ScopeType:        domain.ScopeType(req.ScopeType),
			ScopeID:          req.ScopeID,
			Key:              req.Key,
			Category:         domain.Category(req.Category),
			Sensitivity:      domain.Sensitivity(req.Sensitivity),
			Value:            req.Value,
		})
		if err != nil {
			writeVariablesError(w, err, "variable not found")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(toVariableResponse(v))
	}
}

// ListVariablesHandler implements
// GET /api/v1/orgs/{id}/variables?scope_type=...&scope_id=...
func ListVariablesHandler(svc *application.ListVariablesService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		scopeType := domain.ScopeType(r.URL.Query().Get("scope_type"))
		scopeID := r.URL.Query().Get("scope_id")

		variables, err := svc.Execute(r.Context(), r.PathValue("id"), userID, scopeType, scopeID)
		if err != nil {
			writeVariablesError(w, err, "variable not found")
			return
		}

		responses := make([]variableResponse, 0, len(variables))
		for _, v := range variables {
			responses = append(responses, toVariableResponse(v))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"data": responses})
	}
}

// ResolveVariableHandler implements
// GET .../workspaces/{workspaceID}/variables/resolve?key=... - the
// cascade this whole context exists for.
func ResolveVariableHandler(svc *application.ResolveVariableService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		key := r.URL.Query().Get("key")

		v, err := svc.Execute(r.Context(), r.PathValue("id"), r.PathValue("workspaceID"), key, userID)
		if err != nil {
			writeVariablesError(w, err, "variable not found in this workspace's scope cascade")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(toVariableResponse(v))
	}
}
