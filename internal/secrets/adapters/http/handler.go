// Package http is the Secrets context's REST adapter -
// docs/architecture/11-module-secrets-state.md §1.
package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"platform-of-platform/internal/platform/httpserver"
	"platform-of-platform/internal/secrets/application"
	"platform-of-platform/internal/secrets/domain"
)

func writeSecretsError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrForbidden):
		httpserver.WriteProblem(w, http.StatusForbidden, "forbidden", "")
	case errors.Is(err, domain.ErrSecretMountNotFound):
		httpserver.WriteProblem(w, http.StatusNotFound, "secret mount not found", "")
	default:
		var validationErr *domain.ValidationError
		if errors.As(err, &validationErr) {
			httpserver.WriteProblem(w, http.StatusBadRequest, "validation failed", validationErr.Error())
			return
		}
		httpserver.WriteProblem(w, http.StatusInternalServerError, "internal error", "")
	}
}

type createSecretMountRequest struct {
	Name        string `json:"name"`
	BackendType string `json:"backend_type"`
	Address     string `json:"address"`
	RoleID      string `json:"role_id"`
	SecretID    string `json:"secret_id"`
}

// secretMountResponse deliberately has no field for the sealed
// credential bytes - EncryptedSecretID/Nonce/Salt never serialize out
// through this API at all, not even to the org that owns the mount.
type secretMountResponse struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	Name           string `json:"name"`
	BackendType    string `json:"backend_type"`
	Address        string `json:"address"`
	RoleID         string `json:"role_id"`
	CreatedAt      string `json:"created_at"`
}

func toSecretMountResponse(m *domain.SecretMount) secretMountResponse {
	return secretMountResponse{
		ID:             m.ID,
		OrganizationID: m.OrganizationID,
		Name:           m.Name,
		BackendType:    string(m.BackendType),
		Address:        m.Address,
		RoleID:         m.RoleID,
		CreatedAt:      m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// CreateSecretMountHandler implements POST /api/v1/orgs/{id}/secret-mounts.
func CreateSecretMountHandler(svc *application.CreateSecretMountService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		var req createSecretMountRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		mount, err := svc.Execute(r.Context(), application.CreateSecretMountInput{
			OrganizationID:   r.PathValue("id"),
			RequestingUserID: userID,
			Name:             req.Name,
			BackendType:      req.BackendType,
			Address:          req.Address,
			RoleID:           req.RoleID,
			SecretID:         req.SecretID,
		})
		if err != nil {
			writeSecretsError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(toSecretMountResponse(mount))
	}
}

// ListSecretMountsHandler implements GET /api/v1/orgs/{id}/secret-mounts.
func ListSecretMountsHandler(svc *application.ListSecretMountsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		mounts, err := svc.Execute(r.Context(), r.PathValue("id"), userID)
		if err != nil {
			writeSecretsError(w, err)
			return
		}

		responses := make([]secretMountResponse, 0, len(mounts))
		for _, m := range mounts {
			responses = append(responses, toSecretMountResponse(m))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"data": responses})
	}
}

// TestConnectionHandler implements
// POST /api/v1/orgs/{id}/secret-mounts/{mount}/test-connection - 204 on
// success, a real error status otherwise; never any response body that
// could carry secret content either way.
func TestConnectionHandler(svc *application.TestConnectionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}

		err := svc.Execute(r.Context(), r.PathValue("id"), r.PathValue("mount"), userID)
		if err != nil {
			if errors.Is(err, domain.ErrForbidden) || errors.Is(err, domain.ErrSecretMountNotFound) {
				writeSecretsError(w, err)
				return
			}
			// A real connection/auth failure against the backend itself
			// (wrong credential, unreachable address) - 502, the honest
			// "we reached out on your behalf and it failed" status,
			// distinct from this platform's own 4xx validation/auth
			// failures above.
			httpserver.WriteProblem(w, http.StatusBadGateway, "connection test failed", err.Error())
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
