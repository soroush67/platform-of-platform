// Package http is the RBAC context's REST adapter -
// docs/architecture/13-module-identity-rbac-tenancy.md §3's
// custom-role/role-binding endpoints, previously entirely unbuilt (every
// gated action elsewhere called straight into the postgres adapter as a
// cross-context port; this is RBAC's own first-class surface).
package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"platform-of-platform/internal/platform/httpserver"
	"platform-of-platform/internal/rbac/application"
	"platform-of-platform/internal/rbac/domain"
)

func writeRBACError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrForbidden):
		httpserver.WriteProblem(w, http.StatusForbidden, "forbidden", "")
	case errors.Is(err, domain.ErrRoleNotFound):
		httpserver.WriteProblem(w, http.StatusNotFound, "role not found", "")
	case errors.Is(err, domain.ErrRoleAlreadyExists):
		httpserver.WriteProblem(w, http.StatusConflict, "role already exists", err.Error())
	default:
		var validationErr *domain.ValidationError
		if errors.As(err, &validationErr) {
			httpserver.WriteProblem(w, http.StatusBadRequest, "validation failed", validationErr.Error())
			return
		}
		httpserver.WriteProblem(w, http.StatusInternalServerError, "internal error", "")
	}
}

type createRoleRequest struct {
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
}

type roleResponse struct {
	ID             string   `json:"id"`
	OrganizationID *string  `json:"organization_id"`
	Name           string   `json:"name"`
	Permissions    []string `json:"permissions"`
}

func toRoleResponse(r *domain.Role) roleResponse {
	permissions := make([]string, len(r.Permissions))
	for i, p := range r.Permissions {
		permissions[i] = string(p)
	}
	return roleResponse{ID: r.ID, OrganizationID: r.OrganizationID, Name: r.Name, Permissions: permissions}
}

// CreateRoleHandler implements `POST /orgs/{org}/roles`.
func CreateRoleHandler(svc *application.CreateRoleService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		var req createRoleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		role, err := svc.Execute(r.Context(), application.CreateRoleInput{
			OrganizationID:   r.PathValue("id"),
			RequestingUserID: userID,
			Name:             req.Name,
			Permissions:      req.Permissions,
		})
		if err != nil {
			writeRBACError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(toRoleResponse(role))
	}
}

type updateRoleRequest struct {
	Permissions []string `json:"permissions"`
}

// UpdateRoleHandler implements `PUT /orgs/{org}/roles/{role}` - rewrites
// a custom Role's permission set in place (name is immutable, see
// UpdateRoleService's own doc comment); a builtin Role's id maps to
// domain.ErrForbidden via writeRBACError, same as every other
// permission-denied case in this file.
func UpdateRoleHandler(svc *application.UpdateRoleService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		var req updateRoleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		role, err := svc.Execute(r.Context(), application.UpdateRoleInput{
			OrganizationID:   r.PathValue("id"),
			RequestingUserID: userID,
			RoleID:           r.PathValue("role"),
			Permissions:      req.Permissions,
		})
		if err != nil {
			writeRBACError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(toRoleResponse(role))
	}
}

// ListRolesHandler implements `GET /orgs/{org}/roles`.
func ListRolesHandler(svc *application.ListRolesService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		roles, err := svc.Execute(r.Context(), r.PathValue("id"), userID)
		if err != nil {
			writeRBACError(w, err)
			return
		}

		responses := make([]roleResponse, 0, len(roles))
		for _, role := range roles {
			responses = append(responses, toRoleResponse(role))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"data": responses})
	}
}

type subjectRef struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type scopeRef struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type createRoleBindingRequest struct {
	RoleID  string     `json:"role_id"`
	Subject subjectRef `json:"subject"`
	Scope   scopeRef   `json:"scope"`
	// Effect defaults to "allow" server-side (CreateRoleBindingService)
	// when omitted - existing clients that don't know about deny
	// bindings keep getting exactly the behavior they always had.
	Effect string `json:"effect,omitempty"`
}

type roleBindingResponse struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	RoleID         string `json:"role_id"`
	SubjectType    string `json:"subject_type"`
	SubjectID      string `json:"subject_id"`
	ScopeType      string `json:"scope_type"`
	ScopeID        string `json:"scope_id"`
	Effect         string `json:"effect"`
	CreatedAt      string `json:"created_at"`
}

func toRoleBindingResponse(b *domain.RoleBinding) roleBindingResponse {
	return roleBindingResponse{
		ID: b.ID, OrganizationID: b.OrganizationID, RoleID: b.RoleID,
		SubjectType: b.SubjectType, SubjectID: b.SubjectID,
		ScopeType: b.ScopeType, ScopeID: b.ScopeID, Effect: b.Effect,
		CreatedAt: b.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// CreateRoleBindingHandler implements `POST /orgs/{org}/role-bindings`.
func CreateRoleBindingHandler(svc *application.CreateRoleBindingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		var req createRoleBindingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		binding, err := svc.Execute(r.Context(), application.CreateRoleBindingInput{
			OrganizationID:   r.PathValue("id"),
			RequestingUserID: userID,
			RoleID:           req.RoleID,
			SubjectType:      req.Subject.Type,
			SubjectID:        req.Subject.ID,
			ScopeType:        req.Scope.Type,
			ScopeID:          req.Scope.ID,
			Effect:           req.Effect,
		})
		if err != nil {
			writeRBACError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(toRoleBindingResponse(binding))
	}
}

// roleBindingSummaryResponse mirrors roleBindingResponse plus the
// resolved display names ListRoleBindingsService now computes
// (list_role_bindings.go) - *Name is "" when unresolved (a real,
// displayable state, not an error) or when scope_type="organization"
// (the frontend already has the current org's own name loaded, no
// round trip needed for that one case).
type roleBindingSummaryResponse struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	RoleID         string `json:"role_id"`
	RoleName       string `json:"role_name"`
	SubjectType    string `json:"subject_type"`
	SubjectID      string `json:"subject_id"`
	SubjectName    string `json:"subject_name"`
	ScopeType      string `json:"scope_type"`
	ScopeID        string `json:"scope_id"`
	ScopeName      string `json:"scope_name"`
	Effect         string `json:"effect"`
	CreatedAt      string `json:"created_at"`
}

func toRoleBindingSummaryResponse(b application.RoleBindingSummary) roleBindingSummaryResponse {
	return roleBindingSummaryResponse{
		ID: b.ID, OrganizationID: b.OrganizationID, RoleID: b.RoleID, RoleName: b.RoleName,
		SubjectType: b.SubjectType, SubjectID: b.SubjectID, SubjectName: b.SubjectName,
		ScopeType: b.ScopeType, ScopeID: b.ScopeID, ScopeName: b.ScopeName, Effect: b.Effect,
		CreatedAt: b.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// ListRoleBindingsHandler implements
// `GET /orgs/{org}/role-bindings?subject_id=...`.
func ListRoleBindingsHandler(svc *application.ListRoleBindingsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		bindings, err := svc.Execute(r.Context(), r.PathValue("id"), r.URL.Query().Get("subject_id"), userID)
		if err != nil {
			writeRBACError(w, err)
			return
		}

		responses := make([]roleBindingSummaryResponse, 0, len(bindings))
		for _, b := range bindings {
			responses = append(responses, toRoleBindingSummaryResponse(b))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"data": responses})
	}
}

// DeleteRoleBindingHandler implements
// `DELETE /orgs/{org}/role-bindings/{id}` - a real, permanent removal.
func DeleteRoleBindingHandler(svc *application.DeleteRoleBindingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		err := svc.Execute(r.Context(), application.DeleteRoleBindingInput{
			OrganizationID:   r.PathValue("id"),
			RequestingUserID: userID,
			BindingID:        r.PathValue("bindingID"),
		})
		if err != nil {
			writeRBACError(w, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
