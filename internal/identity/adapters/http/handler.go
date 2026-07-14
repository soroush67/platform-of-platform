package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"platform-of-platform/internal/identity/application"
	"platform-of-platform/internal/identity/domain"
	"platform-of-platform/internal/platform/auth"
	"platform-of-platform/internal/platform/httpserver"
)

type createUserRequest struct {
	Username   string `json:"username"`
	Email      string `json:"email"`
	AuthSource string `json:"auth_source"`
	Password   string `json:"password"`
}

type userResponse struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	Email      string `json:"email"`
	AuthSource string `json:"auth_source"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
}

// CreateUserHandler implements POST /api/v1/users - see the use case's
// own comment for why this sits at the API root, not under an org.
func CreateUserHandler(svc *application.CreateUserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		user, err := svc.Execute(r.Context(), application.CreateUserInput{
			Username:   req.Username,
			Email:      req.Email,
			AuthSource: domain.AuthSource(req.AuthSource),
			Password:   req.Password,
		})
		if err != nil {
			var validationErr *domain.ValidationError
			if errors.As(err, &validationErr) {
				httpserver.WriteProblem(w, http.StatusBadRequest, "validation failed", validationErr.Error())
				return
			}
			httpserver.WriteProblem(w, http.StatusInternalServerError, "failed to create user", "")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(userResponse{
			ID:         user.ID,
			Username:   user.Username,
			Email:      user.Email,
			AuthSource: string(user.AuthSource),
			Status:     user.Status,
			CreatedAt:  user.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// LoginHandler implements POST /api/v1/auth/login
// (docs/architecture/04-api-design.md §4's "User session ... local
// login" credential type). Same error message and status for every
// failure mode - see AuthenticateService's own comment on why unknown
// username / wrong password / non-local user are indistinguishable
// here on purpose.
func LoginHandler(svc *application.AuthenticateService, jwtSecret []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		user, err := svc.Execute(r.Context(), req.Username, req.Password)
		if err != nil {
			if errors.Is(err, domain.ErrInvalidCredentials) {
				httpserver.WriteProblem(w, http.StatusUnauthorized, "invalid credentials", "")
				return
			}
			httpserver.WriteProblem(w, http.StatusInternalServerError, "login failed", "")
			return
		}

		token, err := auth.IssueAccessToken(jwtSecret, user.ID)
		if err != nil {
			httpserver.WriteProblem(w, http.StatusInternalServerError, "login failed", "")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(loginResponse{
			AccessToken: token,
			TokenType:   "Bearer",
			ExpiresIn:   int(auth.AccessTokenTTL.Seconds()),
		})
	}
}
