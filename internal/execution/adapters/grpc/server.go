package grpc

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "platform-of-platform/internal/execution/adapters/grpc/proto"
)

// StatusReportHandler is called for every real ReportJobStatus RPC a
// Worker makes - registered from main.go, implemented by
// execution/application's WorkerReportService. Kept as a plain function
// type (not importing execution/application's own interfaces here) so
// this adapter package doesn't need to depend on the application
// package just to declare the callback shape.
type StatusReportHandler func(ctx context.Context, organizationID, runID, workspaceID, status, logLine, errorMessage string) error

type Server struct {
	pb.UnimplementedWorkerServiceServer
	registry       *Registry
	onStatusReport StatusReportHandler
}

func NewServer(registry *Registry, onStatusReport StatusReportHandler) *Server {
	return &Server{registry: registry, onStatusReport: onStatusReport}
}

func (s *Server) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	if req.WorkerId == "" {
		return nil, status.Error(codes.InvalidArgument, "worker_id is required")
	}
	s.registry.register(req.WorkerId, req.SupportedEngines)
	return &pb.RegisterResponse{Accepted: true}, nil
}

// StreamJobs blocks for the Worker's entire connected lifetime - real
// gRPC server-streaming, per docs/architecture/17-workers.md §1's "the
// Worker keeps this long-lived connection open." Deregisters the
// Worker on disconnect so the Run Dispatcher stops routing new work to
// a channel nobody is reading anymore.
func (s *Server) StreamJobs(req *pb.StreamJobsRequest, stream pb.WorkerService_StreamJobsServer) error {
	s.registry.mu.RLock()
	entry, ok := s.registry.workers[req.WorkerId]
	s.registry.mu.RUnlock()
	if !ok {
		return status.Errorf(codes.FailedPrecondition, "worker %s must call Register before StreamJobs", req.WorkerId)
	}
	defer s.registry.deregister(req.WorkerId)

	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case cmd := <-entry.jobs:
			if err := stream.Send(cmd); err != nil {
				return err
			}
		}
	}
}

func (s *Server) ReportJobStatus(ctx context.Context, report *pb.JobStatusReport) (*pb.Ack, error) {
	if err := s.onStatusReport(ctx, report.OrganizationId, report.RunId, report.WorkspaceId, report.Status, report.LogLine, report.ErrorMessage); err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	return &pb.Ack{Ok: true}, nil
}
