package mbt

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
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
	roles, err := s.model.GetRoles()

	if err != nil {
		return &pb.InitResponse{
			Status: &pb.Status{
				Code:    pb.StatusCode_STATUS_EXECUTION_FAILED,
				Message: fmt.Sprintf("Failed to get roles: %v", err),
			},
		}, nil
	}
	roleStates, refs, err := GetRoleRefsAndStates(roles, req.GetOptions().GetCaptureState())
	if err != nil {
		return &pb.InitResponse{
			Status: &pb.Status{
				Code:    pb.StatusCode_STATUS_EXECUTION_FAILED,
				Message: fmt.Sprintf("Failed to get state for roles: %v", err),
			},
		}, nil
	}

	return &pb.InitResponse{
		Status: &pb.Status{
			Code:    pb.StatusCode_STATUS_OK,
			Message: "Initialization successful",
		},
		Roles:      refs,
		RoleStates: roleStates,
	}, nil
}

func GetRoleRefsAndStates(roles map[RoleId]Role, captureState bool) ([]*pb.RoleState, []*pb.RoleRef, error) {
	refs := make([]*pb.RoleRef, 0, len(roles))
	roleStates := make([]*pb.RoleState, 0, len(roles))
	for id, role := range roles {
		refs = append(refs, &pb.RoleRef{
			RoleName: id.RoleName,
			RoleId:   int32(id.Index),
		})
		if !captureState {
			continue
		}
		roleState, err := snapshotOrGetStateAsProto(role, id)
		if err != nil {
			return roleStates, refs, err
		}
		if roleState != nil {
			roleStates = append(roleStates, roleState)
		}
	}
	return roleStates, refs, nil
}

func snapshotOrGetStateAsProto(role Role, id RoleId) (*pb.RoleState, error) {
	state, err := snapshotOrGetState(role)
	if state == nil || err != nil {
		return nil, err
	}

	return getRoleStateProto(id, state), nil
}

func snapshotOrGetState(role Role) (map[string]any, error) {
	if sg, ok := role.(SnapshotStateGetter); ok {
		return sg.SnapshotState()
	} else if sg, ok := role.(StateGetter); ok {
		return sg.GetState()
	}
	return nil, nil
}

func getRoleStateProto(id RoleId, state map[string]any) *pb.RoleState {
	roleState := &pb.RoleState{
		Role:  &pb.RoleRef{RoleName: id.RoleName, RoleId: int32(id.Index)},
		State: make(map[string]*pb.Value),
	}
	for k, v := range state {
		roleState.State[k] = fromAnyToProtoValue(v)
	}
	return roleState
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
	if err != nil {
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
	endTime := time.Now()

	returnValuesProto := []*pb.Value{}
	if returnVal != nil {
		returnValuesProto = []*pb.Value{
			fromAnyToProtoValue(returnVal),
		}
	}
	roles, err := s.model.GetRoles()

	if err != nil {
		return &pb.ExecuteActionResponse{
			Status: &pb.Status{
				Code:    pb.StatusCode_STATUS_EXECUTION_FAILED,
				Message: fmt.Sprintf("Failed to get roles: %v", err),
			},
		}, nil
	}
	roleStates, refs, err := GetRoleRefsAndStates(roles, req.GetOptions().GetCaptureState())
	if err != nil {
		return &pb.ExecuteActionResponse{
			Status: &pb.Status{
				Code:    pb.StatusCode_STATUS_EXECUTION_FAILED,
				Message: fmt.Sprintf("Failed to get state for roles: %v", err),
			},
		}, nil
	}
	res := &pb.ExecuteActionResponse{
		ReturnValues: returnValuesProto,
		ExecTime: &pb.Interval{
			StartUnixNano: startTime.UnixNano(),
			EndUnixNano:   endTime.UnixNano(),
		}, // Empty interval
		Status: &pb.Status{
			Code:    pb.StatusCode_STATUS_OK,
			Message: "OK",
		},
		Roles:      refs,
		RoleStates: roleStates,
	}
	return res, nil

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
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.String:
		return &pb.Value{Kind: &pb.Value_StrValue{StrValue: rv.String()}}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &pb.Value{Kind: &pb.Value_IntValue{IntValue: rv.Int()}}
	case reflect.Bool:
		return &pb.Value{Kind: &pb.Value_BoolValue{BoolValue: rv.Bool()}}
	case reflect.Map:
		mapEntries := make([]*pb.MapEntry, 0, rv.Len())
		for _, rKey := range rv.MapKeys() {
			mapEntries = append(mapEntries, &pb.MapEntry{
				Key:   fromAnyToProtoValue(rKey.Interface()),
				Value: fromAnyToProtoValue(rv.MapIndex(rKey).Interface()),
			})
		}
		slices.SortFunc(mapEntries, func(a, b *pb.MapEntry) int {
			return strings.Compare(a.Key.String(), b.Key.String())
		})
		return &pb.Value{Kind: &pb.Value_MapValue{MapValue: &pb.MapValue{Entries: mapEntries}}}
	case reflect.Slice, reflect.Array:
		listItems := make([]*pb.Value, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			listItems[i] = fromAnyToProtoValue(rv.Index(i).Interface())
		}
		return &pb.Value{Kind: &pb.Value_ListValue{ListValue: &pb.ListValue{Items: listItems}}}
	case reflect.Interface, reflect.Pointer:
		if rv.IsNil() {
			return nil
		}
		return fromAnyToProtoValue(rv.Elem().Interface())
	default:

		fmt.Printf("Unknown type: %T, %+v\n", value, value)
		return nil
	}
}
