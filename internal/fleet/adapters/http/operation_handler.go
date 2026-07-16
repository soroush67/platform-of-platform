package http

import (
	"encoding/json"
	"net/http"

	"platform-of-platform/internal/fleet/application"
	"platform-of-platform/internal/fleet/domain"
	"platform-of-platform/internal/platform/httpserver"
)

type triggerOperationRequest struct {
	ComposeFileID string `json:"compose_file_id"`
	MachineID     string `json:"machine_id"`
	OperationType string `json:"operation_type"`
}

type operationResponse struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organization_id"`
	ComposeFileID  string  `json:"compose_file_id"`
	MachineID      string  `json:"machine_id"`
	OperationType  string  `json:"operation_type"`
	Status         string  `json:"status"`
	TriggeredBy    string  `json:"triggered_by"`
	CreatedAt      string  `json:"created_at"`
	StartedAt      *string `json:"started_at,omitempty"`
	FinishedAt     *string `json:"finished_at,omitempty"`
	ExitCode       *int    `json:"exit_code,omitempty"`
	Output         string  `json:"output,omitempty"`
}

const timeLayout = "2006-01-02T15:04:05Z07:00"

func toOperationResponse(o *domain.Operation) operationResponse {
	resp := operationResponse{
		ID: o.ID, OrganizationID: o.OrganizationID, ComposeFileID: o.ComposeFileID, MachineID: o.MachineID,
		OperationType: string(o.OperationType), Status: string(o.Status), TriggeredBy: o.TriggeredBy,
		CreatedAt: o.CreatedAt.Format(timeLayout), ExitCode: o.ExitCode, Output: o.Output,
	}
	if o.StartedAt != nil {
		f := o.StartedAt.Format(timeLayout)
		resp.StartedAt = &f
	}
	if o.FinishedAt != nil {
		f := o.FinishedAt.Format(timeLayout)
		resp.FinishedAt = &f
	}
	return resp
}

func TriggerOperationHandler(svc *application.TriggerOperationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		var req triggerOperationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteProblem(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}

		operation, err := svc.Execute(r.Context(), application.TriggerOperationInput{
			OrganizationID: r.PathValue("id"), RequestingUserID: userID, ComposeFileID: req.ComposeFileID,
			MachineID: req.MachineID, OperationType: req.OperationType,
		})
		if err != nil {
			writeFleetError(w, err, "")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(toOperationResponse(operation))
	}
}

func ListOperationsHandler(svc *application.ListOperationsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		operations, err := svc.Execute(r.Context(), r.PathValue("id"), userID, r.URL.Query().Get("compose_file_id"), r.URL.Query().Get("machine_id"))
		if err != nil {
			writeFleetError(w, err, "")
			return
		}
		responses := make([]operationResponse, 0, len(operations))
		for _, o := range operations {
			responses = append(responses, toOperationResponse(o))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"data": responses})
	}
}

func GetOperationHandler(svc *application.GetOperationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		operation, err := svc.Execute(r.Context(), r.PathValue("id"), userID, r.PathValue("operationID"))
		if err != nil {
			writeFleetError(w, err, "operation not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(toOperationResponse(operation))
	}
}
