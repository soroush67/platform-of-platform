package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"platform-of-platform/internal/platform/httpserver"
	"platform-of-platform/internal/tenancy/application"
	"platform-of-platform/internal/tenancy/domain"
)

type createTeamRequest struct {
	Name string `json:"name"`
}

type teamResponse struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	Name           string `json:"name"`
	CreatedAt      string `json:"created_at"`
}

func toTeamResponse(t *domain.Team) teamResponse {
	return teamResponse{ID: t.ID, OrganizationID: t.OrganizationID, Name: t.Name, CreatedAt: t.CreatedAt.Format("2006-01-02T15:04:05Z07:00")}
}

func writeTeamError(w http.ResponseWriter, err error, notFoundMessage string) {
	var validationErr *domain.ValidationError
	if errors.As(err, &validationErr) {
		httpserver.WriteProblem(w, http.StatusBadRequest, "validation failed", validationErr.Error())
		return
	}
	if errors.Is(err, domain.ErrForbidden) {
		httpserver.WriteProblem(w, http.StatusForbidden, "forbidden", "requires organization:manage")
		return
	}
	if errors.Is(err, domain.ErrOrganizationNotFound) || errors.Is(err, domain.ErrTeamNotFound) {
		httpserver.WriteProblem(w, http.StatusNotFound, notFoundMessage, "")
		return
	}
	httpserver.WriteProblem(w, http.StatusInternalServerError, "internal error", "")
}

// CreateTeamHandler implements POST /api/v1/orgs/{id}/teams.
func CreateTeamHandler(svc *application.CreateTeamService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		var req createTeamRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		team, err := svc.Execute(r.Context(), application.CreateTeamInput{
			OrganizationID:   r.PathValue("id"),
			RequestingUserID: userID,
			Name:             req.Name,
		})
		if err != nil {
			writeTeamError(w, err, "organization not found")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(toTeamResponse(team))
	}
}

type addTeamMemberRequest struct {
	UserID string `json:"user_id"`
}

// AddTeamMemberHandler implements
// POST /api/v1/orgs/{id}/teams/{team}/members.
func AddTeamMemberHandler(svc *application.AddTeamMemberService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		var req addTeamMemberRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		err := svc.Execute(r.Context(), application.AddTeamMemberInput{
			OrganizationID:   r.PathValue("id"),
			RequestingUserID: userID,
			TeamID:           r.PathValue("team"),
			NewMemberUserID:  req.UserID,
		})
		if err != nil {
			writeTeamError(w, err, "team not found")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// RemoveTeamMemberHandler implements
// DELETE /api/v1/orgs/{id}/teams/{team}/members/{user_id}.
func RemoveTeamMemberHandler(svc *application.RemoveTeamMemberService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		err := svc.Execute(r.Context(), application.RemoveTeamMemberInput{
			OrganizationID:   r.PathValue("id"),
			RequestingUserID: userID,
			TeamID:           r.PathValue("team"),
			MemberUserID:     r.PathValue("user_id"),
		})
		if err != nil {
			writeTeamError(w, err, "team not found")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
