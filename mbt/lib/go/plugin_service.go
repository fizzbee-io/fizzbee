package mbt

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

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

	refs := make([]*pb.RoleRef, 0)
	//convert s.model.GetRoles() to RoleRefs
	roles, err := s.model.GetRoles()
	if err != nil {
		return &pb.InitResponse{
			Status: &pb.Status{
				Code:    pb.StatusCode_STATUS_EXECUTION_FAILED,
				Message: fmt.Sprintf("Failed to get roles: %v", err),
			},
		}, nil
	}
	for id, _ := range roles {
		refs = append(refs, &pb.RoleRef{
			RoleName: id.RoleName,
			RoleId:   int32(id.Index),
		})
	}
	return &pb.InitResponse{
		Status: &pb.Status{
			Code:    pb.StatusCode_STATUS_OK,
			Message: "Initialization successful",
		},
		Roles: refs,
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
	roleName := req.GetRole().GetRoleName()
	roleId := req.GetRole().GetRoleId()
	actionName := req.GetActionName()
	action := s.actions[roleName][actionName]
	var instance Role
	var err error
	if roleName == "" {
		instance = s.model
	} else {
		instance, err = s.getRole(roleName, roleId)
		if err != nil {
			return &pb.ExecuteActionResponse{
				Status: &pb.Status{
					Code:    pb.StatusCode_STATUS_EXECUTION_FAILED,
					Message: fmt.Sprintf("Failed to get role %s: %v", roleName, err),
				},
			}, nil
		}
	}
	startTime := time.Now()
	returnVal, err := action(instance, fromProtoArgsToLibArgs(req.GetArgs()))
	if err == nil {
		endTime := time.Now()
		return &pb.ExecuteActionResponse{
			ReturnValues: []*pb.Value{
				fromAnyToProtoValue(returnVal),
			},
			ExecTime: &pb.Interval{
				StartUnixNano: startTime.UnixNano(),
				EndUnixNano:   endTime.UnixNano(),
			}, // Empty interval
			Status: &pb.Status{
				Code:    pb.StatusCode_STATUS_OK,
				Message: "OK",
			},
		}, nil
	}
	if errors.Is(err, ErrNotImplemented) {
		// If the action is not implemented, return a specific status code
		return &pb.ExecuteActionResponse{
			ReturnValues: nil,
			ExecTime:     &pb.Interval{}, // Empty interval
			Status: &pb.Status{
				Code:    pb.StatusCode_STATUS_NOT_IMPLEMENTED,
				Message: fmt.Sprintf("Action %s for role %s is not implemented", actionName, roleName),
			},
		}, nil
	} else {
		// If there is any other error, return it as a failed status
		return &pb.ExecuteActionResponse{
			ReturnValues: nil,
			ExecTime:     &pb.Interval{}, // Empty interval
			Status: &pb.Status{
				Code:    pb.StatusCode_STATUS_EXECUTION_FAILED,
				Message: fmt.Sprintf("Action %s for role %s failed: %v", actionName, roleName, err),
			},
		}, nil
	}

}

func (s *FizzBeeMbtPluginServer) getRole(roleName string, roleId int32) (Role, error) {
	roles, err := s.model.GetRoles()
	if err != nil {
		return nil, err
	}
	id := RoleId{
		RoleName: roleName,
		Index:    int(roleId),
	}
	if role, ok := roles[id]; ok {
		return role, nil
	} else {
		return nil, fmt.Errorf("role %s with id %d not found", roleName, roleId)
	}
}

func fromProtoArgsToLibArgs(protoArgs []*pb.Arg) []Arg {
	if protoArgs == nil {
		return nil
	}
	args := make([]Arg, len(protoArgs))
	for i, protoArg := range protoArgs {
		args[i] = fromProtoArgToLibArg(protoArg)
	}
	return args
}
func fromProtoArgToLibArg(protoArg *pb.Arg) Arg {
	if protoArg == nil {
		return Arg{}
	}
	value := fromProtoValueToAny(protoArg.Value)

	return Arg{
		Name:  protoArg.Name,
		Value: value,
	}
}

func fromProtoValueToAny(protoValue *pb.Value) any {
	if protoValue == nil {
		return nil
	}

	switch v := protoValue.Kind.(type) {
	case *pb.Value_StrValue:
		return v.StrValue
	case *pb.Value_IntValue:
		return int(v.IntValue)
	case *pb.Value_BoolValue:
		return v.BoolValue
	case *pb.Value_MapValue:
		mapValue := make(map[any]any)
		for _, entry := range v.MapValue.Entries {
			key := fromProtoValueToAny(entry.Key)
			val := fromProtoValueToAny(entry.Value)
			mapValue[key] = val
		}
		return mapValue
	case *pb.Value_ListValue:
		listValue := make([]any, len(v.ListValue.Items))
		for i, item := range v.ListValue.Items {
			listValue[i] = fromProtoValueToAny(item)
		}
		return listValue
	default:
		return nil
	}
}

func fromAnyToProtoValue(value any) *pb.Value {
	switch v := value.(type) {
	case string:
		return &pb.Value{Kind: &pb.Value_StrValue{StrValue: v}}
	case int:
		return &pb.Value{Kind: &pb.Value_IntValue{IntValue: int64(v)}}
	case bool:
		return &pb.Value{Kind: &pb.Value_BoolValue{BoolValue: v}}
	case map[any]any:
		mapEntries := make([]*pb.MapEntry, 0, len(v))
		for key, val := range v {
			mapEntries = append(mapEntries, &pb.MapEntry{
				Key:   fromAnyToProtoValue(key),
				Value: fromAnyToProtoValue(val),
			})
		}
		return &pb.Value{Kind: &pb.Value_MapValue{MapValue: &pb.MapValue{Entries: mapEntries}}}
	case []any:
		listItems := make([]*pb.Value, len(v))
		for i, item := range v {
			listItems[i] = fromAnyToProtoValue(item)
		}
		return &pb.Value{Kind: &pb.Value_ListValue{ListValue: &pb.ListValue{Items: listItems}}}
	default:
		return nil
	}
}
