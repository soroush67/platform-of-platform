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
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

// CreateOrganizationHandler implements POST /api/v1/orgs
// (docs/architecture/04-api-design.md §1). Registered behind
// httpserver.RequireAuth in main.go - the creator becomes the org's
// first member (see the use case's own comment).
func CreateOrganizationHandler(svc *application.CreateOrganizationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		var req createOrganizationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		org, err := svc.Execute(r.Context(), application.CreateOrganizationInput{
			Name:            req.Name,
			Slug:            req.Slug,
			CreatedByUserID: userID,
		})
		if err != nil {
			var validationErr *domain.ValidationError
			if errors.As(err, &validationErr) {
				httpserver.WriteProblem(w, http.StatusBadRequest, "validation failed", validationErr.Error())
				return
			}
			if errors.Is(err, domain.ErrForbidden) {
				httpserver.WriteProblem(w, http.StatusForbidden, "forbidden", "requires platform admin")
				return
			}
			if errors.Is(err, domain.ErrOrganizationSlugTaken) {
				httpserver.WriteProblem(w, http.StatusConflict, "organization already exists", err.Error())
				return
			}
			httpserver.WriteProblem(w, http.StatusInternalServerError, "failed to create organization", "")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(toOrganizationResponse(org))
	}
}

// GetOrganizationHandler implements GET /api/v1/orgs/{id}. Registered
// behind httpserver.RequireAuth in main.go - the use case checks
// OrganizationMembership for the authenticated user before returning
// anything.
func GetOrganizationHandler(svc *application.GetOrganizationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		id := r.PathValue("id")

		org, err := svc.Execute(r.Context(), id, userID)
		if err != nil {
			if errors.Is(err, domain.ErrOrganizationNotFound) {
				httpserver.WriteProblem(w, http.StatusNotFound, "organization not found", "")
				return
			}
			httpserver.WriteProblem(w, http.StatusInternalServerError, "failed to fetch organization", "")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(toOrganizationResponse(org))
	}
}

type addMemberRequest struct {
	UserID string `json:"user_id"`
}

// AddMemberHandler implements POST /api/v1/orgs/{id}/members. Registered
// behind httpserver.RequireAuth in main.go - the use case is what
// actually checks organization:manage, not this handler.
func AddMemberHandler(svc *application.AddMemberService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		orgID := r.PathValue("id")

		var req addMemberRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		err := svc.Execute(r.Context(), application.AddMemberInput{
			OrganizationID:   orgID,
			RequestingUserID: userID,
			NewMemberUserID:  req.UserID,
		})
		if err != nil {
			if errors.Is(err, domain.ErrForbidden) {
				httpserver.WriteProblem(w, http.StatusForbidden, "forbidden", "requires organization:manage")
				return
			}
			if errors.Is(err, domain.ErrOrganizationNotFound) {
				httpserver.WriteProblem(w, http.StatusNotFound, "organization not found", "")
				return
			}
			httpserver.WriteProblem(w, http.StatusInternalServerError, "failed to add member", "")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

type changeMemberRoleRequest struct {
	Role string `json:"role"`
}

// ChangeMemberRoleHandler implements
// PUT /api/v1/orgs/{id}/members/{userID}/role. Registered behind
// httpserver.RequireAuth in main.go.
func ChangeMemberRoleHandler(svc *application.ChangeMemberRoleService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		var req changeMemberRoleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		err := svc.Execute(r.Context(), application.ChangeMemberRoleInput{
			OrganizationID:   r.PathValue("id"),
			RequestingUserID: userID,
			TargetUserID:     r.PathValue("userID"),
			RoleName:         req.Role,
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
				httpserver.WriteProblem(w, http.StatusNotFound, "organization or member not found", "")
				return
			}
			httpserver.WriteProblem(w, http.StatusInternalServerError, "failed to change member role", "")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

type memberResponse struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	RoleName string `json:"role_name"`
	JoinedAt string `json:"joined_at"`
	Blocked  bool   `json:"blocked"`
}

func toMemberResponse(m application.MemberSummary) memberResponse {
	return memberResponse{
		UserID:   m.UserID,
		Username: m.Username,
		Email:    m.Email,
		RoleName: m.RoleName,
		JoinedAt: m.JoinedAt.Format("2006-01-02T15:04:05Z07:00"),
		Blocked:  m.Blocked,
	}
}

// BlockMemberHandler implements PUT /api/v1/orgs/{id}/members/{userID}/block.
func BlockMemberHandler(svc *application.BlockMemberService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		err := svc.Execute(r.Context(), application.BlockMemberInput{
			OrganizationID:   r.PathValue("id"),
			RequestingUserID: userID,
			TargetUserID:     r.PathValue("userID"),
		})
		if err != nil {
			if errors.Is(err, domain.ErrForbidden) {
				httpserver.WriteProblem(w, http.StatusForbidden, "forbidden", "requires organization:manage")
				return
			}
			if errors.Is(err, domain.ErrOrganizationNotFound) {
				httpserver.WriteProblem(w, http.StatusNotFound, "organization or member not found", "")
				return
			}
			httpserver.WriteProblem(w, http.StatusInternalServerError, "failed to block member", "")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// UnblockMemberHandler implements PUT /api/v1/orgs/{id}/members/{userID}/unblock.
func UnblockMemberHandler(svc *application.UnblockMemberService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		err := svc.Execute(r.Context(), application.UnblockMemberInput{
			OrganizationID:   r.PathValue("id"),
			RequestingUserID: userID,
			TargetUserID:     r.PathValue("userID"),
		})
		if err != nil {
			if errors.Is(err, domain.ErrForbidden) {
				httpserver.WriteProblem(w, http.StatusForbidden, "forbidden", "requires organization:manage")
				return
			}
			if errors.Is(err, domain.ErrOrganizationNotFound) {
				httpserver.WriteProblem(w, http.StatusNotFound, "organization or member not found", "")
				return
			}
			httpserver.WriteProblem(w, http.StatusInternalServerError, "failed to unblock member", "")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// RemoveMemberHandler implements DELETE /api/v1/orgs/{id}/members/{userID} -
// a real, permanent removal of this membership (the long-flagged gap,
// finally closed).
func RemoveMemberHandler(svc *application.RemoveMemberService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		err := svc.Execute(r.Context(), application.RemoveMemberInput{
			OrganizationID:   r.PathValue("id"),
			RequestingUserID: userID,
			TargetUserID:     r.PathValue("userID"),
		})
		if err != nil {
			if errors.Is(err, domain.ErrForbidden) {
				httpserver.WriteProblem(w, http.StatusForbidden, "forbidden", "requires organization:manage")
				return
			}
			if errors.Is(err, domain.ErrOrganizationNotFound) {
				httpserver.WriteProblem(w, http.StatusNotFound, "organization or member not found", "")
				return
			}
			httpserver.WriteProblem(w, http.StatusInternalServerError, "failed to remove member", "")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// ListMembersHandler implements GET /api/v1/orgs/{id}/members -
// membership-gated only, same "any member can see the roster" posture
// as ListProjectsHandler.
func ListMembersHandler(svc *application.ListMembersService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		orgID := r.PathValue("id")

		members, err := svc.Execute(r.Context(), orgID, userID)
		if err != nil {
			if errors.Is(err, domain.ErrOrganizationNotFound) {
				httpserver.WriteProblem(w, http.StatusNotFound, "organization not found", "")
				return
			}
			httpserver.WriteProblem(w, http.StatusInternalServerError, "failed to list members", "")
			return
		}

		responses := make([]memberResponse, 0, len(members))
		for _, m := range members {
			responses = append(responses, toMemberResponse(m))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"data": responses})
	}
}

type availableUserResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

// ListAvailableUsersHandler implements
// `GET /api/v1/orgs/{id}/members/available` - backs the Members page's
// "add existing user" picker, same organization:manage gate as
// AddMemberHandler.
func ListAvailableUsersHandler(svc *application.ListAvailableUsersService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		orgID := r.PathValue("id")

		users, err := svc.Execute(r.Context(), orgID, userID)
		if err != nil {
			if errors.Is(err, domain.ErrForbidden) {
				httpserver.WriteProblem(w, http.StatusForbidden, "forbidden", "requires organization:manage")
				return
			}
			if errors.Is(err, domain.ErrOrganizationNotFound) {
				httpserver.WriteProblem(w, http.StatusNotFound, "organization not found", "")
				return
			}
			httpserver.WriteProblem(w, http.StatusInternalServerError, "failed to list available users", "")
			return
		}

		responses := make([]availableUserResponse, 0, len(users))
		for _, u := range users {
			responses = append(responses, availableUserResponse{ID: u.ID, Username: u.Username, Email: u.Email})
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"data": responses})
	}
}

func toOrganizationResponse(org *domain.Organization) organizationResponse {
	return organizationResponse{
		ID:        org.ID,
		Name:      org.Name,
		Slug:      org.Slug,
		Status:    org.Status,
		CreatedAt: org.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// ArchiveOrganizationHandler implements DELETE /api/v1/orgs/{id}
// (docs/architecture/13-module-identity-rbac-tenancy.md §1) - a soft
// delete (status: archived), gated by organization:delete, the first
// real Owner-only capability in this codebase (see
// internal/rbac/domain/role.go's own comment).
func ArchiveOrganizationHandler(svc *application.ArchiveOrganizationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		org, err := svc.Execute(r.Context(), application.ArchiveOrganizationInput{
			OrganizationID:   r.PathValue("id"),
			RequestingUserID: userID,
		})
		if err != nil {
			if errors.Is(err, domain.ErrForbidden) {
				httpserver.WriteProblem(w, http.StatusForbidden, "forbidden", "requires organization:delete")
				return
			}
			if errors.Is(err, domain.ErrOrganizationNotFound) {
				httpserver.WriteProblem(w, http.StatusNotFound, "organization not found", "")
				return
			}
			if errors.Is(err, domain.ErrOrganizationAlreadyArchived) {
				httpserver.WriteProblem(w, http.StatusConflict, "organization is already archived", "")
				return
			}
			httpserver.WriteProblem(w, http.StatusInternalServerError, "failed to archive organization", "")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(toOrganizationResponse(org))
	}
}

// DeleteOrganizationHandler implements POST /api/v1/orgs/{id}/hard-delete -
// a genuinely irreversible hard delete (calls OrganizationRepository.Purge
// directly, the same method PurgeReaperService uses after its own 30-day
// grace window), not the soft-delete ArchiveOrganizationHandler above
// implements. POST, not DELETE - matches this codebase's existing
// convention for non-idempotent, non-plain-CRUD actions (test-connection,
// check-connection, platform-admin promotion), deliberately distinct from
// the resource-deletion semantics `DELETE /orgs/{id}` already owns.
// Same organization:delete permission as Archive (Owner-only) - works on
// an Organization regardless of its current status, operator-confirmed,
// no "must be archived first" gate.
func DeleteOrganizationHandler(svc *application.DeleteOrganizationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		err := svc.Execute(r.Context(), application.DeleteOrganizationInput{
			OrganizationID:   r.PathValue("id"),
			RequestingUserID: userID,
		})
		if err != nil {
			if errors.Is(err, domain.ErrForbidden) {
				httpserver.WriteProblem(w, http.StatusForbidden, "forbidden", "requires organization:delete")
				return
			}
			if errors.Is(err, domain.ErrOrganizationNotFound) {
				httpserver.WriteProblem(w, http.StatusNotFound, "organization not found", "")
				return
			}
			httpserver.WriteProblem(w, http.StatusInternalServerError, "failed to delete organization", "")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
