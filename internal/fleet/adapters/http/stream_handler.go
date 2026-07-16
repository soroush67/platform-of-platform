package http

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"platform-of-platform/internal/fleet/adapters/redisstream"
	"platform-of-platform/internal/fleet/application"
	"platform-of-platform/internal/fleet/domain"
	"platform-of-platform/internal/platform/httpserver"
)

// operationRecheckInterval is the defensive re-check StreamOperationHandler
// runs alongside its Redis subscription - closes a real, unfixed race in
// the ported Python original (its own WebSocket handler could miss the
// "__END__" sentinel if the Operation finished between its initial
// status check and the pubsub.subscribe call, hanging until the client
// gave up). Re-polling GetOperationService periodically means a missed
// pub/sub message just gets caught on the next tick instead.
const operationRecheckInterval = 2 * time.Second

// StreamOperationHandler implements
// GET /api/v1/orgs/{id}/operations/{operationID}/stream - real
// Server-Sent-Events (text/event-stream), not the browser's native
// EventSource (see the Fleet plan's own decision #4: EventSource can't
// set the Authorization header, which would force the access token
// into a URL query param - a real, avoidable security relaxation). The
// client instead uses fetch()+ReadableStream, hand-parsing the same
// wire format (web/src/api/hooks/useFleet.ts's own useOperationLogStream).
func StreamOperationHandler(svc *application.GetOperationService, redisClient *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := httpserver.UserIDFromContext(r.Context())
		if !ok {
			httpserver.WriteProblem(w, http.StatusUnauthorized, "authentication required", "")
			return
		}
		orgID, operationID := r.PathValue("id"), r.PathValue("operationID")

		operation, err := svc.Execute(r.Context(), orgID, userID, operationID)
		if err != nil {
			writeFleetError(w, err, "operation not found")
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			httpserver.WriteProblem(w, http.StatusInternalServerError, "streaming unsupported", "")
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)

		// Already terminal - replay the persisted (already-scrubbed)
		// output as SSE frames and close. Folds the ported Python
		// original's separate GET .../output REST fallback into this
		// one route instead of a second endpoint.
		if operation.Status == domain.OperationStatusSuccess || operation.Status == domain.OperationStatusFailed {
			writeSSEData(w, flusher, operation.Output)
			writeSSEEvent(w, flusher, "done", fmt.Sprintf(`{"exit_code":%d}`, intOrZero(operation.ExitCode)))
			return
		}

		ctx := r.Context()
		sub := redisstream.Subscribe(ctx, redisClient, operationID)
		defer sub.Close()
		ch := sub.Channel()

		recheck := time.NewTicker(operationRecheckInterval)
		defer recheck.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				var logMsg redisstream.LogMessage
				if err := json.Unmarshal([]byte(msg.Payload), &logMsg); err != nil {
					continue
				}
				if logMsg.Done {
					writeSSEEvent(w, flusher, "done", fmt.Sprintf(`{"exit_code":%d}`, intOrZero(logMsg.ExitCode)))
					return
				}
				writeSSEData(w, flusher, logMsg.Line)
			case <-recheck.C:
				current, err := svc.Execute(ctx, orgID, userID, operationID)
				if err != nil {
					continue
				}
				if current.Status == domain.OperationStatusSuccess || current.Status == domain.OperationStatusFailed {
					writeSSEEvent(w, flusher, "done", fmt.Sprintf(`{"exit_code":%d}`, intOrZero(current.ExitCode)))
					return
				}
			}
		}
	}
}

func writeSSEData(w http.ResponseWriter, flusher http.Flusher, data string) {
	scanner := bufio.NewScanner(strings.NewReader(data))
	for scanner.Scan() {
		fmt.Fprintf(w, "data: %s\n\n", scanner.Text())
	}
	if data == "" {
		fmt.Fprint(w, "data: \n\n")
	}
	flusher.Flush()
}

func writeSSEEvent(w http.ResponseWriter, flusher http.Flusher, event, data string) {
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	flusher.Flush()
}

func intOrZero(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
