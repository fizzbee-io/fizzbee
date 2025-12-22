import * as grpc from '@grpc/grpc-js';
import type { Model, Role, StateGetter, SnapshotStateGetter, AfterActionHook, ActionFunc } from './interfaces';
import type { RoleId } from './types';
import { NotImplementedError } from './types';
import { toProtoValue, fromProtoArgs } from './value';
import * as pb from '../proto-gen/mbt_plugin_pb';
import type { IFizzBeeMbtPluginServiceServer } from '../proto-gen/mbt_plugin_grpc_pb';

/**
 * Implements the FizzBeeMbtPluginService gRPC service.
 */
export class FizzBeeMbtPluginService implements IFizzBeeMbtPluginServiceServer {
  [name: string]: any; // Index signature for gRPC service implementation

  private model: Model;
  private actions: Map<string, Map<string, ActionFunc>>;
  private baseTime: bigint;

  constructor(model: Model, actions: Map<string, Map<string, ActionFunc>>) {
    this.model = model;
    this.actions = actions;
    this.baseTime = process.hrtime.bigint();
  }

  /**
   * Initializes the model for each test run.
   */
  init: grpc.handleUnaryCall<pb.InitRequest, pb.InitResponse> = async (call, callback) => {
    try {
      // Initialize the model
      await this.model.init();

      // Get all roles
      const roles = await this.model.getRoles();
      const roleRefs: pb.RoleRef[] = [];
      const roleStates: pb.RoleState[] = [];

      const captureState = call.request.getOptions()?.getCaptureState() ?? false;

      for (const [roleIdStr, role] of roles) {
        const roleId = this.parseRoleId(roleIdStr);
        const roleRef = new pb.RoleRef();
        roleRef.setRoleName(roleId.roleName);
        roleRef.setRoleId(roleId.index);
        roleRefs.push(roleRef);

        if (captureState) {
          const state = await this.getRoleState(role);
          if (state) {
            const roleState = new pb.RoleState();
            roleState.setRole(roleRef);

            const stateMap = roleState.getStateMap();
            for (const [key, value] of Object.entries(state)) {
              stateMap.set(key, toProtoValue(value));
            }
            roleStates.push(roleState);
          }
        }
      }

      const response = new pb.InitResponse();
      const status = new pb.Status();
      status.setCode(pb.StatusCode.STATUS_OK);
      status.setMessage('Initialization successful');
      response.setStatus(status);

      const interval = new pb.Interval();
      interval.setStartUnixNano(0);
      interval.setEndUnixNano(0);
      response.setExecTime(interval);

      response.setRolesList(roleRefs);
      response.setRoleStatesList(roleStates);

      callback(null, response);
    } catch (error) {
      const response = new pb.InitResponse();
      const status = new pb.Status();
      status.setCode(pb.StatusCode.STATUS_EXECUTION_FAILED);
      status.setMessage(`Init failed: ${error instanceof Error ? error.message : String(error)}`);
      response.setStatus(status);

      const interval = new pb.Interval();
      interval.setStartUnixNano(0);
      interval.setEndUnixNano(0);
      response.setExecTime(interval);

      callback(null, response);
    }
  };

  /**
   * Cleans up the model after each test run.
   */
  cleanup: grpc.handleUnaryCall<pb.CleanupRequest, pb.CleanupResponse> = async (_call, callback) => {
    try {
      await this.model.cleanup();

      const response = new pb.CleanupResponse();
      const status = new pb.Status();
      status.setCode(pb.StatusCode.STATUS_OK);
      status.setMessage('Cleanup successful');
      response.setStatus(status);

      const interval = new pb.Interval();
      interval.setStartUnixNano(0);
      interval.setEndUnixNano(0);
      response.setExecTime(interval);

      callback(null, response);
    } catch (error) {
      const response = new pb.CleanupResponse();
      const status = new pb.Status();
      status.setCode(pb.StatusCode.STATUS_EXECUTION_FAILED);
      status.setMessage(`Cleanup failed: ${error instanceof Error ? error.message : String(error)}`);
      response.setStatus(status);

      const interval = new pb.Interval();
      interval.setStartUnixNano(0);
      interval.setEndUnixNano(0);
      response.setExecTime(interval);

      callback(null, response);
    }
  };

  /**
   * Executes a single action on a role.
   */
  executeAction: grpc.handleUnaryCall<pb.ExecuteActionRequest, pb.ExecuteActionResponse> = async (call, callback) => {
    const startTime = process.hrtime.bigint();

    try {
      const roleRef = call.request.getRole();
      const roleName = roleRef?.getRoleName() || '';
      const roleId = roleRef?.getRoleId() || 0;
      const actionName = call.request.getActionName();
      const args = fromProtoArgs(call.request.getArgsList());

      // Get the role instance
      let instance: any;
      if (roleName === '') {
        instance = this.model;
      } else {
        const roleIdStr = this.formatRoleId({ roleName, index: roleId });
        const roles = await this.model.getRoles();
        instance = roles.get(roleIdStr);
        if (!instance) {
          throw new Error(`Role ${roleName} with id ${roleId} not found`);
        }
      }

      // Get the action function
      const roleActions = this.actions.get(roleName);
      if (!roleActions) {
        throw new Error(`No actions registered for role ${roleName}`);
      }

      const action = roleActions.get(actionName);
      if (!action) {
        throw new Error(`Action ${actionName} not found for role ${roleName}`);
      }

      // Execute the action
      const returnValue = await action(instance, args);

      // Call afterAction hook if implemented (sequential mode only)
      if (this.isAfterActionHook(this.model)) {
        await this.model.afterAction();
      }

      const endTime = process.hrtime.bigint();

      // Get updated role states if requested
      const roles = await this.model.getRoles();
      const roleRefs: pb.RoleRef[] = [];
      const roleStates: pb.RoleState[] = [];
      const captureState = call.request.getOptions()?.getCaptureState() ?? false;

      for (const [roleIdStr, role] of roles) {
        const parsedId = this.parseRoleId(roleIdStr);
        const ref = new pb.RoleRef();
        ref.setRoleName(parsedId.roleName);
        ref.setRoleId(parsedId.index);
        roleRefs.push(ref);

        if (captureState) {
          const state = await this.getRoleState(role);
          if (state) {
            const roleState = new pb.RoleState();
            roleState.setRole(ref);

            const stateMap = roleState.getStateMap();
            for (const [key, value] of Object.entries(state)) {
              stateMap.set(key, toProtoValue(value));
            }
            roleStates.push(roleState);
          }
        }
      }

      const response = new pb.ExecuteActionResponse();
      if (returnValue !== undefined) {
        response.setReturnValuesList([toProtoValue(returnValue)]);
      }

      const interval = new pb.Interval();
      interval.setStartUnixNano(Number(startTime - this.baseTime));
      interval.setEndUnixNano(Number(endTime - this.baseTime));
      response.setExecTime(interval);

      const status = new pb.Status();
      status.setCode(pb.StatusCode.STATUS_OK);
      status.setMessage('OK');
      response.setStatus(status);

      response.setRolesList(roleRefs);
      response.setRoleStatesList(roleStates);

      callback(null, response);
    } catch (error) {
      const endTime = process.hrtime.bigint();

      const response = new pb.ExecuteActionResponse();
      const interval = new pb.Interval();
      const status = new pb.Status();

      if (error instanceof NotImplementedError) {
        interval.setStartUnixNano(0);
        interval.setEndUnixNano(0);
        response.setExecTime(interval);

        status.setCode(pb.StatusCode.STATUS_NOT_IMPLEMENTED);
        status.setMessage(`Action ${call.request.getActionName()} for role ${call.request.getRole()?.getRoleName()} is not implemented`);
        response.setStatus(status);
      } else {
        interval.setStartUnixNano(Number(startTime - this.baseTime));
        interval.setEndUnixNano(Number(endTime - this.baseTime));
        response.setExecTime(interval);

        status.setCode(pb.StatusCode.STATUS_EXECUTION_FAILED);
        status.setMessage(`Action ${call.request.getActionName()} for role ${call.request.getRole()?.getRoleName()} failed: ${error instanceof Error ? error.message : String(error)}`);
        response.setStatus(status);
      }

      callback(null, response);
    }
  };

  /**
   * Executes multiple action sequences concurrently.
   */
  executeActionSequences: grpc.handleUnaryCall<pb.ExecuteActionSequencesRequest, pb.ExecuteActionSequencesResponse> = async (call, callback) => {
    try {
      const sequences = call.request.getActionSequenceList();

      // Execute all sequences concurrently
      const results = await Promise.all(
        sequences.map(seq => this.executeSequence(seq))
      );

      const response = new pb.ExecuteActionSequencesResponse();
      response.setResultsList(results);

      callback(null, response);
    } catch (error) {
      callback({
        code: grpc.status.INTERNAL,
        message: `Failed to execute action sequences: ${error instanceof Error ? error.message : String(error)}`
      });
    }
  };

  /**
   * Executes a single action sequence.
   * Actions within a sequence run sequentially, but different sequences
   * run concurrently via Promise.all in executeActionSequences.
   * The setTimeout ensures the event loop can switch between sequences.
   */
  private async executeSequence(sequence: pb.ActionSequence): Promise<pb.ActionSequenceResult> {
    const responses: pb.ExecuteActionResponse[] = [];
    const requests = sequence.getRequestsList();

    for (const request of requests) {
      const response = await this.executeActionInternal(request);
      responses.push(response);

      // Yield to event loop to allow other sequences to run
      // This is crucial for concurrent sequence execution in Node.js
      // Use setImmediate (faster) or setTimeout for more reliable yielding
      await new Promise(resolve => setImmediate(resolve));
    }

    const result = new pb.ActionSequenceResult();
    result.setResponsesList(responses);
    return result;
  }

  /**
   * Internal method to execute an action and return the response.
   */
  private async executeActionInternal(request: pb.ExecuteActionRequest): Promise<pb.ExecuteActionResponse> {
    return new Promise((resolve) => {
      const call = {
        request
      } as grpc.ServerUnaryCall<pb.ExecuteActionRequest, pb.ExecuteActionResponse>;

      this.executeAction(call, (error, response) => {
        if (error || !response) {
          const fallbackResponse = new pb.ExecuteActionResponse();
          const status = new pb.Status();
          status.setCode(pb.StatusCode.STATUS_EXECUTION_FAILED);
          const errorMessage = error && typeof error === 'object' && 'message' in error
            ? (error as any).message
            : 'Unknown error';
          status.setMessage(errorMessage);
          fallbackResponse.setStatus(status);

          const interval = new pb.Interval();
          interval.setStartUnixNano(0);
          interval.setEndUnixNano(0);
          fallbackResponse.setExecTime(interval);

          resolve(fallbackResponse);
        } else {
          resolve(response);
        }
      });
    });
  }

  /**
   * Gets the state of a role, preferring SnapshotState over GetState.
   */
  private async getRoleState(role: Role): Promise<Record<string, any> | null> {
    if (this.isSnapshotStateGetter(role)) {
      return await role.snapshotState();
    } else if (this.isStateGetter(role)) {
      return await role.getState();
    }
    return null;
  }

  /**
   * Type guard for SnapshotStateGetter.
   */
  private isSnapshotStateGetter(obj: any): obj is SnapshotStateGetter {
    return typeof obj?.snapshotState === 'function';
  }

  /**
   * Type guard for StateGetter.
   */
  private isStateGetter(obj: any): obj is StateGetter {
    return typeof obj?.getState === 'function';
  }

  /**
   * Type guard for AfterActionHook.
   */
  private isAfterActionHook(obj: any): obj is AfterActionHook {
    return typeof obj?.afterAction === 'function';
  }

  /**
   * Formats a RoleId as a string for use as a map key.
   */
  private formatRoleId(roleId: RoleId): string {
    return `${roleId.roleName}#${roleId.index}`;
  }

  /**
   * Parses a RoleId string back into a RoleId object.
   */
  private parseRoleId(roleIdStr: string): RoleId {
    const parts = roleIdStr.split('#');
    return {
      roleName: parts[0] || '',
      index: parseInt(parts[1] || '0', 10)
    };
  }
}
