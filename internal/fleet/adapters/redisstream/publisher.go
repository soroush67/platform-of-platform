// Package redisstream is DeployExecutor's real live-log-streaming
// transport - the exact Redis Pub/Sub channel-per-id pattern already
// proven in internal/execution/adapters/grpc/registry.go's own
// CancelJob/SubscribeCancelForwarding (cancelChannel(instanceID) there,
// operationChannel(operationID) here), reused rather than inventing a
// second mechanism.
package redisstream

import (
	"context"
	"encoding/json"

	"github.com/redis/go-redis/v9"
)

// LogMessage is a structured JSON envelope, not the ported Python
// original's own bare "__END__" magic string - avoids any risk
// (however small) of colliding with a genuine line of real docker
// output, and lets the SSE handler decode structurally instead of
// doing string equality.
type LogMessage struct {
	Line     string `json:"line,omitempty"`
	Done     bool   `json:"done,omitempty"`
	ExitCode *int   `json:"exit_code,omitempty"`
}

func operationChannel(operationID string) string { return "fleet:operation:" + operationID + ":log" }

type Publisher struct {
	redis *redis.Client
}

func NewPublisher(redisClient *redis.Client) *Publisher {
	return &Publisher{redis: redisClient}
}

func (p *Publisher) PublishLine(ctx context.Context, operationID, line string) error {
	payload, err := json.Marshal(LogMessage{Line: line})
	if err != nil {
		return err
	}
	return p.redis.Publish(ctx, operationChannel(operationID), payload).Err()
}

func (p *Publisher) PublishEnd(ctx context.Context, operationID string, exitCode int) error {
	payload, err := json.Marshal(LogMessage{Done: true, ExitCode: &exitCode})
	if err != nil {
		return err
	}
	return p.redis.Publish(ctx, operationChannel(operationID), payload).Err()
}

// Subscribe is the SSE handler's own half - a thin wrapper so
// adapters/http/stream_handler.go doesn't need its own *redis.Client
// import, matching this codebase's "narrow port per real dependency"
// posture elsewhere.
func Subscribe(ctx context.Context, redisClient *redis.Client, operationID string) *redis.PubSub {
	return redisClient.Subscribe(ctx, operationChannel(operationID))
}
