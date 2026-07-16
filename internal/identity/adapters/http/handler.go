package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"platform-of-platform/internal/identity/application"
	"platform-of-platform/internal/identity/domain"
	"platform-of-platform/internal/platform/auth"
	"platform-of-platform/internal/platform/httpserver"
	"platform-of-platform/internal/platform/ratelimit"
)

type createUserRequest struct {
	Username   string `json:"username"`
	Email      string `json:"email"`
	AuthSource string `json:"auth_source"`
	Password   string `json:"password"`
}

type userResponse struct {
	ID              string `json:"id"`
	Username        string `json:"username"`
	Email           string `json:"email"`
	AuthSource      string `json:"auth_source"`
	Status          string `json:"status"`
	CreatedAt       string `json:"created_at"`
	IsPlatformAdmin bool   `json:"is_platform_admin"`
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

type setPlatformAdminRequest struct {
	IsPlatformAdmin bool `json:"is_platform_admin"`
}

// SetPlatformAdminHandler implements PUT /api/v1/users/{id}/platform-
// admin - an existing platform admin promotes/demotes another user.
func SetPlatformAdminHandler(svc *application.SetPlatformAdminService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		var req setPlatformAdminRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		err := svc.Execute(r.Context(), application.SetPlatformAdminInput{
			RequestingUserID: userID,
			TargetUserID:     r.PathValue("id"),
			IsPlatformAdmin:  req.IsPlatformAdmin,
		})
		if err != nil {
			if errors.Is(err, domain.ErrForbidden) {
				httpserver.WriteProblem(w, http.StatusForbidden, "forbidden", "requires platform admin")
				return
			}
			if errors.Is(err, domain.ErrUserNotFound) {
				httpserver.WriteProblem(w, http.StatusNotFound, "user not found", "")
				return
			}
			httpserver.WriteProblem(w, http.StatusInternalServerError, "failed to update platform admin status", "")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

// LoginHandler implements POST /api/v1/auth/login
// (docs/architecture/04-api-design.md §4's "User session ... local
// login" credential type). Same error message and status for every
// failure mode - see AuthenticateService's own comment on why unknown
// username / wrong password / non-local user are indistinguishable
// here on purpose. Also issues a real refresh token now (previously the
// access token's 15-minute TTL was the only session mechanism at all -
// POST /auth/refresh, RefreshTokenHandler below, is the actual fix).
//
// loginLimiter is keyed by *username*, not client IP - the general
// per-IP limiter (httpserver.RateLimit, wrapping the whole mux) already
// covers "one IP hammering everything"; this is the narrower, more
// valuable defense for login specifically: credential stuffing spread
// across many IPs against one account. Every attempt against a username
// counts against its budget, successful or not - a legitimate user
// briefly locked out after mistyping a password five times is the
// accepted tradeoff for actually stopping brute force, not a bug.
func LoginHandler(svc *application.AuthenticateService, refreshSvc *application.RefreshTokenService, loginLimiter *ratelimit.Limiter, jwtSecret []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		if allowed, retryAfter := loginLimiter.Allow(req.Username); !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
			httpserver.WriteProblem(w, http.StatusTooManyRequests, "too many login attempts", "")
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

		refreshToken, err := refreshSvc.Issue(r.Context(), user.ID)
		if err != nil {
			httpserver.WriteProblem(w, http.StatusInternalServerError, "login failed", "")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(loginResponse{
			AccessToken:  token,
			TokenType:    "Bearer",
			ExpiresIn:    int(auth.AccessTokenTTL.Seconds()),
			RefreshToken: refreshToken,
		})
	}
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// RefreshTokenHandler implements POST /api/v1/auth/refresh - exchanges
// a valid, unexpired refresh token for a new access token AND a new
// refresh token (rotation - see RefreshTokenService's own comment on
// why the old one is revoked, not reused).
func RefreshTokenHandler(svc *application.RefreshTokenService, jwtSecret []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req refreshRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		userID, newRefreshToken, err := svc.Refresh(r.Context(), req.RefreshToken)
		if err != nil {
			if errors.Is(err, domain.ErrRefreshTokenInvalid) {
				httpserver.WriteProblem(w, http.StatusUnauthorized, "invalid refresh token", "")
				return
			}
			httpserver.WriteProblem(w, http.StatusInternalServerError, "refresh failed", "")
			return
		}

		token, err := auth.IssueAccessToken(jwtSecret, userID)
		if err != nil {
			httpserver.WriteProblem(w, http.StatusInternalServerError, "refresh failed", "")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(loginResponse{
			AccessToken:  token,
			TokenType:    "Bearer",
			ExpiresIn:    int(auth.AccessTokenTTL.Seconds()),
			RefreshToken: newRefreshToken,
		})
	}
}

type requestPasswordResetRequest struct {
	Email string `json:"email"`
}

// RequestPasswordResetHandler implements
// POST /api/v1/auth/password-reset/request - always 202, regardless of
// whether the email exists or belongs to a local-auth user (see
// PasswordResetService.RequestReset's own comment on why).
func RequestPasswordResetHandler(svc *application.PasswordResetService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req requestPasswordResetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		if err := svc.RequestReset(r.Context(), req.Email); err != nil {
			httpserver.WriteProblem(w, http.StatusInternalServerError, "password reset request failed", "")
			return
		}

		w.WriteHeader(http.StatusAccepted)
	}
}

type confirmPasswordResetRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

// ConfirmPasswordResetHandler implements
// POST /api/v1/auth/password-reset/confirm.
func ConfirmPasswordResetHandler(svc *application.PasswordResetService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req confirmPasswordResetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		err := svc.ConfirmReset(r.Context(), req.Token, req.NewPassword)
		if err != nil {
			var validationErr *domain.ValidationError
			if errors.As(err, &validationErr) {
				httpserver.WriteProblem(w, http.StatusBadRequest, "validation failed", validationErr.Error())
				return
			}
			if errors.Is(err, domain.ErrPasswordResetTokenInvalid) {
				httpserver.WriteProblem(w, http.StatusUnauthorized, "invalid or expired token", "")
				return
			}
			httpserver.WriteProblem(w, http.StatusInternalServerError, "password reset failed", "")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
