package io.fizzbee.mbt.pb;

import static io.grpc.MethodDescriptor.generateFullMethodName;

/**
 * <pre>
 * ------------------ Service Definition ------------------
 * </pre>
 */
@io.grpc.stub.annotations.GrpcGenerated
public final class FizzBeeMbtPluginServiceGrpc {

  private FizzBeeMbtPluginServiceGrpc() {}

  public static final java.lang.String SERVICE_NAME = "fizzbee.mbt.FizzBeeMbtPluginService";

  // Static method descriptors that strictly reflect the proto.
  private static volatile io.grpc.MethodDescriptor<io.fizzbee.mbt.pb.MbtPlugin.InitRequest,
      io.fizzbee.mbt.pb.MbtPlugin.InitResponse> getInitMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "Init",
      requestType = io.fizzbee.mbt.pb.MbtPlugin.InitRequest.class,
      responseType = io.fizzbee.mbt.pb.MbtPlugin.InitResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.UNARY)
  public static io.grpc.MethodDescriptor<io.fizzbee.mbt.pb.MbtPlugin.InitRequest,
      io.fizzbee.mbt.pb.MbtPlugin.InitResponse> getInitMethod() {
    io.grpc.MethodDescriptor<io.fizzbee.mbt.pb.MbtPlugin.InitRequest, io.fizzbee.mbt.pb.MbtPlugin.InitResponse> getInitMethod;
    if ((getInitMethod = FizzBeeMbtPluginServiceGrpc.getInitMethod) == null) {
      synchronized (FizzBeeMbtPluginServiceGrpc.class) {
        if ((getInitMethod = FizzBeeMbtPluginServiceGrpc.getInitMethod) == null) {
          FizzBeeMbtPluginServiceGrpc.getInitMethod = getInitMethod =
              io.grpc.MethodDescriptor.<io.fizzbee.mbt.pb.MbtPlugin.InitRequest, io.fizzbee.mbt.pb.MbtPlugin.InitResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.UNARY)
              .setFullMethodName(generateFullMethodName(SERVICE_NAME, "Init"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  io.fizzbee.mbt.pb.MbtPlugin.InitRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  io.fizzbee.mbt.pb.MbtPlugin.InitResponse.getDefaultInstance()))
              .setSchemaDescriptor(new FizzBeeMbtPluginServiceMethodDescriptorSupplier("Init"))
              .build();
        }
      }
    }
    return getInitMethod;
  }

  private static volatile io.grpc.MethodDescriptor<io.fizzbee.mbt.pb.MbtPlugin.CleanupRequest,
      io.fizzbee.mbt.pb.MbtPlugin.CleanupResponse> getCleanupMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "Cleanup",
      requestType = io.fizzbee.mbt.pb.MbtPlugin.CleanupRequest.class,
      responseType = io.fizzbee.mbt.pb.MbtPlugin.CleanupResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.UNARY)
  public static io.grpc.MethodDescriptor<io.fizzbee.mbt.pb.MbtPlugin.CleanupRequest,
      io.fizzbee.mbt.pb.MbtPlugin.CleanupResponse> getCleanupMethod() {
    io.grpc.MethodDescriptor<io.fizzbee.mbt.pb.MbtPlugin.CleanupRequest, io.fizzbee.mbt.pb.MbtPlugin.CleanupResponse> getCleanupMethod;
    if ((getCleanupMethod = FizzBeeMbtPluginServiceGrpc.getCleanupMethod) == null) {
      synchronized (FizzBeeMbtPluginServiceGrpc.class) {
        if ((getCleanupMethod = FizzBeeMbtPluginServiceGrpc.getCleanupMethod) == null) {
          FizzBeeMbtPluginServiceGrpc.getCleanupMethod = getCleanupMethod =
              io.grpc.MethodDescriptor.<io.fizzbee.mbt.pb.MbtPlugin.CleanupRequest, io.fizzbee.mbt.pb.MbtPlugin.CleanupResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.UNARY)
              .setFullMethodName(generateFullMethodName(SERVICE_NAME, "Cleanup"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  io.fizzbee.mbt.pb.MbtPlugin.CleanupRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  io.fizzbee.mbt.pb.MbtPlugin.CleanupResponse.getDefaultInstance()))
              .setSchemaDescriptor(new FizzBeeMbtPluginServiceMethodDescriptorSupplier("Cleanup"))
              .build();
        }
      }
    }
    return getCleanupMethod;
  }

  private static volatile io.grpc.MethodDescriptor<io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionRequest,
      io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionResponse> getExecuteActionMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "ExecuteAction",
      requestType = io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionRequest.class,
      responseType = io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.UNARY)
  public static io.grpc.MethodDescriptor<io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionRequest,
      io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionResponse> getExecuteActionMethod() {
    io.grpc.MethodDescriptor<io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionRequest, io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionResponse> getExecuteActionMethod;
    if ((getExecuteActionMethod = FizzBeeMbtPluginServiceGrpc.getExecuteActionMethod) == null) {
      synchronized (FizzBeeMbtPluginServiceGrpc.class) {
        if ((getExecuteActionMethod = FizzBeeMbtPluginServiceGrpc.getExecuteActionMethod) == null) {
          FizzBeeMbtPluginServiceGrpc.getExecuteActionMethod = getExecuteActionMethod =
              io.grpc.MethodDescriptor.<io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionRequest, io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.UNARY)
              .setFullMethodName(generateFullMethodName(SERVICE_NAME, "ExecuteAction"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionResponse.getDefaultInstance()))
              .setSchemaDescriptor(new FizzBeeMbtPluginServiceMethodDescriptorSupplier("ExecuteAction"))
              .build();
        }
      }
    }
    return getExecuteActionMethod;
  }

  private static volatile io.grpc.MethodDescriptor<io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionSequencesRequest,
      io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionSequencesResponse> getExecuteActionSequencesMethod;

  @io.grpc.stub.annotations.RpcMethod(
      fullMethodName = SERVICE_NAME + '/' + "ExecuteActionSequences",
      requestType = io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionSequencesRequest.class,
      responseType = io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionSequencesResponse.class,
      methodType = io.grpc.MethodDescriptor.MethodType.UNARY)
  public static io.grpc.MethodDescriptor<io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionSequencesRequest,
      io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionSequencesResponse> getExecuteActionSequencesMethod() {
    io.grpc.MethodDescriptor<io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionSequencesRequest, io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionSequencesResponse> getExecuteActionSequencesMethod;
    if ((getExecuteActionSequencesMethod = FizzBeeMbtPluginServiceGrpc.getExecuteActionSequencesMethod) == null) {
      synchronized (FizzBeeMbtPluginServiceGrpc.class) {
        if ((getExecuteActionSequencesMethod = FizzBeeMbtPluginServiceGrpc.getExecuteActionSequencesMethod) == null) {
          FizzBeeMbtPluginServiceGrpc.getExecuteActionSequencesMethod = getExecuteActionSequencesMethod =
              io.grpc.MethodDescriptor.<io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionSequencesRequest, io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionSequencesResponse>newBuilder()
              .setType(io.grpc.MethodDescriptor.MethodType.UNARY)
              .setFullMethodName(generateFullMethodName(SERVICE_NAME, "ExecuteActionSequences"))
              .setSampledToLocalTracing(true)
              .setRequestMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionSequencesRequest.getDefaultInstance()))
              .setResponseMarshaller(io.grpc.protobuf.ProtoUtils.marshaller(
                  io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionSequencesResponse.getDefaultInstance()))
              .setSchemaDescriptor(new FizzBeeMbtPluginServiceMethodDescriptorSupplier("ExecuteActionSequences"))
              .build();
        }
      }
    }
    return getExecuteActionSequencesMethod;
  }

  /**
   * Creates a new async stub that supports all call types for the service
   */
  public static FizzBeeMbtPluginServiceStub newStub(io.grpc.Channel channel) {
    io.grpc.stub.AbstractStub.StubFactory<FizzBeeMbtPluginServiceStub> factory =
      new io.grpc.stub.AbstractStub.StubFactory<FizzBeeMbtPluginServiceStub>() {
        @java.lang.Override
        public FizzBeeMbtPluginServiceStub newStub(io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
          return new FizzBeeMbtPluginServiceStub(channel, callOptions);
        }
      };
    return FizzBeeMbtPluginServiceStub.newStub(factory, channel);
  }

  /**
   * Creates a new blocking-style stub that supports all types of calls on the service
   */
  public static FizzBeeMbtPluginServiceBlockingV2Stub newBlockingV2Stub(
      io.grpc.Channel channel) {
    io.grpc.stub.AbstractStub.StubFactory<FizzBeeMbtPluginServiceBlockingV2Stub> factory =
      new io.grpc.stub.AbstractStub.StubFactory<FizzBeeMbtPluginServiceBlockingV2Stub>() {
        @java.lang.Override
        public FizzBeeMbtPluginServiceBlockingV2Stub newStub(io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
          return new FizzBeeMbtPluginServiceBlockingV2Stub(channel, callOptions);
        }
      };
    return FizzBeeMbtPluginServiceBlockingV2Stub.newStub(factory, channel);
  }

  /**
   * Creates a new blocking-style stub that supports unary and streaming output calls on the service
   */
  public static FizzBeeMbtPluginServiceBlockingStub newBlockingStub(
      io.grpc.Channel channel) {
    io.grpc.stub.AbstractStub.StubFactory<FizzBeeMbtPluginServiceBlockingStub> factory =
      new io.grpc.stub.AbstractStub.StubFactory<FizzBeeMbtPluginServiceBlockingStub>() {
        @java.lang.Override
        public FizzBeeMbtPluginServiceBlockingStub newStub(io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
          return new FizzBeeMbtPluginServiceBlockingStub(channel, callOptions);
        }
      };
    return FizzBeeMbtPluginServiceBlockingStub.newStub(factory, channel);
  }

  /**
   * Creates a new ListenableFuture-style stub that supports unary calls on the service
   */
  public static FizzBeeMbtPluginServiceFutureStub newFutureStub(
      io.grpc.Channel channel) {
    io.grpc.stub.AbstractStub.StubFactory<FizzBeeMbtPluginServiceFutureStub> factory =
      new io.grpc.stub.AbstractStub.StubFactory<FizzBeeMbtPluginServiceFutureStub>() {
        @java.lang.Override
        public FizzBeeMbtPluginServiceFutureStub newStub(io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
          return new FizzBeeMbtPluginServiceFutureStub(channel, callOptions);
        }
      };
    return FizzBeeMbtPluginServiceFutureStub.newStub(factory, channel);
  }

  /**
   * <pre>
   * ------------------ Service Definition ------------------
   * </pre>
   */
  public interface AsyncService {

    /**
     */
    default void init(io.fizzbee.mbt.pb.MbtPlugin.InitRequest request,
        io.grpc.stub.StreamObserver<io.fizzbee.mbt.pb.MbtPlugin.InitResponse> responseObserver) {
      io.grpc.stub.ServerCalls.asyncUnimplementedUnaryCall(getInitMethod(), responseObserver);
    }

    /**
     */
    default void cleanup(io.fizzbee.mbt.pb.MbtPlugin.CleanupRequest request,
        io.grpc.stub.StreamObserver<io.fizzbee.mbt.pb.MbtPlugin.CleanupResponse> responseObserver) {
      io.grpc.stub.ServerCalls.asyncUnimplementedUnaryCall(getCleanupMethod(), responseObserver);
    }

    /**
     */
    default void executeAction(io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionRequest request,
        io.grpc.stub.StreamObserver<io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionResponse> responseObserver) {
      io.grpc.stub.ServerCalls.asyncUnimplementedUnaryCall(getExecuteActionMethod(), responseObserver);
    }

    /**
     */
    default void executeActionSequences(io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionSequencesRequest request,
        io.grpc.stub.StreamObserver<io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionSequencesResponse> responseObserver) {
      io.grpc.stub.ServerCalls.asyncUnimplementedUnaryCall(getExecuteActionSequencesMethod(), responseObserver);
    }
  }

  /**
   * Base class for the server implementation of the service FizzBeeMbtPluginService.
   * <pre>
   * ------------------ Service Definition ------------------
   * </pre>
   */
  public static abstract class FizzBeeMbtPluginServiceImplBase
      implements io.grpc.BindableService, AsyncService {

    @java.lang.Override public final io.grpc.ServerServiceDefinition bindService() {
      return FizzBeeMbtPluginServiceGrpc.bindService(this);
    }
  }

  /**
   * A stub to allow clients to do asynchronous rpc calls to service FizzBeeMbtPluginService.
   * <pre>
   * ------------------ Service Definition ------------------
   * </pre>
   */
  public static final class FizzBeeMbtPluginServiceStub
      extends io.grpc.stub.AbstractAsyncStub<FizzBeeMbtPluginServiceStub> {
    private FizzBeeMbtPluginServiceStub(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      super(channel, callOptions);
    }

    @java.lang.Override
    protected FizzBeeMbtPluginServiceStub build(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      return new FizzBeeMbtPluginServiceStub(channel, callOptions);
    }

    /**
     */
    public void init(io.fizzbee.mbt.pb.MbtPlugin.InitRequest request,
        io.grpc.stub.StreamObserver<io.fizzbee.mbt.pb.MbtPlugin.InitResponse> responseObserver) {
      io.grpc.stub.ClientCalls.asyncUnaryCall(
          getChannel().newCall(getInitMethod(), getCallOptions()), request, responseObserver);
    }

    /**
     */
    public void cleanup(io.fizzbee.mbt.pb.MbtPlugin.CleanupRequest request,
        io.grpc.stub.StreamObserver<io.fizzbee.mbt.pb.MbtPlugin.CleanupResponse> responseObserver) {
      io.grpc.stub.ClientCalls.asyncUnaryCall(
          getChannel().newCall(getCleanupMethod(), getCallOptions()), request, responseObserver);
    }

    /**
     */
    public void executeAction(io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionRequest request,
        io.grpc.stub.StreamObserver<io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionResponse> responseObserver) {
      io.grpc.stub.ClientCalls.asyncUnaryCall(
          getChannel().newCall(getExecuteActionMethod(), getCallOptions()), request, responseObserver);
    }

    /**
     */
    public void executeActionSequences(io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionSequencesRequest request,
        io.grpc.stub.StreamObserver<io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionSequencesResponse> responseObserver) {
      io.grpc.stub.ClientCalls.asyncUnaryCall(
          getChannel().newCall(getExecuteActionSequencesMethod(), getCallOptions()), request, responseObserver);
    }
  }

  /**
   * A stub to allow clients to do synchronous rpc calls to service FizzBeeMbtPluginService.
   * <pre>
   * ------------------ Service Definition ------------------
   * </pre>
   */
  public static final class FizzBeeMbtPluginServiceBlockingV2Stub
      extends io.grpc.stub.AbstractBlockingStub<FizzBeeMbtPluginServiceBlockingV2Stub> {
    private FizzBeeMbtPluginServiceBlockingV2Stub(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      super(channel, callOptions);
    }

    @java.lang.Override
    protected FizzBeeMbtPluginServiceBlockingV2Stub build(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      return new FizzBeeMbtPluginServiceBlockingV2Stub(channel, callOptions);
    }

    /**
     */
    public io.fizzbee.mbt.pb.MbtPlugin.InitResponse init(io.fizzbee.mbt.pb.MbtPlugin.InitRequest request) throws io.grpc.StatusException {
      return io.grpc.stub.ClientCalls.blockingV2UnaryCall(
          getChannel(), getInitMethod(), getCallOptions(), request);
    }

    /**
     */
    public io.fizzbee.mbt.pb.MbtPlugin.CleanupResponse cleanup(io.fizzbee.mbt.pb.MbtPlugin.CleanupRequest request) throws io.grpc.StatusException {
      return io.grpc.stub.ClientCalls.blockingV2UnaryCall(
          getChannel(), getCleanupMethod(), getCallOptions(), request);
    }

    /**
     */
    public io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionResponse executeAction(io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionRequest request) throws io.grpc.StatusException {
      return io.grpc.stub.ClientCalls.blockingV2UnaryCall(
          getChannel(), getExecuteActionMethod(), getCallOptions(), request);
    }

    /**
     */
    public io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionSequencesResponse executeActionSequences(io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionSequencesRequest request) throws io.grpc.StatusException {
      return io.grpc.stub.ClientCalls.blockingV2UnaryCall(
          getChannel(), getExecuteActionSequencesMethod(), getCallOptions(), request);
    }
  }

  /**
   * A stub to allow clients to do limited synchronous rpc calls to service FizzBeeMbtPluginService.
   * <pre>
   * ------------------ Service Definition ------------------
   * </pre>
   */
  public static final class FizzBeeMbtPluginServiceBlockingStub
      extends io.grpc.stub.AbstractBlockingStub<FizzBeeMbtPluginServiceBlockingStub> {
    private FizzBeeMbtPluginServiceBlockingStub(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      super(channel, callOptions);
    }

    @java.lang.Override
    protected FizzBeeMbtPluginServiceBlockingStub build(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      return new FizzBeeMbtPluginServiceBlockingStub(channel, callOptions);
    }

    /**
     */
    public io.fizzbee.mbt.pb.MbtPlugin.InitResponse init(io.fizzbee.mbt.pb.MbtPlugin.InitRequest request) {
      return io.grpc.stub.ClientCalls.blockingUnaryCall(
          getChannel(), getInitMethod(), getCallOptions(), request);
    }

    /**
     */
    public io.fizzbee.mbt.pb.MbtPlugin.CleanupResponse cleanup(io.fizzbee.mbt.pb.MbtPlugin.CleanupRequest request) {
      return io.grpc.stub.ClientCalls.blockingUnaryCall(
          getChannel(), getCleanupMethod(), getCallOptions(), request);
    }

    /**
     */
    public io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionResponse executeAction(io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionRequest request) {
      return io.grpc.stub.ClientCalls.blockingUnaryCall(
          getChannel(), getExecuteActionMethod(), getCallOptions(), request);
    }

    /**
     */
    public io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionSequencesResponse executeActionSequences(io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionSequencesRequest request) {
      return io.grpc.stub.ClientCalls.blockingUnaryCall(
          getChannel(), getExecuteActionSequencesMethod(), getCallOptions(), request);
    }
  }

  /**
   * A stub to allow clients to do ListenableFuture-style rpc calls to service FizzBeeMbtPluginService.
   * <pre>
   * ------------------ Service Definition ------------------
   * </pre>
   */
  public static final class FizzBeeMbtPluginServiceFutureStub
      extends io.grpc.stub.AbstractFutureStub<FizzBeeMbtPluginServiceFutureStub> {
    private FizzBeeMbtPluginServiceFutureStub(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      super(channel, callOptions);
    }

    @java.lang.Override
    protected FizzBeeMbtPluginServiceFutureStub build(
        io.grpc.Channel channel, io.grpc.CallOptions callOptions) {
      return new FizzBeeMbtPluginServiceFutureStub(channel, callOptions);
    }

    /**
     */
    public com.google.common.util.concurrent.ListenableFuture<io.fizzbee.mbt.pb.MbtPlugin.InitResponse> init(
        io.fizzbee.mbt.pb.MbtPlugin.InitRequest request) {
      return io.grpc.stub.ClientCalls.futureUnaryCall(
          getChannel().newCall(getInitMethod(), getCallOptions()), request);
    }

    /**
     */
    public com.google.common.util.concurrent.ListenableFuture<io.fizzbee.mbt.pb.MbtPlugin.CleanupResponse> cleanup(
        io.fizzbee.mbt.pb.MbtPlugin.CleanupRequest request) {
      return io.grpc.stub.ClientCalls.futureUnaryCall(
          getChannel().newCall(getCleanupMethod(), getCallOptions()), request);
    }

    /**
     */
    public com.google.common.util.concurrent.ListenableFuture<io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionResponse> executeAction(
        io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionRequest request) {
      return io.grpc.stub.ClientCalls.futureUnaryCall(
          getChannel().newCall(getExecuteActionMethod(), getCallOptions()), request);
    }

    /**
     */
    public com.google.common.util.concurrent.ListenableFuture<io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionSequencesResponse> executeActionSequences(
        io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionSequencesRequest request) {
      return io.grpc.stub.ClientCalls.futureUnaryCall(
          getChannel().newCall(getExecuteActionSequencesMethod(), getCallOptions()), request);
    }
  }

  private static final int METHODID_INIT = 0;
  private static final int METHODID_CLEANUP = 1;
  private static final int METHODID_EXECUTE_ACTION = 2;
  private static final int METHODID_EXECUTE_ACTION_SEQUENCES = 3;

  private static final class MethodHandlers<Req, Resp> implements
      io.grpc.stub.ServerCalls.UnaryMethod<Req, Resp>,
      io.grpc.stub.ServerCalls.ServerStreamingMethod<Req, Resp>,
      io.grpc.stub.ServerCalls.ClientStreamingMethod<Req, Resp>,
      io.grpc.stub.ServerCalls.BidiStreamingMethod<Req, Resp> {
    private final AsyncService serviceImpl;
    private final int methodId;

    MethodHandlers(AsyncService serviceImpl, int methodId) {
      this.serviceImpl = serviceImpl;
      this.methodId = methodId;
    }

    @java.lang.Override
    @java.lang.SuppressWarnings("unchecked")
    public void invoke(Req request, io.grpc.stub.StreamObserver<Resp> responseObserver) {
      switch (methodId) {
        case METHODID_INIT:
          serviceImpl.init((io.fizzbee.mbt.pb.MbtPlugin.InitRequest) request,
              (io.grpc.stub.StreamObserver<io.fizzbee.mbt.pb.MbtPlugin.InitResponse>) responseObserver);
          break;
        case METHODID_CLEANUP:
          serviceImpl.cleanup((io.fizzbee.mbt.pb.MbtPlugin.CleanupRequest) request,
              (io.grpc.stub.StreamObserver<io.fizzbee.mbt.pb.MbtPlugin.CleanupResponse>) responseObserver);
          break;
        case METHODID_EXECUTE_ACTION:
          serviceImpl.executeAction((io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionRequest) request,
              (io.grpc.stub.StreamObserver<io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionResponse>) responseObserver);
          break;
        case METHODID_EXECUTE_ACTION_SEQUENCES:
          serviceImpl.executeActionSequences((io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionSequencesRequest) request,
              (io.grpc.stub.StreamObserver<io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionSequencesResponse>) responseObserver);
          break;
        default:
          throw new AssertionError();
      }
    }

    @java.lang.Override
    @java.lang.SuppressWarnings("unchecked")
    public io.grpc.stub.StreamObserver<Req> invoke(
        io.grpc.stub.StreamObserver<Resp> responseObserver) {
      switch (methodId) {
        default:
          throw new AssertionError();
      }
    }
  }

  public static final io.grpc.ServerServiceDefinition bindService(AsyncService service) {
    return io.grpc.ServerServiceDefinition.builder(getServiceDescriptor())
        .addMethod(
          getInitMethod(),
          io.grpc.stub.ServerCalls.asyncUnaryCall(
            new MethodHandlers<
              io.fizzbee.mbt.pb.MbtPlugin.InitRequest,
              io.fizzbee.mbt.pb.MbtPlugin.InitResponse>(
                service, METHODID_INIT)))
        .addMethod(
          getCleanupMethod(),
          io.grpc.stub.ServerCalls.asyncUnaryCall(
            new MethodHandlers<
              io.fizzbee.mbt.pb.MbtPlugin.CleanupRequest,
              io.fizzbee.mbt.pb.MbtPlugin.CleanupResponse>(
                service, METHODID_CLEANUP)))
        .addMethod(
          getExecuteActionMethod(),
          io.grpc.stub.ServerCalls.asyncUnaryCall(
            new MethodHandlers<
              io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionRequest,
              io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionResponse>(
                service, METHODID_EXECUTE_ACTION)))
        .addMethod(
          getExecuteActionSequencesMethod(),
          io.grpc.stub.ServerCalls.asyncUnaryCall(
            new MethodHandlers<
              io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionSequencesRequest,
              io.fizzbee.mbt.pb.MbtPlugin.ExecuteActionSequencesResponse>(
                service, METHODID_EXECUTE_ACTION_SEQUENCES)))
        .build();
  }

  private static abstract class FizzBeeMbtPluginServiceBaseDescriptorSupplier
      implements io.grpc.protobuf.ProtoFileDescriptorSupplier, io.grpc.protobuf.ProtoServiceDescriptorSupplier {
    FizzBeeMbtPluginServiceBaseDescriptorSupplier() {}

    @java.lang.Override
    public com.google.protobuf.Descriptors.FileDescriptor getFileDescriptor() {
      return io.fizzbee.mbt.pb.MbtPlugin.getDescriptor();
    }

    @java.lang.Override
    public com.google.protobuf.Descriptors.ServiceDescriptor getServiceDescriptor() {
      return getFileDescriptor().findServiceByName("FizzBeeMbtPluginService");
    }
  }

  private static final class FizzBeeMbtPluginServiceFileDescriptorSupplier
      extends FizzBeeMbtPluginServiceBaseDescriptorSupplier {
    FizzBeeMbtPluginServiceFileDescriptorSupplier() {}
  }

  private static final class FizzBeeMbtPluginServiceMethodDescriptorSupplier
      extends FizzBeeMbtPluginServiceBaseDescriptorSupplier
      implements io.grpc.protobuf.ProtoMethodDescriptorSupplier {
    private final java.lang.String methodName;

    FizzBeeMbtPluginServiceMethodDescriptorSupplier(java.lang.String methodName) {
      this.methodName = methodName;
    }

    @java.lang.Override
    public com.google.protobuf.Descriptors.MethodDescriptor getMethodDescriptor() {
      return getServiceDescriptor().findMethodByName(methodName);
    }
  }

  private static volatile io.grpc.ServiceDescriptor serviceDescriptor;

  public static io.grpc.ServiceDescriptor getServiceDescriptor() {
    io.grpc.ServiceDescriptor result = serviceDescriptor;
    if (result == null) {
      synchronized (FizzBeeMbtPluginServiceGrpc.class) {
        result = serviceDescriptor;
        if (result == null) {
          serviceDescriptor = result = io.grpc.ServiceDescriptor.newBuilder(SERVICE_NAME)
              .setSchemaDescriptor(new FizzBeeMbtPluginServiceFileDescriptorSupplier())
              .addMethod(getInitMethod())
              .addMethod(getCleanupMethod())
              .addMethod(getExecuteActionMethod())
              .addMethod(getExecuteActionSequencesMethod())
              .build();
        }
      }
    }
    return result;
  }
}
