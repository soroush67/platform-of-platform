package http

import (
	"encoding/json"
	"net/http"

	"platform-of-platform/internal/fleet/application"
	"platform-of-platform/internal/fleet/domain"
	"platform-of-platform/internal/platform/httpserver"
)

type createVolumeRequest struct {
	Name     string `json:"name"`
	HostPath string `json:"host_path"`
}

type volumeResponse struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	Name           string `json:"name"`
	HostPath       string `json:"host_path"`
	CreatedBy      string `json:"created_by"`
	CreatedAt      string `json:"created_at"`
}

func toVolumeResponse(v *domain.Volume) volumeResponse {
	return volumeResponse{
		ID: v.ID, OrganizationID: v.OrganizationID, Name: v.Name, HostPath: v.HostPath,
		CreatedBy: v.CreatedBy, CreatedAt: v.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func CreateVolumeHandler(svc *application.CreateVolumeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		var req createVolumeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		volume, err := svc.Execute(r.Context(), application.CreateVolumeInput{
			OrganizationID: r.PathValue("id"), RequestingUserID: userID, Name: req.Name, HostPath: req.HostPath,
		})
		if err != nil {
			writeFleetError(w, err, "")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(toVolumeResponse(volume))
	}
}

func ListVolumesHandler(svc *application.ListVolumesService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		volumes, err := svc.Execute(r.Context(), r.PathValue("id"), userID)
		if err != nil {
			writeFleetError(w, err, "")
			return
		}
		responses := make([]volumeResponse, 0, len(volumes))
		for _, v := range volumes {
			responses = append(responses, toVolumeResponse(v))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"data": responses})
	}
}

func DeleteVolumeHandler(svc *application.DeleteVolumeService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		if err := svc.Execute(r.Context(), r.PathValue("id"), userID, r.PathValue("volumeID")); err != nil {
			writeFleetError(w, err, "volume not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
