package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"platform-of-platform/internal/identity/application"
	"platform-of-platform/internal/identity/domain"
	"platform-of-platform/internal/platform/httpserver"
)

// GetOwnUserHandler implements GET /api/v1/users/me - single object
// response (not list-wrapped), same shape as GetProjectHandler
// (internal/tenancy/adapters/http/project_handler.go). Reuses the
// existing userResponse DTO (handler.go) unchanged - it already excludes
// PasswordHash.
func GetOwnUserHandler(svc *application.GetOwnUserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		user, err := svc.Execute(r.Context(), userID)
		if err != nil {
			if errors.Is(err, domain.ErrUserNotFound) {
				httpserver.WriteProblem(w, http.StatusNotFound, "user not found", "")
				return
			}
			httpserver.WriteProblem(w, http.StatusInternalServerError, "failed to fetch user", "")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(userResponse{
			ID:              user.ID,
			Username:        user.Username,
			Email:           user.Email,
			AuthSource:      string(user.AuthSource),
			Status:          user.Status,
			CreatedAt:       user.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			IsPlatformAdmin: user.IsPlatformAdmin,
		})
	}
}
