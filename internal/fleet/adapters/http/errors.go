// Package http is the Fleet context's REST adapter.
package http

import (
	"errors"
	"net/http"

	"platform-of-platform/internal/fleet/domain"
	"platform-of-platform/internal/platform/httpserver"
)

// writeFleetError is this context's own version of Execution's
// writeExecutionError - one shared domain-error-to-HTTP-status mapping,
// called from every handler below instead of repeating the same
// errors.Is chain in each one.
func writeFleetError(w http.ResponseWriter, err error, notFoundTitle string) {
	switch {
	case errors.Is(err, domain.ErrForbidden):
		httpserver.WriteProblem(w, http.StatusForbidden, "forbidden", "")
	case errors.Is(err, domain.ErrMachineNotFound), errors.Is(err, domain.ErrNetworkNotFound),
		errors.Is(err, domain.ErrVolumeNotFound), errors.Is(err, domain.ErrComposeFileNotFound),
		errors.Is(err, domain.ErrVariableNotFound), errors.Is(err, domain.ErrOperationNotFound),
		errors.Is(err, domain.ErrProjectNotFound):
		httpserver.WriteProblem(w, http.StatusNotFound, notFoundTitle, "")
	case errors.Is(err, domain.ErrNetworkInUse):
		httpserver.WriteProblem(w, http.StatusConflict, "network is still attached to a compose file", "")
	case errors.Is(err, domain.ErrVolumeInUse):
		httpserver.WriteProblem(w, http.StatusConflict, "volume is still attached to a compose file", "")
	case errors.Is(err, domain.ErrGlobalComposeFileExists):
		httpserver.WriteProblem(w, http.StatusConflict, "this organization already has a global compose file", "")
	case errors.Is(err, domain.ErrMachineHasHistory):
		httpserver.WriteProblem(w, http.StatusConflict, "machine has operation history and cannot be hard-deleted", "")
	case errors.Is(err, domain.ErrComposeFileHasHistory):
		httpserver.WriteProblem(w, http.StatusConflict, "compose file has operation history and cannot be deleted", "")
	case errors.Is(err, domain.ErrMachineNameTaken):
		httpserver.WriteProblem(w, http.StatusConflict, "a machine with this name already exists in this organization", "")
	case errors.Is(err, domain.ErrVariableKeyTaken):
		httpserver.WriteProblem(w, http.StatusConflict, "a variable with this key already exists on this compose file", "")
	default:
		var validationErr *domain.ValidationError
		if errors.As(err, &validationErr) {
			httpserver.WriteProblem(w, http.StatusBadRequest, "validation failed", validationErr.Error())
			return
		}
		httpserver.WriteProblem(w, http.StatusInternalServerError, "internal error", "")
	}
}
