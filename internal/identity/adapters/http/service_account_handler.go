package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"platform-of-platform/internal/identity/application"
	"platform-of-platform/internal/identity/domain"
	"platform-of-platform/internal/platform/httpserver"
)

func writeIdentityError(w http.ResponseWriter, err error, notFoundMessage string) {
	var validationErr *domain.ValidationError
	if errors.As(err, &validationErr) {
		httpserver.WriteProblem(w, http.StatusBadRequest, "validation failed", validationErr.Error())
		return
	}
	if errors.Is(err, domain.ErrForbidden) {
		httpserver.WriteProblem(w, http.StatusForbidden, "forbidden", "requires organization:manage")
		return
	}
	if errors.Is(err, domain.ErrServiceAccountNotFound) || errors.Is(err, domain.ErrAPIKeyInvalid) {
		httpserver.WriteProblem(w, http.StatusNotFound, notFoundMessage, "")
		return
	}
	httpserver.WriteProblem(w, http.StatusInternalServerError, "internal error", "")
}

type createServiceAccountRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type serviceAccountResponse struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	CreatedAt      string `json:"created_at"`
}

func toServiceAccountResponse(sa *domain.ServiceAccount) serviceAccountResponse {
	return serviceAccountResponse{
		ID: sa.ID, OrganizationID: sa.OrganizationID, Name: sa.Name, Description: sa.Description,
		CreatedAt: sa.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// CreateServiceAccountHandler implements
// POST /api/v1/orgs/{id}/service-accounts.
func CreateServiceAccountHandler(svc *application.CreateServiceAccountService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		var req createServiceAccountRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		sa, err := svc.Execute(r.Context(), application.CreateServiceAccountInput{
			OrganizationID:   r.PathValue("id"),
			RequestingUserID: userID,
			Name:             req.Name,
			Description:      req.Description,
		})
		if err != nil {
			writeIdentityError(w, err, "organization not found")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(toServiceAccountResponse(sa))
	}
}

type createAPIKeyRequest struct {
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at"`
}

type apiKeyResponse struct {
	ID         string   `json:"id"`
	OwnerType  string   `json:"owner_type"`
	OwnerID    string   `json:"owner_id"`
	Name       string   `json:"name"`
	Scopes     []string `json:"scopes"`
	ExpiresAt  string   `json:"expires_at"`
	LastUsedAt *string  `json:"last_used_at,omitempty"`
	CreatedAt  string   `json:"created_at"`
	// Key is only ever populated on the CREATE response - "shown once,
	// never again" (see CreateAPIKeyResult's own comment). Every other
	// response that returns an APIKey leaves this nil/omitted.
	Key *string `json:"key,omitempty"`
}

func toAPIKeyResponse(k *domain.APIKey) apiKeyResponse {
	resp := apiKeyResponse{
		ID: k.ID, OwnerType: k.OwnerType, OwnerID: k.OwnerID, Name: k.Name,
		Scopes: k.Scopes, ExpiresAt: k.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
		CreatedAt: k.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if k.LastUsedAt != nil {
		formatted := k.LastUsedAt.Format("2006-01-02T15:04:05Z07:00")
		resp.LastUsedAt = &formatted
	}
	return resp
}

// CreateAPIKeyHandler implements
// POST /api/v1/orgs/{id}/service-accounts/{sa}/api-keys - the plaintext
// key is only ever present in THIS response, exactly once.
func CreateAPIKeyHandler(svc *application.CreateAPIKeyService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		var req createAPIKeyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		result, err := svc.Execute(r.Context(), application.CreateAPIKeyInput{
			OrganizationID:   r.PathValue("id"),
			RequestingUserID: userID,
			ServiceAccountID: r.PathValue("sa"),
			Name:             req.Name,
			Scopes:           req.Scopes,
			ExpiresAt:        req.ExpiresAt,
		})
		if err != nil {
			writeIdentityError(w, err, "service account not found")
			return
		}

		resp := toAPIKeyResponse(result.Key)
		resp.Key = &result.Plaintext

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(resp)
	}
}

// RevokeAPIKeyHandler implements
// DELETE /api/v1/orgs/{id}/service-accounts/{sa}/api-keys/{key}.
func RevokeAPIKeyHandler(svc *application.RevokeAPIKeyService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		err := svc.Execute(r.Context(), application.RevokeAPIKeyInput{
			OrganizationID:   r.PathValue("id"),
			RequestingUserID: userID,
			APIKeyID:         r.PathValue("key"),
		})
		if err != nil {
			writeIdentityError(w, err, "api key not found")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
