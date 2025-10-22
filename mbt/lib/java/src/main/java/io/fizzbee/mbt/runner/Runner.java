package io.fizzbee.mbt.runner;

import io.fizzbee.mbt.types.Model;
import io.grpc.Server;
import io.grpc.ServerBuilder;
import io.grpc.netty.NettyServerBuilder;
import io.grpc.protobuf.services.ProtoReflectionService;
import io.netty.channel.EventLoopGroup;
import io.netty.channel.epoll.EpollEventLoopGroup;
import io.netty.channel.epoll.EpollServerDomainSocketChannel;
import io.netty.channel.kqueue.KQueueEventLoopGroup;
import io.netty.channel.kqueue.KQueueServerDomainSocketChannel;
import io.netty.channel.unix.DomainSocketAddress;

import java.io.File;
import java.io.IOException;
import java.lang.reflect.Method;
import java.nio.file.Files;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.concurrent.*;

public class Runner {
    private Server server;
    private Process childProcess;
    private final int port = 50051; // default TCP port fallback
    private EventLoopGroup group;

    private static final Map<String, String> configEnvMap = Map.of(
        "max-seq-runs", "MAX_SEQ_RUNS",
        "max-parallel-runs", "MAX_PARALLEL_RUNS",
        "max-actions", "MAX_ACTIONS",
        "seq-seed", "SEQ_SEED",
        "parallel-seed", "PARALLEL_SEED"
    );
    public static int run(Model m, Map<String, Map<String, Method>> actions, Map<String, Object> options) throws IOException, InterruptedException {
        Runner r = new Runner();
        return r.start(m, actions, options);
    }

    public int start(Model m, Map<String, Map<String, Method>> actions, Map<String, Object> options) throws IOException, InterruptedException {
        System.out.println("Starting model-based testing framework...");

        // 1. Start gRPC service
        String socketPath = startGrpcService(m, actions);

        // 2. Once started, launch child process
        startChildProcess(socketPath, options);

        // 3. Wait for process completion
        int exitCode = childProcess.waitFor();
        System.out.println("Child process exited with code: " + exitCode);

        // 4. Gracefully shutdown the gRPC server
        shutdownServerGracefully();

        System.out.println("Runner completed.");
        return exitCode;
    }

    private String startGrpcService(Model m, Map<String, Map<String, Method>> actions) throws IOException {
        String socketPath;

        try {
            File tmpDir = Files.createTempDirectory("fizzbee-mbt-").toFile();
            tmpDir.deleteOnExit();
            socketPath = new File(tmpDir, "plugin.sock").getAbsolutePath();
            DomainSocketAddress domainSocketAddress = new DomainSocketAddress(socketPath);

            System.out.println("Using Unix domain socket at: " + socketPath);

            NettyServerBuilder builder = NettyServerBuilder.forAddress(domainSocketAddress)
                    .addService(new PluginService(m, actions))
                    .addService(ProtoReflectionService.newInstance());

            // 🔹 Determine transport (Linux = Epoll, macOS = KQueue)
            if (io.netty.channel.epoll.Epoll.isAvailable()) {
                group = new EpollEventLoopGroup();
                builder.channelType(EpollServerDomainSocketChannel.class);
                builder.workerEventLoopGroup(group);
                builder.bossEventLoopGroup(group);
                System.out.println("Using Epoll for UDS");
            } else if (io.netty.channel.kqueue.KQueue.isAvailable()) {
                group = new KQueueEventLoopGroup();
                builder.channelType(KQueueServerDomainSocketChannel.class);
                builder.workerEventLoopGroup(group);
                builder.bossEventLoopGroup(group);
                System.out.println("Using KQueue for UDS");
            } else {
                throw new UnsupportedOperationException("Native UDS not supported on this platform");
            }

            server = builder.build().start();
        } catch (UnsupportedOperationException e) {
            // 🔹 Fallback to TCP if UDS not available
            System.out.println("UDS not supported, falling back to TCP port " + port);
            server = ServerBuilder.forPort(port)
                    .addService(new PluginService(m, actions))
                    .addService(ProtoReflectionService.newInstance())
                    .build()
                    .start();
            socketPath = "localhost:" + port;
        }

        System.out.println("Server started and listening on " + socketPath);

        Runtime.getRuntime().addShutdownHook(new Thread(() -> {
            System.out.println("Shutdown signal received. Cleaning up...");
            stopChildProcess();
            stopServer();
            System.out.println("Server shut down.");
        }));

        return socketPath;
    }

    private void startChildProcess(String socketPath, Map<String, Object> options) throws IOException {
        // 1️⃣ Resolve fizzbee-mbt binary path
        String fizzbeeMbtBin = System.getenv("FIZZBEE_MBT_BIN");
        if (fizzbeeMbtBin == null || fizzbeeMbtBin.isEmpty()) {
            fizzbeeMbtBin = "fizzbee-mbt-runner";  // Default fallback
        }

        // 2️⃣ Build the command
        List<String> command = new ArrayList<>();
        command.add(fizzbeeMbtBin);
        command.add("--plugin-addr=" + socketPath);

        // Iterate over all the configenvmap to add options from environment variables
        // If the environment variable is set, it takes precedence over the options map
        for (Map.Entry<String, String> entry : configEnvMap.entrySet()) {
            String key = entry.getKey();
            String envName = entry.getValue();
            Object value = null;
            // Check if the option is provided in the options map
            if (options != null && options.containsKey(key)) {
                value = options.get(key);
            }
            // Override from environment variable if set
            String envValue = System.getenv(envName);
            if (envValue != null && !envValue.isEmpty()) {
                value = envValue;
            }
            if (value != null) {
                command.add("--" + key + "=" + value);
            }
        }

        // 3️⃣ Create the process builder
        ProcessBuilder pb = new ProcessBuilder(command);

        // 4️⃣ Forward stdout/stderr to parent
        pb.inheritIO();

        // 5️⃣ Start the process
        System.out.println("Starting child process: " + String.join(" ", command));
        childProcess = pb.start();
    }


    private void shutdownServerGracefully() {
        System.out.println("Shutting down gRPC server gracefully...");

        ExecutorService executor = Executors.newSingleThreadExecutor();
        Future<?> shutdownFuture = executor.submit(() -> {
            server.shutdown();
            try {
                if (!server.awaitTermination(5, TimeUnit.SECONDS)) {
                    System.out.println("Forcing gRPC server stop...");
                    server.shutdownNow();
                }
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                server.shutdownNow();
            }
        });

        try {
            shutdownFuture.get(6, TimeUnit.SECONDS);
        } catch (TimeoutException | InterruptedException | ExecutionException e) {
            System.err.println("Failed to shut down server cleanly: " + e.getMessage());
            server.shutdownNow();
        } finally {
            executor.shutdownNow();
        }
        if (group != null) {
            group.shutdownGracefully();
        }
        System.out.println("gRPC server stopped.");
    }

    private void stopChildProcess() {
        if (childProcess != null && childProcess.isAlive()) {
            System.out.println("Killing child process...");
            childProcess.destroy();
            try {
                if (!childProcess.waitFor(3, TimeUnit.SECONDS)) {
                    System.out.println("Force killing child process...");
                    childProcess.destroyForcibly();
                }
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                childProcess.destroyForcibly();
            }
        }
    }

    private void stopServer() {
        if (server != null) {
            server.shutdownNow();
        }
    }
}

