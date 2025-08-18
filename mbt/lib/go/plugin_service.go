package mbt

import (
	"context"
	"fmt"
	"testing"

	pb "github.com/fizzbee-io/fizzbee/mbt/lib/go/internalpb"
)

// FizzBeeMbtPluginServer implements the gRPC service.
type FizzBeeMbtPluginServer struct {
	pb.UnimplementedFizzBeeMbtPluginServiceServer
	test    *testing.T // Reference to the test instance
	model   Model
	actions map[string]map[string]ActionFunc // Registry of actions by role
}

// NewFizzBeeMbtPluginServer creates a new server instance
func NewFizzBeeMbtPluginServer(t *testing.T, m Model, actionsRegistry map[string]map[string]ActionFunc) *FizzBeeMbtPluginServer {
	return &FizzBeeMbtPluginServer{test: t, model: m, actions: actionsRegistry}
}

// Init is the RPC handler for initializing the model for each test.
func (s *FizzBeeMbtPluginServer) Init(
	ctx context.Context,
	req *pb.InitRequest,
) (*pb.InitResponse, error) {
	// Call the model's Init method
	if err := s.model.Init(); err != nil {
		return &pb.InitResponse{
			Status: &pb.Status{
				Code:    pb.StatusCode_STATUS_EXECUTION_FAILED,
				Message: fmt.Sprintf("Init failed: %v", err),
			},
		}, nil
	}

	return &pb.InitResponse{
		Status: &pb.Status{
			Code:    pb.StatusCode_STATUS_OK,
			Message: "Initialization successful",
		},
	}, nil
}

// Cleanup is the RPC handler for cleaning up the model after each test.
func (s *FizzBeeMbtPluginServer) Cleanup(
	ctx context.Context,
	req *pb.CleanupRequest,
) (*pb.CleanupResponse, error) {
	// Call the model's Cleanup method
	if err := s.model.Cleanup(); err != nil {
		return &pb.CleanupResponse{
			Status: &pb.Status{
				Code:    pb.StatusCode_STATUS_EXECUTION_FAILED,
				Message: fmt.Sprintf("Cleanup failed: %v", err),
			},
		}, nil
	}

	return &pb.CleanupResponse{
		Status: &pb.Status{
			Code:    pb.StatusCode_STATUS_OK,
			Message: "Cleanup successful",
		},
	}, nil
}

// ExecuteAction is the main RPC handler for executing a role action.
// Currently, unimplemented; returns a "not implemented" status.
func (s *FizzBeeMbtPluginServer) ExecuteAction(
	ctx context.Context,
	req *pb.ExecuteActionRequest,
) (*pb.ExecuteActionResponse, error) {
	// TODO: Implement action execution logic based on the role and action name.
	// Return a response with STATUS_NOT_IMPLEMENTED
	resp := &pb.ExecuteActionResponse{
		ReturnValues: nil,
		ExecTime:     &pb.Interval{}, // Empty interval
		Status: &pb.Status{
			Code:    pb.StatusCode_STATUS_NOT_IMPLEMENTED,
			Message: fmt.Sprintf("ExecuteAction is not implemented for role %s action %s", req.Role.RoleName, req.ActionName),
		},
	}

	return resp, nil
}
