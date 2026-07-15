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
	// SecretRef is mutually exclusive with Value - see
	// application.CreateVariableInput's own comment.
	SecretRef *secretRefRequest `json:"secret_ref"`
}

type secretRefRequest struct {
	MountID string `json:"mount_id"`
	Path    string `json:"path"`
}

type secretRefResponse struct {
	MountID string `json:"mount_id"`
	Path    string `json:"path"`
}

type variableResponse struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	ScopeType      string `json:"scope_type"`
	ScopeID        string `json:"scope_id"`
	Key            string `json:"key"`
	Category       string `json:"category"`
	Sensitivity    string `json:"sensitivity"`
	// Value is a pointer so a sensitive variable serializes it as JSON
	// null instead of an empty string - distinguishable from "the value
	// genuinely is empty," which a plain "" would hide. Same masking
	// posture this operator's own compose-platform already established
	// this session for sensitive values. Always null for a SecretRef-
	// backed Variable outside of a real resolve call (Create/List never
	// touch the real backend), and masked the same as any other
	// sensitive Value when it *is* populated by ResolveVariableHandler.
	Value     *string            `json:"value"`
	SecretRef *secretRefResponse `json:"secret_ref"`
	CreatedAt string             `json:"created_at"`
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
	if v.SecretRef != nil {
		resp.SecretRef = &secretRefResponse{MountID: v.SecretRef.MountID, Path: v.SecretRef.Path}
	}
	// A SecretRef-backed Variable's Value is only ever non-empty right
	// after ResolveVariableHandler's own live fetch - Create/List/GetByID
	// never touch the real backend, so v.Value is always "" for them,
	// and this stays null rather than surfacing a misleading empty
	// string.
	if v.Sensitivity != domain.SensitivitySensitive && (v.SecretRef == nil || v.Value != "") {
		value := v.Value
		resp.Value = &value
	}
	return resp
}

type updateVariableRequest struct {
	Category    string `json:"category"`
	Sensitivity string `json:"sensitivity"`
	Value       string `json:"value"`
}

// UpdateVariableHandler implements
// `PUT /orgs/{org}/variables/{variableID}` - Key/ScopeType/ScopeID are
// immutable (see the postgres adapter's own comment), so the request
// body only carries Value/Category/Sensitivity.
func UpdateVariableHandler(svc *application.UpdateVariableService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		var req updateVariableRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		v, err := svc.Execute(r.Context(), application.UpdateVariableInput{
			OrganizationID:   r.PathValue("id"),
			RequestingUserID: userID,
			VariableID:       r.PathValue("variableID"),
			Value:            req.Value,
			Category:         domain.Category(req.Category),
			Sensitivity:      domain.Sensitivity(req.Sensitivity),
		})
		if err != nil {
			writeVariablesError(w, err, "variable not found")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(toVariableResponse(v))
	}
}

// DeleteVariableHandler implements
// `DELETE /orgs/{org}/variables/{variableID}`.
func DeleteVariableHandler(svc *application.DeleteVariableService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		err := svc.Execute(r.Context(), application.DeleteVariableInput{
			OrganizationID:   r.PathValue("id"),
			RequestingUserID: userID,
			VariableID:       r.PathValue("variableID"),
		})
		if err != nil {
			writeVariablesError(w, err, "variable not found")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func writeVariablesError(w http.ResponseWriter, err error, notFoundTitle string) {
	switch {
	case errors.Is(err, domain.ErrForbidden):
		httpserver.WriteProblem(w, http.StatusForbidden, "forbidden", "")
	case errors.Is(err, domain.ErrScopeNotFound):
		httpserver.WriteProblem(w, http.StatusNotFound, "scope not found", "")
	case errors.Is(err, domain.ErrVariableNotFound):
		httpserver.WriteProblem(w, http.StatusNotFound, notFoundTitle, "")
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

		in := application.CreateVariableInput{
			OrganizationID:   r.PathValue("id"),
			RequestingUserID: userID,
			ScopeType:        domain.ScopeType(req.ScopeType),
			ScopeID:          req.ScopeID,
			Key:              req.Key,
			Category:         domain.Category(req.Category),
			Sensitivity:      domain.Sensitivity(req.Sensitivity),
			Value:            req.Value,
		}
		if req.SecretRef != nil {
			in.SecretMountID = req.SecretRef.MountID
			in.SecretPath = req.SecretRef.Path
		}

		v, err := svc.Execute(r.Context(), in)
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
