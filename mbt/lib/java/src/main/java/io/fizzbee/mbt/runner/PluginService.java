package io.fizzbee.mbt.runner;

import io.fizzbee.mbt.pb.FizzBeeMbtPluginServiceGrpc;
import io.fizzbee.mbt.pb.MbtPlugin;
import io.fizzbee.mbt.pb.MbtPlugin.*;
import io.fizzbee.mbt.types.*;
import io.fizzbee.mbt.types.Arg;

import io.grpc.stub.StreamObserver;

import java.lang.reflect.Method;
import java.util.*;
import java.util.concurrent.ExecutionException;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.stream.Collectors;

public class PluginService extends FizzBeeMbtPluginServiceGrpc.FizzBeeMbtPluginServiceImplBase {
    private final Model model;
    private final Map<String, Map<String, Method>> actions;

    private static record Command(ExecuteActionRequest req, Object role, Method action, String actionName, String roleName, int roleId,
                                  Arg[] args) {}
    private static record Result(Object returnVal, Exception exception, long startTime, long endTime) {}

    public PluginService(Model m, Map<String, Map<String, Method>> actions) {
        this.model = m;
        this.actions = actions;
    }

    @Override
    public void init(InitRequest request, StreamObserver<InitResponse> responseObserver) {
        try {
            this.model.init();
            if (!(this.model instanceof RoleMapper)) {
                throw new NotImplementedException("Model does not implement RoleMapper");
            }
            InitResponse.Builder responseBuilder = InitResponse.newBuilder();

            addRolesToResponse(responseBuilder);
            responseBuilder.setStatus(Status.newBuilder()
                    .setCode(StatusCode.STATUS_OK)
                    .build());

            responseObserver.onNext(responseBuilder.build());
            responseObserver.onCompleted();
        } catch (NotImplementedException e) {
            InitResponse response = InitResponse.newBuilder()
                    .setStatus(Status.newBuilder()
                            .setCode(StatusCode.STATUS_NOT_IMPLEMENTED)
                            .setMessage(Objects.toString(e.getMessage(), ""))
                            .build())
                    .build();
            responseObserver.onNext(response);
            responseObserver.onCompleted();
        }
    }

    private void addRolesToResponse(InitResponse.Builder responseBuilder) {
        RoleMapper mapper = (RoleMapper) this.model;

        for (Map.Entry<RoleId, Role> entry : mapper.getRoles().entrySet()) {
            RoleId roleId = entry.getKey();
            responseBuilder.addRoles(RoleRef.newBuilder()
                    .setRoleName(roleId.roleName())
                    .setRoleId(roleId.index())
                    .build());
        }
    }

    private void addRolesToResponse(ExecuteActionResponse.Builder responseBuilder) {
        RoleMapper mapper = (RoleMapper) this.model;

        for (Map.Entry<RoleId, Role> entry : mapper.getRoles().entrySet()) {
            RoleId roleId = entry.getKey();
            responseBuilder.addRoles(RoleRef.newBuilder()
                    .setRoleName(roleId.roleName())
                    .setRoleId(roleId.index())
                    .build());
        }
    }

    @Override
    public void cleanup(CleanupRequest request, io.grpc.stub.StreamObserver<CleanupResponse> responseObserver) {

        try {
            this.model.cleanup();
            CleanupResponse response = CleanupResponse.newBuilder()
                    .setStatus(Status.newBuilder()
                            .setCode(StatusCode.STATUS_OK)
                            .build())
                    .build();
            responseObserver.onNext(response);
            responseObserver.onCompleted();
        } catch (NotImplementedException e) {
            CleanupResponse response = CleanupResponse.newBuilder()
                    .setStatus(Status.newBuilder()
                            .setCode(StatusCode.STATUS_NOT_IMPLEMENTED)
                            .setMessage(Objects.toString(e.getMessage(), ""))
                            .build())
                    .build();
            responseObserver.onNext(response);
            responseObserver.onCompleted();
        }
    }

    @Override
    public void executeAction(ExecuteActionRequest request, io.grpc.stub.StreamObserver<ExecuteActionResponse> responseObserver) {
        Command cmd = newCommand(request);
        Result result = executeCommand(cmd);
        ExecuteActionResponse.Builder responseBuilder = ExecuteActionResponse.newBuilder();
        if (result.exception() != null) {
            if (result.exception() instanceof NotImplementedException) {
                responseBuilder.setStatus(Status.newBuilder()
                        .setCode(StatusCode.STATUS_NOT_IMPLEMENTED)
                        .setMessage(Objects.toString(result.exception().getMessage(), "")));
            } else {
                responseBuilder.setStatus(Status.newBuilder()
                        .setCode(StatusCode.STATUS_EXECUTION_FAILED)
                        .setMessage(Objects.toString(result.exception().getMessage(), "")));
            }
        } else {
            responseBuilder.setStatus(Status.newBuilder()
                    .setCode(StatusCode.STATUS_OK))
                    .setExecTime(Interval.newBuilder()
                            .setStartUnixNano(result.startTime())
                            .setEndUnixNano(result.endTime())
                            .build());
            if (result.returnVal() != null) {
                responseBuilder.addReturnValues(fromObjectToProtoValue(result.returnVal()));
            }
            addRolesToResponse(responseBuilder);
        }
        responseObserver.onNext(responseBuilder.build());
        responseObserver.onCompleted();
    }

    @Override
    public void executeActionSequences(
            ExecuteActionSequencesRequest request,
            io.grpc.stub.StreamObserver<ExecuteActionSequencesResponse> responseObserver) {
        Thread interferenceThread = null;
        try {
            List<List<Command>> allBundles = deserializeSequences(request);

            final Thread mainThread = Thread.currentThread();
            interferenceThread = new Thread(() -> {
                try {
                    while (!Thread.currentThread().isInterrupted()) {
                        // Busy-wait with tiny sleep to increase thread contention
                        Thread.sleep(0, 1000);  // 1 microsecond; you can reduce to microseconds using LockSupport.parkNanos
                        Thread.yield();   // Hint to scheduler
                    }
                } catch (InterruptedException ignored) {
                    // Thread was interrupted -> exit
                }
            }, "InterferenceThread");

            interferenceThread.setDaemon(true);
            interferenceThread.setPriority(Thread.MAX_PRIORITY); // highest priority
            interferenceThread.start();

            // 3️⃣ Execute the sequences concurrently
            List<List<Result>> allResults = executeSequencesConcurrent(allBundles);

            // 4️⃣ Stop the interference thread
            interferenceThread.interrupt();
            interferenceThread.join();

            // 4️⃣ Serialize results into response
            ExecuteActionSequencesResponse response =
                    serializeSequenceResults(request, allBundles, allResults);

            // 5️⃣ Send back the response
            responseObserver.onNext(response);
            responseObserver.onCompleted();

        } catch (Exception e) {
            // 6️⃣ Handle exceptions cleanly
            e.printStackTrace();
            responseObserver.onError(
                    io.grpc.Status.INTERNAL
                            .withDescription("Error executing action sequences: " + e.getMessage())
                            .withCause(e)
                            .asRuntimeException()
            );
        }
    }


    public List<List<Command>> deserializeSequences(ExecuteActionSequencesRequest req) throws Exception {
        List<List<Command>> allBundles = new ArrayList<>();

        List<ActionSequence> actionSequences = req.getActionSequenceList();
        for (int seqIdx = 0; seqIdx < actionSequences.size(); seqIdx++) {
            ActionSequence seq = actionSequences.get(seqIdx);
            List<Command> cmds = new ArrayList<>();

            List<ExecuteActionRequest> requests = seq.getRequestsList();
            for (int actIdx = 0; actIdx < requests.size(); actIdx++) {
                ExecuteActionRequest actionReq = requests.get(actIdx);

                try {
                    Command cmd = newCommand(actionReq);
                    cmds.add(cmd);
                } catch (Exception e) {
                    throw new RuntimeException(String.format(
                            "Failed to deserialize: sequence index %d, action index %d: %s",
                            seqIdx, actIdx, e.getMessage()), e);
                }
            }

            allBundles.add(cmds);
        }

        return allBundles;
    }

    /**
     * Executes a sequence of commands.
     * Returns normally if all succeed, throws an exception if any command fails.
     *
     * @return
     */
    private List<Result> executeSequence(List<Command> cmds, int seqIdx) throws Exception {
        List<Result> results = new ArrayList<>();
        for (int actIdx = 0; actIdx < cmds.size(); actIdx++) {
            Command cmd = cmds.get(actIdx);

            Result result = executeCommand(cmd);
            results.add(result);

            if (result.exception() != null) {
                throw new Exception(String.format(
                        "Execution failed: sequence index %d, action index %d: %s",
                        seqIdx, actIdx, result.exception().getMessage()), result.exception());
            }

            // Sleep 1 microsecond (same as Go’s time.Sleep(1 * time.Microsecond))
            Thread.sleep(0, 1000); // (milliseconds=0, nanoseconds=1000)
        }
        return results;
    }

    /**
     * Executes all sequences concurrently, each sequence running actions sequentially.
     * Waits for all sequences to complete or returns early on the first failure.
     *
     * @return
     */
    private List<List<Result>> executeSequencesConcurrent(List<List<Command>> allBundles) throws Exception {
        // Use a fixed thread pool sized to number of sequences
        ExecutorService executor = Executors.newFixedThreadPool(allBundles.size());
        List<Future<List<Result>>> futures = new ArrayList<>();

        for (int seqIdx = 0; seqIdx < allBundles.size(); seqIdx++) {
            final int seqIdxCopy = seqIdx;
            final List<Command> cmdsCopy = allBundles.get(seqIdx);

            // Submit each sequence as a separate task
            Future<List<Result>> future = executor.submit(() -> {
                return executeSequence(cmdsCopy, seqIdxCopy);
            });

            futures.add(future);
        }

        // Wait for all sequences to finish, or return first error
        List<List<Result>> allResults = new ArrayList<>();
        Exception firstError = null;
        for (Future<List<Result>> f : futures) {
            try {
                List<Result> results = f.get(); // blocks until this sequence completes
                allResults.add(results);
            } catch (ExecutionException e) {
                // unwrap and store first exception
                if (firstError == null) {
                    Throwable cause = e.getCause();
                    if (cause instanceof Exception ex) {
                        firstError = ex;
                    } else {
                        firstError = new Exception(cause);
                    }
                }
            }
        }

        // Shutdown the executor
        executor.shutdownNow();

        if (firstError != null) {
            throw firstError;
        }
        return allResults;
    }
    /**
     * Step 3: Serialize all results back into proto responses.
     *
     * @param req         The original request containing the sequences.
     * @param allBundles  Nested list of commands for each sequence.
     * @param allResults  Nested list of corresponding results for each sequence.
     * @return A fully populated ExecuteActionSequencesResponse protobuf.
     * @throws Exception If any serialization step fails.
     */
    private ExecuteActionSequencesResponse serializeSequenceResults(
            ExecuteActionSequencesRequest req,
            List<List<Command>> allBundles,
            List<List<Result>> allResults
    ) throws Exception {
        List<ActionSequenceResult> sequenceResults = new ArrayList<>();

        // iterate over each sequence
        for (int seqIdx = 0; seqIdx < req.getActionSequenceCount(); seqIdx++) {
            ActionSequenceResult.Builder seqResultBuilder =
                    ActionSequenceResult.newBuilder();

            List<Command> cmds = allBundles.get(seqIdx);
            List<Result> results = allResults.get(seqIdx);

            for (int actIdx = 0; actIdx < cmds.size(); actIdx++) {
                Command cmd = cmds.get(actIdx);
                Result res = results.get(actIdx);

                try {
                    ExecuteActionResponse response = serializeResult(cmd, res, cmd.req());
                    seqResultBuilder.addResponses(response);
                } catch (Exception e) {
                    throw new RuntimeException(String.format(
                            "Serialization failed: sequence index %d, action index %d: %s",
                            seqIdx, actIdx, e.getMessage()), e);
                }
            }

            sequenceResults.add(seqResultBuilder.build());
        }

        return ExecuteActionSequencesResponse.newBuilder()
                .addAllResults(sequenceResults)
                .build();
    }

    /**
     * Stub: Converts one command + result into a protobuf ExecuteActionResponse.
     * To be implemented later.
     */
    private ExecuteActionResponse serializeResult(
            Command cmd,
            Result result,
            ExecuteActionRequest req
    ) {
        if (result.exception instanceof NotImplementedException) {
            return ExecuteActionResponse.newBuilder()
                    .setStatus(Status.newBuilder()
                            .setCode(StatusCode.STATUS_NOT_IMPLEMENTED)
                            .setMessage(Objects.toString(result.exception().getMessage(), ""))
                            .build())
                    .build();
        } else if (result.exception() != null) {
            return ExecuteActionResponse.newBuilder()
                    .setStatus(Status.newBuilder()
                            .setCode(StatusCode.STATUS_EXECUTION_FAILED)
                            .setMessage(Objects.toString(result.exception().getMessage(), ""))
                            .build())
                    .build();
        } else {
            ExecuteActionResponse.Builder responseBuilder = ExecuteActionResponse.newBuilder()
                    .setStatus(Status.newBuilder()
                            .setCode(StatusCode.STATUS_OK))
                    .setExecTime(Interval.newBuilder()
                            .setStartUnixNano(result.startTime())
                            .setEndUnixNano(result.endTime())
                            .build());
            if (result.returnVal() != null) {
                responseBuilder.addReturnValues(fromObjectToProtoValue(result.returnVal()));
            }
            addRolesToResponse(responseBuilder);
            return responseBuilder.build();
        }
    }

    private Command newCommand(ExecuteActionRequest req) {
        String roleName = req.getRole().getRoleName();
        int roleId = req.getRole().getRoleId();
        String actionName = req.getActionName();

        // Lookup the action method from the map
        Method action = null;
        Map<String, Method> roleActions = actions.get(roleName);
        if (roleActions != null) {
            action = roleActions.get(actionName);
        }

        if (action == null) {
            throw new IllegalArgumentException("No such action: " + actionName + " for role: " + roleName);
        }

        Object instance;
        if (roleName.isEmpty()) {
            instance = model;
        } else {
            instance = getRole(roleName, roleId);
            if (instance == null) {
                throw new IllegalStateException("Failed to get role " + roleName + " with ID " + roleId);
            }
        }

        Arg[] args = fromProtoArgsToLibArgs(req.getArgsList());

        return new Command(
                req,
                instance,
                action,
                actionName,
                roleName,
                roleId,
                args
        );
    }

    private Result executeCommand(Command command) {
        long startTime = System.nanoTime();
        Object returnVal = null;
        Exception exception = null;

        try {
            // Reflection invocation
            returnVal = command.action().invoke(command.role(), (Object) command.args());
        } catch (Exception e) {
            exception = e;
        }

        long endTime = System.nanoTime();

        return new Result(returnVal, exception, startTime, endTime);
    }

    // Dummy placeholders for context
    private Role getRole(String roleName, int roleId) {
        Map<RoleId, Role> roles = ((RoleMapper) this.model).getRoles();
        return roles.get(new RoleId(roleName, roleId));
    }

    public static Object fromProtoValueToObject(Value protoValue) {
        if (protoValue == null || protoValue.getKindCase() == Value.KindCase.KIND_NOT_SET) {
            return null;
        }

        switch (protoValue.getKindCase()) {
            case STR_VALUE:
                return protoValue.getStrValue();
            case INT_VALUE:
                return protoValue.getIntValue();
            case BOOL_VALUE:
                return protoValue.getBoolValue();
            case MAP_VALUE:
                Map<Object, Object> mapValue = new LinkedHashMap<>();
                for (MapEntry entry : protoValue.getMapValue().getEntriesList()) {
                    Object key = fromProtoValueToObject(entry.getKey());
                    Object val = fromProtoValueToObject(entry.getValue());
                    mapValue.put(key, val);
                }
                return mapValue;
            case LIST_VALUE:
                List<Object> listValue = new ArrayList<>();
                for (Value item : protoValue.getListValue().getItemsList()) {
                    listValue.add(fromProtoValueToObject(item));
                }
                return listValue;
            default:
                return null;
        }
    }

    public static Value fromObjectToProtoValue(Object value) {
        if (value == null) {
            return Value.getDefaultInstance();
        }

        if (value instanceof String str) {
            return Value.newBuilder().setStrValue(str).build();

        } else if (value instanceof Number num) {
            return Value.newBuilder().setIntValue(num.longValue()).build();

        } else if (value instanceof Boolean bool) {
            return Value.newBuilder().setBoolValue(bool).build();

        } else if (value instanceof Map<?, ?> map) {
            List<MapEntry> entries = new ArrayList<>();
            for (Map.Entry<?, ?> e : map.entrySet()) {
                Value keyVal = fromObjectToProtoValue(e.getKey());
                Value valVal = fromObjectToProtoValue(e.getValue());
                entries.add(MapEntry.newBuilder().setKey(keyVal).setValue(valVal).build());
            }

            // Optional: sort by key string for deterministic output
            entries.sort(Comparator.comparing(a -> a.getKey().toString()));

            MapValue mv = MapValue.newBuilder().addAllEntries(entries).build();
            return Value.newBuilder().setMapValue(mv).build();

        } else if (value instanceof Collection<?> coll) {
            List<Value> items = coll.stream()
                    .map(PluginService::fromObjectToProtoValue)
                    .collect(Collectors.toList());

            ListValue lv = ListValue.newBuilder().addAllItems(items).build();
            return Value.newBuilder().setListValue(lv).build();

        } else if (value.getClass().isArray()) {
            int length = java.lang.reflect.Array.getLength(value);
            List<Value> items = new ArrayList<>(length);
            for (int i = 0; i < length; i++) {
                Object element = java.lang.reflect.Array.get(value, i);
                items.add(fromObjectToProtoValue(element));
            }
            ListValue lv = ListValue.newBuilder().addAllItems(items).build();
            return Value.newBuilder().setListValue(lv).build();

        } else {
            // Unknown or unsupported type
            System.out.printf("Unknown type: %s, value=%s%n", value.getClass(), value);
            return Value.getDefaultInstance();
        }
    }
    /**
     * Converts a list of protobuf Args to an array of internal Arg objects.
     */
    public static Arg[] fromProtoArgsToLibArgs(List<MbtPlugin.Arg> protoArgs) {
        if (protoArgs == null || protoArgs.isEmpty()) {
            return new Arg[0];
        }

        Arg[] args = new Arg[protoArgs.size()];
        for (int i = 0; i < protoArgs.size(); i++) {
            args[i] = fromProtoArgToLibArg(protoArgs.get(i));
        }
        return args;
    }

    /**
     * Converts a single protobuf Arg to an internal Arg object.
     */
    public static Arg fromProtoArgToLibArg(MbtPlugin.Arg protoArg) {
        if (protoArg == null) {
            return new Arg("", null);
        }

        Object value = fromProtoValueToObject(protoArg.getValue());
        return new Arg(protoArg.getName(), value);
    }
}

