package http

import (
	"encoding/json"
	"net/http"

	"platform-of-platform/internal/fleet/application"
	"platform-of-platform/internal/fleet/domain"
	"platform-of-platform/internal/platform/httpserver"
)

type createNetworkRequest struct {
	Name     string `json:"name"`
	External bool   `json:"external"`
}

type networkResponse struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	Name           string `json:"name"`
	External       bool   `json:"external"`
	CreatedBy      string `json:"created_by"`
	CreatedAt      string `json:"created_at"`
}

func toNetworkResponse(n *domain.Network) networkResponse {
	return networkResponse{
		ID: n.ID, OrganizationID: n.OrganizationID, Name: n.Name, External: n.External,
		CreatedBy: n.CreatedBy, CreatedAt: n.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func CreateNetworkHandler(svc *application.CreateNetworkService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		var req createNetworkRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		network, err := svc.Execute(r.Context(), application.CreateNetworkInput{
			OrganizationID: r.PathValue("id"), RequestingUserID: userID, Name: req.Name, External: req.External,
		})
		if err != nil {
			writeFleetError(w, err, "")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(toNetworkResponse(network))
	}
}

func ListNetworksHandler(svc *application.ListNetworksService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		networks, err := svc.Execute(r.Context(), r.PathValue("id"), userID)
		if err != nil {
			writeFleetError(w, err, "")
			return
		}
		responses := make([]networkResponse, 0, len(networks))
		for _, n := range networks {
			responses = append(responses, toNetworkResponse(n))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"data": responses})
	}
}

func DeleteNetworkHandler(svc *application.DeleteNetworkService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		if err := svc.Execute(r.Context(), r.PathValue("id"), userID, r.PathValue("networkID")); err != nil {
			writeFleetError(w, err, "network not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
