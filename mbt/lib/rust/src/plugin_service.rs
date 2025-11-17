use tonic::{Request, Response, Status};
use tokio::sync::RwLock;
use std::time::Instant;
use futures::future::join_all;
use std::sync::Arc;
use crate::types::RoleId;
use crate::value::Value as RustValue;
use crate::value::sorted_map_entries;
use crate::pb::value::Kind;
use crate::pb::fizz_bee_mbt_plugin_service_server::FizzBeeMbtPluginService;
use crate::pb::{Value as ProtoValue, MapValue, MapEntry, ListValue, SetValue, Arg as ProtoArg};
use crate::error::MbtError;
use crate::types::Arg as RustArg; // Alias for clarity
use std::collections::{HashMap, HashSet};

use crate::pb::{
    InitRequest, InitResponse,
    CleanupRequest, CleanupResponse,
    ExecuteActionRequest, ExecuteActionResponse,
    ExecuteActionSequencesRequest, ExecuteActionSequencesResponse,
    Interval, RoleRef, Status as ProtoStatus, StatusCode,
    ActionSequence, ActionSequenceResult, ExecOptions,
};
use crate::traits::{Model, DispatchModel};

fn mbt_error_to_status(err: MbtError) -> Status {
    Status::internal(format!("MBT Execution Error: {}", err))
}

fn proto_ref_to_role_id(proto_ref: RoleRef) -> Result<RoleId, MbtError> {
    Ok(RoleId {
        role_name: proto_ref.role_name,
        // The proto expects i32, so we safely cast the u32 index.
        index: proto_ref.role_id as i32
    })
}
fn role_id_to_proto_ref(role_id: crate::types::RoleId) -> RoleRef {
    RoleRef {
        role_name: role_id.role_name,
        // The proto expects i32, so we safely cast the u32 index.
        role_id: role_id.index as i32,
    }
}
/// Converts an internal Rust Value enum into a Protobuf Value message.
fn rust_value_to_proto_value(rust_value: RustValue) -> ProtoValue {
    match rust_value {
        RustValue::Int(v) => ProtoValue {
            kind: Some(Kind::IntValue(v)),
        },
        RustValue::Str(s) => ProtoValue {
            kind: Some(Kind::StrValue(s)),
        },
        RustValue::Bool(b) => ProtoValue {
            kind: Some(Kind::BoolValue(b)),
        },
        RustValue::Map(map) => {
            let entries = sorted_map_entries(&map)
                .into_iter()
                .map(|(k, v)| MapEntry {
                    // Recursive call to convert key and value
                    key: Some(rust_value_to_proto_value(k.clone())),
                    value: Some(rust_value_to_proto_value(v.clone())),
                })
                .collect();

            let map_value = MapValue { entries };

            ProtoValue {
                kind: Some(Kind::MapValue(map_value)),
            }
        }

        RustValue::List(list) => {
            let items = list
                .into_iter()
                .map(rust_value_to_proto_value) // Recursive call
                .collect();

            let list_value = ListValue { items };

            ProtoValue {
                kind: Some(Kind::ListValue(list_value)),
            }
        }

        RustValue::Set(set) => {
            // Suppressing unused variable warning.
            // TODO: Remove the SetValue in the proto file
            _ = SetValue::default();

            // We serialize it as a List, as the ordering doesn't matter for sets.
            let items = set
                .into_iter()
                .map(rust_value_to_proto_value)
                .collect();

            let list_value = ListValue { items };

            ProtoValue {
                kind: Some(Kind::ListValue(list_value)),
            }
        }

        RustValue::None => {
            ProtoValue::default()
        }
    }
}
/// Converts a Protobuf Value message into an internal Rust Value enum.
fn proto_value_to_rust_value(proto_value: ProtoValue) -> Result<RustValue, MbtError> {
    // If kind is None, it corresponds to RustValue::None
    let kind = match proto_value.kind {
        Some(k) => k,
        None => return Ok(RustValue::None),
    };

    match kind {
        Kind::StrValue(s) => Ok(RustValue::Str(s)),
        Kind::IntValue(v) => Ok(RustValue::Int(v)),
        Kind::BoolValue(b) => Ok(RustValue::Bool(b)),
        Kind::MapValue(MapValue { entries }) => {
            let mut map = HashMap::new();
            for MapEntry { key, value } in entries {
                let key = key.ok_or_else(|| MbtError::Other("MapEntry key is missing.".into()))?;
                let value = value.ok_or_else(|| MbtError::Other("MapEntry value is missing.".into()))?;

                // Recursive call to convert key and value
                let rust_key = proto_value_to_rust_value(key)?;
                let rust_value = proto_value_to_rust_value(value)?;
                map.insert(rust_key, rust_value);
            }
            Ok(RustValue::Map(map))
        }
        Kind::ListValue(ListValue { items }) => {
            let list: Result<Vec<RustValue>, MbtError> = items
                .into_iter()
                .map(proto_value_to_rust_value) // Recursive call
                .collect();

            Ok(RustValue::List(list?))
        }
        _ => {
            Err(MbtError::NotImplemented("Unsupported Protobuf Value kind for conversion.".into()))
        }
    }
}

/// Converts a vector of Protobuf Arg messages into a vector of internal Rust Arg structs.
pub fn proto_args_to_rust_args(proto_args: Vec<ProtoArg>) -> Result<Vec<RustArg>, MbtError> {
    let mut rust_args = Vec::with_capacity(proto_args.len());

    for proto_arg in proto_args {
        // The value field is mandatory based on the Rust struct `Arg`
        let proto_value = proto_arg.value.ok_or_else(|| {
            MbtError::Other(format!("Value is missing for argument '{}'.", proto_arg.name))
        })?;

        let rust_value = proto_value_to_rust_value(proto_value)?;

        let rust_arg = RustArg {
            name: proto_arg.name,
            value: rust_value,
        };
        rust_args.push(rust_arg);
    }

    Ok(rust_args)
}

fn proto_status_ok() -> ProtoStatus {
    ProtoStatus {
        code: StatusCode::StatusOk as i32,
        message: "OK".to_string(),
    }
}

fn mbt_error_to_proto_status(err: MbtError) -> ProtoStatus {
    if err.is_not_implemented() {
        return ProtoStatus {
            code: StatusCode::StatusNotImplemented as i32,
            message: format!("Not Implemented: {}", err),
        };
    }
    ProtoStatus {
        code: StatusCode::StatusExecutionFailed as i32,
        message: format!("Execution Failed: {}", err),
    }
}

/// Internal structure representing a single action command with space for execution results.
#[derive(Debug, Clone)]
struct ExecuteActionCommand {
    pub request: ExecuteActionRequest,
    pub _exec_options: ExecOptions,
    pub args : Vec<RustArg>,
    // Results
    pub start_time: Option<Instant>,
    pub end_time: Option<Instant>,
    pub return_value: Option<RustValue>,
    pub error: Option<MbtError>,
}

/// Internal structure representing a sequence of actions.
type ActionSequenceCommandBundle = Vec<ExecuteActionCommand>;

pub struct FizzBeeServiceImpl<D>
where
    D: Model + DispatchModel + Send + Sync + 'static,
{
    // The model dispatcher is now protected by a Read-Write Lock
    dispatcher: Arc<RwLock<D>>,
    base_instant: Instant,
}

impl<D> FizzBeeServiceImpl<D>
where
    D: Model + DispatchModel + Send + Sync + 'static,
{
    pub fn new(dispatcher: D) -> Self {
        FizzBeeServiceImpl {
            dispatcher: Arc::new(RwLock::new(dispatcher)),
            base_instant: Instant::now(),
        }
    }
    /// Calls get_roles on the dispatcher and converts Rust RoleId structs to Protobuf RoleRef messages.
    /// Requires a write lock guard because the DispatchModel trait defines get_roles(&mut self).
    fn get_and_convert_roles(
        dispatcher: &tokio::sync::RwLockWriteGuard<'_, D>
    ) -> Result<Vec<RoleRef>, MbtError> {

        let rust_role_ids = dispatcher.get_roles()?;

        let proto_role_refs: Vec<RoleRef> = rust_role_ids.into_iter()
            .map(role_id_to_proto_ref)
            .collect();

        Ok(proto_role_refs)
    }

    /// Calculates monotonic time in nanoseconds since the service was created.
    fn nanos_since_base(&self, instant: Instant) -> i64 {
        instant.duration_since(self.base_instant).as_nanos() as i64
    }

    /// Deserializes the proto request into internal command bundles.
    fn deserialize_sequences(
        req: Request<ExecuteActionSequencesRequest>
    ) -> Result<Vec<ActionSequenceCommandBundle>, MbtError> {
        let proto_sequences = req.into_inner().action_sequence;
        let mut all_bundles = Vec::with_capacity(proto_sequences.len());

        for ActionSequence { requests, options } in proto_sequences {
            let options = options.unwrap_or_default();
            let mut bundle = Vec::with_capacity(requests.len());
            for mut request in requests {
                let proto_args = std::mem::take(&mut request.args);
                let rust_args = proto_args_to_rust_args(proto_args)?;
                bundle.push(ExecuteActionCommand {
                    request,
                    _exec_options: options.clone(),
                    args: rust_args.clone(),
                    start_time: None,
                    end_time: None,
                    return_value: None,
                    error: None,
                });
            }
            all_bundles.push(bundle);
        }
        Ok(all_bundles)
    }

    /// Executes all command bundles concurrently using tokio::spawn and waits for results.
    /// Each sequence is executed in a dedicated task, and all actions acquire a READ lock,
    /// allowing multiple actions to execute simultaneously.
    async fn execute_sequences_concurrent(
        &self,
        all_bundles: Vec<ActionSequenceCommandBundle>, // Take ownership (by value)
    ) -> Result<Vec<ActionSequenceCommandBundle>, MbtError> { // Return ownership on success

        let mut futures = Vec::with_capacity(all_bundles.len());

        // Iterate over the bundles, moving ownership and the original index into the future
        for (seq_idx, mut sequence) in all_bundles.into_iter().enumerate() {

            let dispatcher_arc = self.dispatcher.clone(); // Clone dispatcher Arc for the task

            let future = tokio::spawn(async move {

                // Run each action sequentially within this task's owned sequence
                for cmd in sequence.iter_mut() {

                    let action_name = cmd.request.action_name.clone();
                    // Convert Protobuf RoleRef to Rust RoleId struct.
                    let role_ref = cmd.request.role.clone().unwrap_or_default();
                    let role_id = proto_ref_to_role_id(role_ref)
                        .unwrap_or_else(|_| RoleId::default());

                    // Acquire the shared READ lock on the model dispatcher for concurrent execution.
                    let dispatcher_read_lock = dispatcher_arc.read().await;

                    let start_time = Instant::now();
                    // Use `execute` method from the DispatchModel trait.
                    // This relies on the implementer D to use internal locking (e.g., std::sync::Mutex)
                    // on specific parts of its state to manage concurrent writes.
                    let (result, err) = match dispatcher_read_lock.execute(
                        &role_id, // &RoleId struct
                        &action_name,
                        &cmd.args // &Vec<RustArg>
                    ).await {
                        Ok(val) => (Some(val), None),
                        Err(e) => (None, Some(e)),
                    };

                    let end_time = Instant::now();
                    drop(dispatcher_read_lock); // Release READ lock immediately after execution


                    // Record results locally in the owned sequence bundle
                    cmd.start_time = Some(start_time);
                    cmd.end_time = Some(end_time);
                    cmd.return_value = result;
                    cmd.error = err;

                    // Check for critical errors (excluding NotImplemented) and early exit.
                    if let Some(ref e) = cmd.error {
                        if !e.is_not_implemented() {
                            // Critical failure: return the MbtError
                            return Err(e.clone());
                        }
                    }
                }

                // On success, return the index and the completed sequence bundle
                Ok((seq_idx, sequence))
            });

            futures.push(future);
        }

        // --- Use join_all for cleaner concurrent waiting ---
        let results = join_all(futures).await;

        // Initialize the final result vector using iterator to avoid the Clone requirement.
        let mut final_bundles: Vec<Option<ActionSequenceCommandBundle>> =
            std::iter::repeat(None).take(results.len()).collect();

        for res in results {
            match res {
                // Task completed successfully (Outer Result Ok)
                Ok(inner_res) => match inner_res {
                    // Execution succeeded: place bundle at its original index
                    Ok((idx, sequence)) => {
                        final_bundles[idx] = Some(sequence);
                    },
                    // Execution failed with MbtError (Inner Result Err): critical error, return it immediately.
                    Err(e) => return Err(e),
                },
                // Task panicked (Outer Result Err): return a generalized MbtError
                Err(e) => return Err(MbtError::other(format!("Action sequence task failed: {}", e))),
            }
        }

        // Unwrap the vector of Options into the final Vec<Bundle>, panic if one is missing (logic error)
        let bundles = final_bundles.into_iter().map(|o| o.expect("Sequence missing after join_all. Logic error in indexing/processing.")).collect();

        Ok(bundles) // Return the final vector
    }

    /// Serializes the executed command bundles back into a proto response.
    fn serialize_sequence_results(
        &self,
        all_bundles: Vec<ActionSequenceCommandBundle>,
    ) -> Result<Response<ExecuteActionSequencesResponse>, Status> {
        let mut results = Vec::with_capacity(all_bundles.len());

        for bundle in all_bundles {
            let mut action_responses = Vec::with_capacity(bundle.len());

            for cmd in bundle {
                // Convert ExecutionCommand to ExecuteActionResponse

                // 1. Time Interval
                let exec_time = if let (Some(start), Some(end)) = (cmd.start_time, cmd.end_time) {
                    Some(Interval {
                        start_unix_nano: self.nanos_since_base(start),
                        end_unix_nano: self.nanos_since_base(end),
                    })
                } else {
                    None
                };

                // 2. Return Values and Status
                let (return_values, status) = match cmd.return_value {
                    Some(RustValue::None) => (vec![], proto_status_ok()),
                    Some(value) => {
                        // Use the existing function `rust_value_to_proto_value`
                        let proto_value = rust_value_to_proto_value(value);
                        (vec![proto_value], proto_status_ok())
                    }
                    None => {
                        // Use mbt_error_to_proto_status
                        let error = cmd.error.unwrap_or_else(|| {
                            MbtError::other("Unknown error in sequence execution")
                        });
                        (vec![], mbt_error_to_proto_status(error))
                    }
                };

                // 3. Roles and State (Defaults to empty as they are not tracked during execution)
                let roles = vec![];
                let role_states = vec![];

                action_responses.push(ExecuteActionResponse {
                    return_values,
                    exec_time,
                    status: Some(status),
                    roles,
                    role_states,
                });
            }

            results.push(ActionSequenceResult {
                responses: action_responses,
            });
        }

        let response = ExecuteActionSequencesResponse { results };
        Ok(Response::new(response))
    }
}

#[tonic::async_trait]
impl<D> FizzBeeMbtPluginService for FizzBeeServiceImpl<D>
where
    D: Model + DispatchModel + Send + Sync + 'static,
{
    async fn init(
        &self,
        _request: Request<InitRequest>,
    ) -> Result<Response<InitResponse>, Status> {

        // Acquire a WRITE lock for initialization (exclusive access)
        let mut dispatcher = self.dispatcher.write().await;

        match dispatcher.init().await {
            Ok(_) => {
                // get_roles requires &mut self, so it must also hold the WRITE lock.
                let proto_role_refs = Self::get_and_convert_roles(&dispatcher)
                    .map_err(mbt_error_to_status)?;
                let response = InitResponse {
                    status: Some(proto_status_ok()),
                    roles: proto_role_refs,
                    ..Default::default()
                };
                Ok(Response::new(response))
            }
            Err(e) => Err(mbt_error_to_status(e)),
        }
    }

    async fn cleanup(
        &self,
        _request: Request<CleanupRequest>,
    ) -> Result<Response<CleanupResponse>, Status> {

        // Acquire a WRITE lock for cleanup (exclusive access)
        let mut model = self.dispatcher.write().await;

        match model.cleanup().await {
            Ok(_) => {
                let response = CleanupResponse {
                    status: Some(proto_status_ok()),
                    ..Default::default()
                };
                Ok(Response::new(response))
            }
            Err(e) => Err(mbt_error_to_status(e)),
        }
    }

    async fn execute_action(
        &self,
        request: Request<ExecuteActionRequest>,
    ) -> Result<Response<ExecuteActionResponse>, Status> {
        let req = request.into_inner();

        // 1. Acquire READ lock for action execution
        let dispatcher_read_lock = self.dispatcher.read().await;

        // 2. Convert RoleRef
        let role_id = proto_ref_to_role_id(req.role.ok_or_else(|| Status::invalid_argument("RoleRef is missing in request."))?)
                     .map_err(mbt_error_to_status)?;
        let rust_args = proto_args_to_rust_args(req.args)
                .map_err(mbt_error_to_status)?;

        // 3. Delegate execution using the READ lock (relies on D's internal sync)
        let result = dispatcher_read_lock.execute(
            &role_id,
            &req.action_name,
            &rust_args
        ).await;

        // Drop the read lock so we can acquire a write lock for get_roles
        drop(dispatcher_read_lock);

        // 4. Acquire a WRITE lock just to call get_roles (due to trait definition requiring &mut self)
        let dispatcher_write_lock = self.dispatcher.write().await;

        // 5. Convert the result back to the Protobuf response 🔄
        match result {
            Ok(returned_value) => {
                let proto_role_refs = Self::get_and_convert_roles(&dispatcher_write_lock)
                    .map_err(mbt_error_to_status)?;

                let return_values = match returned_value {
                    crate::value::Value::None => {
                        vec![]
                    },
                    value => {
                        let proto_value = rust_value_to_proto_value(value);
                        vec![proto_value]
                    }
                };
                let response = ExecuteActionResponse {
                    return_values: return_values,
                    status: Some(proto_status_ok()),
                    roles: proto_role_refs,
                    ..Default::default()
                };
                Ok(Response::new(response))
            }
            Err(e) => {
                let response = ExecuteActionResponse {
                    status: Some(mbt_error_to_proto_status(e)),
                    ..Default::default()
                };
                Ok(Response::new(response))
            }
        }
    }

    async fn execute_action_sequences(
        &self,
        request: Request<ExecuteActionSequencesRequest>,
    ) -> Result<Response<ExecuteActionSequencesResponse>, Status> {
        // Step 1: Deserialize upfront
        let all_bundles = match Self::deserialize_sequences(request) {
            Ok(bundles) => bundles,
            Err(e) => return Err(mbt_error_to_status(e)),
        };

        // Step 2: Execute sequences concurrently, relying on the RwLock read access
        let all_bundles = self.execute_sequences_concurrent(all_bundles)
            .await
            .map_err(mbt_error_to_status)?;

        // Step 3: Serialize results
        self.serialize_sequence_results(all_bundles)
    }
}
