package http

import (
	"encoding/json"
	"net/http"

	"platform-of-platform/internal/fleet/application"
	"platform-of-platform/internal/fleet/domain"
	"platform-of-platform/internal/platform/httpserver"
)

type createVariableRequest struct {
	Key            string `json:"key"`
	VarType        string `json:"var_type"`
	Value          string `json:"value"`
	SecretMountID  string `json:"secret_mount_id"`
	SecretPath     string `json:"secret_path"`
	FileTargetPath string `json:"file_target_path"`
}

type secretRefResponse struct {
	MountID string `json:"mount_id"`
	Path    string `json:"path"`
}

type variableResponse struct {
	ID             string             `json:"id"`
	OrganizationID string             `json:"organization_id"`
	ComposeFileID  string             `json:"compose_file_id"`
	Key            string             `json:"key"`
	VarType        string             `json:"var_type"`
	Value          *string            `json:"value,omitempty"`
	SecretRef      *secretRefResponse `json:"secret_ref,omitempty"`
	FileTargetPath string             `json:"file_target_path,omitempty"`
	CreatedAt      string             `json:"created_at"`
}

func toVariableResponse(v *domain.Variable) variableResponse {
	resp := variableResponse{
		ID: v.ID, OrganizationID: v.OrganizationID, ComposeFileID: v.ComposeFileID, Key: v.Key,
		VarType: string(v.VarType), FileTargetPath: v.FileTargetPath, CreatedAt: v.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if v.SecretRef != nil {
		resp.SecretRef = &secretRefResponse{MountID: v.SecretRef.MountID, Path: v.SecretRef.Path}
	} else {
		resp.Value = &v.Value
	}
	return resp
}

func CreateVariableHandler(svc *application.CreateVariableService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		var req createVariableRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		variable, err := svc.Execute(r.Context(), application.CreateVariableInput{
			OrganizationID: r.PathValue("id"), RequestingUserID: userID, ComposeFileID: r.PathValue("composeFileID"),
			Key: req.Key, VarType: req.VarType, Value: req.Value, SecretMountID: req.SecretMountID,
			SecretPath: req.SecretPath, FileTargetPath: req.FileTargetPath,
		})
		if err != nil {
			writeFleetError(w, err, "")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(toVariableResponse(variable))
	}
}

type createSecretVariableRequest struct {
	Key     string `json:"key"`
	MountID string `json:"mount_id"`
	Value   string `json:"value"`
}

func CreateSecretVariableHandler(svc *application.CreateSecretVariableService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		var req createSecretVariableRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		variable, err := svc.Execute(r.Context(), application.CreateSecretVariableInput{
			OrganizationID: r.PathValue("id"), RequestingUserID: userID, ComposeFileID: r.PathValue("composeFileID"),
			Key: req.Key, MountID: req.MountID, Value: req.Value,
		})
		if err != nil {
			writeFleetError(w, err, "")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(toVariableResponse(variable))
	}
}

func RevealVariableHandler(svc *application.RevealVariableService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		value, err := svc.Execute(r.Context(), r.PathValue("id"), userID, r.PathValue("variableID"))
		if err != nil {
			writeFleetError(w, err, "variable not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"value": value})
	}
}

func ListVariablesHandler(svc *application.ListVariablesService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		variables, err := svc.Execute(r.Context(), r.PathValue("id"), userID, r.PathValue("composeFileID"))
		if err != nil {
			writeFleetError(w, err, "")
			return
		}
		responses := make([]variableResponse, 0, len(variables))
		for _, v := range variables {
			responses = append(responses, toVariableResponse(v))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"data": responses})
	}
}

type updateVariableRequest struct {
	Value          *string `json:"value"`
	SecretMountID  *string `json:"secret_mount_id"`
	SecretPath     *string `json:"secret_path"`
	FileTargetPath *string `json:"file_target_path"`
}

func UpdateVariableHandler(svc *application.UpdateVariableService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		var req updateVariableRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		variable, err := svc.Execute(r.Context(), application.UpdateVariableInput{
			OrganizationID: r.PathValue("id"), RequestingUserID: userID, VariableID: r.PathValue("variableID"),
			Value: req.Value, SecretMountID: req.SecretMountID, SecretPath: req.SecretPath, FileTargetPath: req.FileTargetPath,
		})
		if err != nil {
			writeFleetError(w, err, "variable not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(toVariableResponse(variable))
	}
}

func DeleteVariableHandler(svc *application.DeleteVariableService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		if err := svc.Execute(r.Context(), r.PathValue("id"), userID, r.PathValue("variableID")); err != nil {
			writeFleetError(w, err, "variable not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
