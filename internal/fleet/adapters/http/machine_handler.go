package http

import (
	"encoding/json"
	"net/http"

	"platform-of-platform/internal/fleet/application"
	"platform-of-platform/internal/fleet/domain"
	"platform-of-platform/internal/platform/httpserver"
)

type createMachineRequest struct {
	Name              string `json:"name"`
	Host              string `json:"host"`
	SSHPort           int    `json:"ssh_port"`
	SSHUser           string `json:"ssh_user"`
	CredentialType    string `json:"credential_type"`
	CredentialMountID string `json:"credential_mount_id"`
	CredentialPath    string `json:"credential_path"`
	DeployBasePath    string `json:"deploy_base_path"`
}

type machineResponse struct {
	ID                string  `json:"id"`
	OrganizationID    string  `json:"organization_id"`
	Name              string  `json:"name"`
	Host              string  `json:"host"`
	SSHPort           int     `json:"ssh_port"`
	SSHUser           string  `json:"ssh_user"`
	CredentialType    string  `json:"credential_type"`
	CredentialMountID string  `json:"credential_mount_id"`
	CredentialPath    string  `json:"credential_path"`
	DeployBasePath    string  `json:"deploy_base_path"`
	ConnectionStatus  string  `json:"connection_status"`
	DockerStatus      string  `json:"docker_status"`
	LastCheckedAt     *string `json:"last_checked_at,omitempty"`
	Archived          bool    `json:"archived"`
	CreatedAt         string  `json:"created_at"`
}

func toMachineResponse(m *domain.Machine) machineResponse {
	resp := machineResponse{
		ID: m.ID, OrganizationID: m.OrganizationID, Name: m.Name, Host: m.Host, SSHPort: m.SSHPort, SSHUser: m.SSHUser,
		CredentialType: string(m.CredentialType), CredentialMountID: m.CredentialRef.MountID, CredentialPath: m.CredentialRef.Path,
		DeployBasePath: m.DeployBasePath, ConnectionStatus: string(m.ConnectionStatus), DockerStatus: string(m.DockerStatus),
		Archived: m.Archived, CreatedAt: m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if m.LastCheckedAt != nil {
		formatted := m.LastCheckedAt.Format("2006-01-02T15:04:05Z07:00")
		resp.LastCheckedAt = &formatted
	}
	return resp
}

func CreateMachineHandler(svc *application.CreateMachineService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		var req createMachineRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		machine, err := svc.Execute(r.Context(), application.CreateMachineInput{
			OrganizationID: r.PathValue("id"), RequestingUserID: userID, Name: req.Name, Host: req.Host,
			SSHPort: req.SSHPort, SSHUser: req.SSHUser, CredentialType: req.CredentialType,
			CredentialMountID: req.CredentialMountID, CredentialPath: req.CredentialPath, DeployBasePath: req.DeployBasePath,
		})
		if err != nil {
			writeFleetError(w, err, "")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(toMachineResponse(machine))
	}
}

func ListMachinesHandler(svc *application.ListMachinesService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		includeArchived := r.URL.Query().Get("include_archived") == "true"

		machines, err := svc.Execute(r.Context(), r.PathValue("id"), userID, includeArchived)
		if err != nil {
			writeFleetError(w, err, "")
			return
		}
		responses := make([]machineResponse, 0, len(machines))
		for _, m := range machines {
			responses = append(responses, toMachineResponse(m))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"data": responses})
	}
}

func GetMachineHandler(svc *application.GetMachineService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		machine, err := svc.Execute(r.Context(), r.PathValue("id"), userID, r.PathValue("machineID"))
		if err != nil {
			writeFleetError(w, err, "machine not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(toMachineResponse(machine))
	}
}

type updateMachineRequest struct {
	SSHUser           *string `json:"ssh_user"`
	CredentialType    *string `json:"credential_type"`
	CredentialMountID *string `json:"credential_mount_id"`
	CredentialPath    *string `json:"credential_path"`
	DeployBasePath    *string `json:"deploy_base_path"`
}

func UpdateMachineHandler(svc *application.UpdateMachineService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		var req updateMachineRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		machine, err := svc.Execute(r.Context(), application.UpdateMachineInput{
			OrganizationID: r.PathValue("id"), RequestingUserID: userID, MachineID: r.PathValue("machineID"),
			SSHUser: req.SSHUser, CredentialType: req.CredentialType, CredentialMountID: req.CredentialMountID,
			CredentialPath: req.CredentialPath, DeployBasePath: req.DeployBasePath,
		})
		if err != nil {
			writeFleetError(w, err, "machine not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(toMachineResponse(machine))
	}
}

func ArchiveMachineHandler(svc *application.ArchiveMachineService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		archived, err := svc.Execute(r.Context(), r.PathValue("id"), userID, r.PathValue("machineID"))
		if err != nil {
			writeFleetError(w, err, "machine not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"archived": archived})
	}
}

type testMachineConnectionRequest struct {
	Host              string `json:"host"`
	SSHPort           int    `json:"ssh_port"`
	SSHUser           string `json:"ssh_user"`
	CredentialType    string `json:"credential_type"`
	CredentialMountID string `json:"credential_mount_id"`
	CredentialPath    string `json:"credential_path"`
}

func TestMachineConnectionHandler(svc *application.TestMachineConnectionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		var req testMachineConnectionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		connStatus, dockerStatus, err := svc.Execute(r.Context(), application.TestMachineConnectionInput{
			OrganizationID: r.PathValue("id"), RequestingUserID: userID, Host: req.Host, SSHPort: req.SSHPort,
			SSHUser: req.SSHUser, CredentialType: req.CredentialType, CredentialMountID: req.CredentialMountID, CredentialPath: req.CredentialPath,
		})
		if err != nil {
			writeFleetError(w, err, "")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"connection_status": string(connStatus), "docker_status": string(dockerStatus)})
	}
}

func CheckMachineConnectionHandler(svc *application.CheckMachineConnectionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		machine, err := svc.Execute(r.Context(), r.PathValue("id"), userID, r.PathValue("machineID"))
		if err != nil {
			writeFleetError(w, err, "machine not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(toMachineResponse(machine))
	}
}
