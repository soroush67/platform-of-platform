package http

import (
	"encoding/json"
	"io"
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
	CredentialStorage string `json:"credential_storage"`
	CredentialMountID string `json:"credential_mount_id"`
	CredentialPath    string `json:"credential_path"`
	// CredentialSecret is the plaintext SSH secret, only used (and only
	// ever read here, never returned) when credential_storage is "local".
	CredentialSecret string `json:"credential_secret"`
	DeployBasePath   string `json:"deploy_base_path"`
}

type machineResponse struct {
	ID                string  `json:"id"`
	OrganizationID    string  `json:"organization_id"`
	Name              string  `json:"name"`
	Host              string  `json:"host"`
	SSHPort           int     `json:"ssh_port"`
	SSHUser           string  `json:"ssh_user"`
	CredentialType    string  `json:"credential_type"`
	CredentialStorage string  `json:"credential_storage"`
	CredentialMountID string  `json:"credential_mount_id,omitempty"`
	CredentialPath    string  `json:"credential_path,omitempty"`
	DeployBasePath    string  `json:"deploy_base_path"`
	ConnectionStatus  string  `json:"connection_status"`
	DockerStatus      string  `json:"docker_status"`
	LastCheckedAt     *string `json:"last_checked_at,omitempty"`
	Archived          bool    `json:"archived"`
	CreatedAt         string  `json:"created_at"`
}

// toMachineResponse deliberately never includes the sealed local-storage
// bytes (EncryptedCredential/CredentialNonce/CredentialSalt) - same
// "never returned" posture secretMountResponse already established for
// SecretMount's own sealed secret_id.
func toMachineResponse(m *domain.Machine) machineResponse {
	resp := machineResponse{
		ID: m.ID, OrganizationID: m.OrganizationID, Name: m.Name, Host: m.Host, SSHPort: m.SSHPort, SSHUser: m.SSHUser,
		CredentialType: string(m.CredentialType), CredentialStorage: string(m.CredentialStorage),
		CredentialMountID: m.CredentialRef.MountID, CredentialPath: m.CredentialRef.Path,
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
			SSHPort: req.SSHPort, SSHUser: req.SSHUser, CredentialType: req.CredentialType, CredentialStorage: req.CredentialStorage,
			CredentialMountID: req.CredentialMountID, CredentialPath: req.CredentialPath, CredentialSecret: req.CredentialSecret,
			DeployBasePath: req.DeployBasePath,
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
	CredentialSecret  *string `json:"credential_secret"`
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
			CredentialPath: req.CredentialPath, CredentialSecret: req.CredentialSecret, DeployBasePath: req.DeployBasePath,
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
		if err := svc.Execute(r.Context(), r.PathValue("id"), userID, r.PathValue("machineID")); err != nil {
			writeFleetError(w, err, "machine not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func UnarchiveMachineHandler(svc *application.UnarchiveMachineService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		if err := svc.Execute(r.Context(), r.PathValue("id"), userID, r.PathValue("machineID")); err != nil {
			writeFleetError(w, err, "machine not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// DeleteMachineHandler implements `POST /api/v1/orgs/{id}/machines/
// {machineID}/hard-delete` - same `/hard-delete` suffix convention
// Organization's own genuine hard delete already uses (as opposed to
// the archive-shaped `DELETE /machines/{machineID}` above), gated by
// machine:delete. A real 409 (via writeFleetError's ErrMachineHasHistory
// mapping) if the Machine has real Operation history - no silent
// archive fallback.
func DeleteMachineHandler(svc *application.DeleteMachineService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		if err := svc.Execute(r.Context(), r.PathValue("id"), userID, r.PathValue("machineID")); err != nil {
			writeFleetError(w, err, "machine not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

type duplicateMachineRequest struct {
	// Name, if given, is the exact name to create the clone under (a
	// real 409 on conflict, not silently renamed) - the UI always sends
	// the "{name} (copy)" it showed the operator to confirm/edit first.
	// An empty body/omitted name still works (auto-generated unique
	// name), for any caller that doesn't want to pick one.
	Name string `json:"name"`
}

// DuplicateMachineHandler implements `POST /api/v1/orgs/{id}/machines/
// {machineID}/duplicate` - a real one-click clone, gated by
// machine:manage (the same tier Create already uses - this is a
// create, not a delete/destructive action).
func DuplicateMachineHandler(svc *application.DuplicateMachineService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		var req duplicateMachineRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		machine, err := svc.Execute(r.Context(), application.DuplicateMachineInput{
			OrganizationID: r.PathValue("id"), RequestingUserID: userID, MachineID: r.PathValue("machineID"), Name: req.Name,
		})
		if err != nil {
			writeFleetError(w, err, "machine not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(toMachineResponse(machine))
	}
}

type testMachineConnectionRequest struct {
	Host              string `json:"host"`
	SSHPort           int    `json:"ssh_port"`
	SSHUser           string `json:"ssh_user"`
	CredentialType    string `json:"credential_type"`
	CredentialStorage string `json:"credential_storage"`
	CredentialMountID string `json:"credential_mount_id"`
	CredentialPath    string `json:"credential_path"`
	CredentialSecret  string `json:"credential_secret"`
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
			SSHUser: req.SSHUser, CredentialType: req.CredentialType, CredentialStorage: req.CredentialStorage,
			CredentialMountID: req.CredentialMountID, CredentialPath: req.CredentialPath, CredentialSecret: req.CredentialSecret,
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
