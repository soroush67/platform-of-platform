package grpc_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	executiongrpc "platform-of-platform/internal/execution/adapters/grpc"
	pb "platform-of-platform/internal/execution/adapters/grpc/proto"
	"platform-of-platform/internal/platform/dbtest"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeStream implements pb.WorkerService_StreamJobsServer (a
// grpc.ServerStreamingServer[WorkerCommand] alias) - the real interface
// Server.StreamJobs is handed by the actual gRPC runtime, faked here
// only because standing up a real network listener isn't needed to
// exercise Server's own logic (which only ever calls Context() and
// Send() on it).
type fakeStream struct {
	ctx      context.Context
	received chan *pb.WorkerCommand
}

func newFakeStream(ctx context.Context) *fakeStream {
	return &fakeStream{ctx: ctx, received: make(chan *pb.WorkerCommand, 16)}
}

func (f *fakeStream) Send(cmd *pb.WorkerCommand) error {
	f.received <- cmd
	return nil
}
func (f *fakeStream) SetHeader(metadata.MD) error  { return nil }
func (f *fakeStream) SendHeader(metadata.MD) error { return nil }
func (f *fakeStream) SetTrailer(metadata.MD)       {}
func (f *fakeStream) Context() context.Context     { return f.ctx }
func (f *fakeStream) SendMsg(m any) error          { return nil }
func (f *fakeStream) RecvMsg(m any) error          { return nil }

func newTestServer(t *testing.T, onStatusReport executiongrpc.StatusReportHandler) (*executiongrpc.Server, *executiongrpc.Registry) {
	t.Helper()
	redisClient := dbtest.RedisClient(t)
	registry := executiongrpc.NewRegistry("instance-"+uuid.NewString(), redisClient, discardLogger())
	return executiongrpc.NewServer(registry, onStatusReport), registry
}

func TestServer_Register_RequiresWorkerID(t *testing.T) {
	server, _ := newTestServer(t, nil)

	_, err := server.Register(context.Background(), &pb.RegisterRequest{WorkerId: ""})
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.InvalidArgument {
		t.Fatalf("expected an InvalidArgument status for a missing worker_id, got: %v", err)
	}
}

func TestServer_Register_Succeeds(t *testing.T) {
	server, registry := newTestServer(t, nil)

	resp, err := server.Register(context.Background(), &pb.RegisterRequest{
		WorkerId: "worker-1", SupportedEngines: []string{"compose"},
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if !resp.Accepted {
		t.Error("expected a successful Register to be accepted")
	}

	// Registered workers are only observable through Registry's own
	// exported behavior (Dispatch) - a real end-to-end confirmation that
	// this RPC actually reached the Registry, not just returned success.
	dispatched, err := registry.Dispatch(context.Background(), uuid.NewString(), "org-1", "ws-1", "compose", "bundle", "cred")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !dispatched {
		t.Error("expected the just-registered worker to be a real Dispatch target")
	}
}

func TestServer_StreamJobs_RequiresPriorRegister(t *testing.T) {
	server, _ := newTestServer(t, nil)
	stream := newFakeStream(context.Background())

	err := server.StreamJobs(&pb.StreamJobsRequest{WorkerId: "never-registered"}, stream)
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.FailedPrecondition {
		t.Fatalf("expected a FailedPrecondition status for StreamJobs without a prior Register, got: %v", err)
	}
}

// TestServer_StreamJobs_DeliversDispatchedJobsAndDeregistersOnDisconnect
// is the real regression test for StreamJobs's own doc comment: a real
// dispatched job reaches this specific stream, and disconnecting (the
// stream's context being canceled, the real signal a dropped Worker
// connection produces) deregisters the worker so the Run Dispatcher
// stops routing new work to a channel nobody is reading anymore.
func TestServer_StreamJobs_DeliversDispatchedJobsAndDeregistersOnDisconnect(t *testing.T) {
	server, registry := newTestServer(t, nil)

	if _, err := server.Register(context.Background(), &pb.RegisterRequest{
		WorkerId: "worker-1", SupportedEngines: []string{"compose"},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	streamCtx, streamCancel := context.WithCancel(context.Background())
	stream := newFakeStream(streamCtx)
	streamDone := make(chan error, 1)
	go func() { streamDone <- server.StreamJobs(&pb.StreamJobsRequest{WorkerId: "worker-1"}, stream) }()

	runID := uuid.NewString()
	dispatched, err := registry.Dispatch(context.Background(), runID, "org-1", "ws-1", "compose", "bundle", "cred")
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !dispatched {
		t.Fatal("expected Dispatch to find worker-1")
	}

	select {
	case cmd := <-stream.received:
		if cmd.GetJobAssignment() == nil || cmd.GetJobAssignment().RunId != runID {
			t.Fatalf("expected this exact stream to receive the JobAssignment for run %s, got %+v", runID, cmd)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for StreamJobs to deliver the dispatched job to this stream")
	}

	streamCancel()
	select {
	case err := <-streamDone:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected StreamJobs to return context.Canceled on disconnect, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for StreamJobs to return after its context was canceled")
	}

	// The real proof deregister() ran: a fresh Dispatch for worker-1's
	// engine must no longer find it.
	dispatched, err = registry.Dispatch(context.Background(), uuid.NewString(), "org-1", "ws-1", "compose", "bundle", "cred")
	if err != nil {
		t.Fatalf("Dispatch (after disconnect): %v", err)
	}
	if dispatched {
		t.Error("expected worker-1 to be deregistered after its stream disconnected")
	}
}

func TestServer_ReportJobStatus_CallsHandlerAndAcks(t *testing.T) {
	var gotOrgID, gotRunID, gotWorkspaceID, gotStatus, gotLog, gotErrMsg string
	handler := func(ctx context.Context, organizationID, runID, workspaceID, status, logLine, errorMessage string) error {
		gotOrgID, gotRunID, gotWorkspaceID, gotStatus, gotLog, gotErrMsg = organizationID, runID, workspaceID, status, logLine, errorMessage
		return nil
	}
	server, _ := newTestServer(t, handler)

	report := &pb.JobStatusReport{
		OrganizationId: "org-1", RunId: "run-1", WorkspaceId: "ws-1",
		Status: "applied", LogLine: "done", ErrorMessage: "",
	}
	ack, err := server.ReportJobStatus(context.Background(), report)
	if err != nil {
		t.Fatalf("ReportJobStatus: %v", err)
	}
	if !ack.Ok {
		t.Error("expected a successful report to be acked")
	}
	if gotOrgID != "org-1" || gotRunID != "run-1" || gotWorkspaceID != "ws-1" || gotStatus != "applied" || gotLog != "done" || gotErrMsg != "" {
		t.Errorf("expected the handler to receive the report's own fields verbatim, got org=%q run=%q ws=%q status=%q log=%q err=%q",
			gotOrgID, gotRunID, gotWorkspaceID, gotStatus, gotLog, gotErrMsg)
	}
}

func TestServer_ReportJobStatus_HandlerErrorMapsToInternal(t *testing.T) {
	handler := func(ctx context.Context, organizationID, runID, workspaceID, status, logLine, errorMessage string) error {
		return errors.New("boom")
	}
	server, _ := newTestServer(t, handler)

	_, err := server.ReportJobStatus(context.Background(), &pb.JobStatusReport{RunId: "run-1"})
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.Internal {
		t.Fatalf("expected an Internal status when the handler fails, got: %v", err)
	}
}
