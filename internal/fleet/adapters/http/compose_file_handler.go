package http

import (
	"encoding/json"
	"net/http"

	"platform-of-platform/internal/fleet/application"
	"platform-of-platform/internal/fleet/domain"
	"platform-of-platform/internal/platform/httpserver"
)

type createComposeFileRequest struct {
	Name           string `json:"name"`
	ComposeContent string `json:"compose_content"`
	IsGlobal       bool   `json:"is_global"`
}

type composeFileResponse struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	Name           string `json:"name"`
	IsGlobal       bool   `json:"is_global"`
	ComposeContent string `json:"compose_content"`
	CreatedBy      string `json:"created_by"`
	CreatedAt      string `json:"created_at"`
}

func toComposeFileResponse(c *domain.ComposeFile) composeFileResponse {
	return composeFileResponse{
		ID: c.ID, OrganizationID: c.OrganizationID, Name: c.Name, IsGlobal: c.IsGlobal,
		ComposeContent: c.ComposeContent, CreatedBy: c.CreatedBy, CreatedAt: c.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func CreateComposeFileHandler(svc *application.CreateComposeFileService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		var req createComposeFileRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		composeFile, err := svc.Execute(r.Context(), application.CreateComposeFileInput{
			OrganizationID: r.PathValue("id"), RequestingUserID: userID, Name: req.Name,
			ComposeContent: req.ComposeContent, IsGlobal: req.IsGlobal,
		})
		if err != nil {
			writeFleetError(w, err, "")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(toComposeFileResponse(composeFile))
	}
}

func ListComposeFilesHandler(svc *application.ListComposeFilesService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		files, err := svc.Execute(r.Context(), r.PathValue("id"), userID)
		if err != nil {
			writeFleetError(w, err, "")
			return
		}
		responses := make([]composeFileResponse, 0, len(files))
		for _, c := range files {
			responses = append(responses, toComposeFileResponse(c))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"data": responses})
	}
}

func GetComposeFileHandler(svc *application.GetComposeFileService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		composeFile, err := svc.Execute(r.Context(), r.PathValue("id"), userID, r.PathValue("composeFileID"))
		if err != nil {
			writeFleetError(w, err, "compose file not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(toComposeFileResponse(composeFile))
	}
}

type updateComposeFileContentRequest struct {
	ComposeContent string `json:"compose_content"`
}

func UpdateComposeFileContentHandler(svc *application.UpdateComposeFileContentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		var req updateComposeFileContentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}
		if err := svc.Execute(r.Context(), r.PathValue("id"), userID, r.PathValue("composeFileID"), req.ComposeContent); err != nil {
			writeFleetError(w, err, "compose file not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ---- attachments ----

type attachNetworkRequest struct {
	NetworkID string `json:"network_id"`
}

func AttachNetworkHandler(svc *application.AttachmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		var req attachNetworkRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}
		if err := svc.AttachNetwork(r.Context(), r.PathValue("id"), userID, r.PathValue("composeFileID"), req.NetworkID); err != nil {
			writeFleetError(w, err, "")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func DetachNetworkHandler(svc *application.AttachmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		if err := svc.DetachNetwork(r.Context(), r.PathValue("id"), userID, r.PathValue("composeFileID"), r.PathValue("networkID")); err != nil {
			writeFleetError(w, err, "")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func ListComposeFileNetworksHandler(svc *application.AttachmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		networks, err := svc.ListNetworks(r.Context(), r.PathValue("id"), userID, r.PathValue("composeFileID"))
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

type attachVolumeRequest struct {
	VolumeID      string `json:"volume_id"`
	ContainerPath string `json:"container_path"`
}

func AttachVolumeHandler(svc *application.AttachmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		var req attachVolumeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}
		if err := svc.AttachVolume(r.Context(), r.PathValue("id"), userID, r.PathValue("composeFileID"), req.VolumeID, req.ContainerPath); err != nil {
			writeFleetError(w, err, "")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func DetachVolumeHandler(svc *application.AttachmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		if err := svc.DetachVolume(r.Context(), r.PathValue("id"), userID, r.PathValue("composeFileID"), r.PathValue("volumeID")); err != nil {
			writeFleetError(w, err, "")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

type volumeAttachmentResponse struct {
	Volume        volumeResponse `json:"volume"`
	ContainerPath string         `json:"container_path"`
}

func ListComposeFileVolumesHandler(svc *application.AttachmentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		views, err := svc.ListVolumes(r.Context(), r.PathValue("id"), userID, r.PathValue("composeFileID"))
		if err != nil {
			writeFleetError(w, err, "")
			return
		}
		responses := make([]volumeAttachmentResponse, 0, len(views))
		for _, v := range views {
			responses = append(responses, volumeAttachmentResponse{Volume: toVolumeResponse(v.Volume), ContainerPath: v.ContainerPath})
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"data": responses})
	}
}
