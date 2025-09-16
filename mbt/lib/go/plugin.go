package mbt

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	pb "github.com/fizzbee-io/fizzbee/mbt/lib/go/internalpb"
	"google.golang.org/grpc"
)

var (
	seqSeedFlag         int64
	parallelSeedFlag    int64
	maxSeqRunsFlag      int
	maxParallelRunsFlag int
	maxActionsFlag      int
	fizzbeeMbtBinFlag   string
)

func ParseFlags() []string {
	flag.Int64Var(&seqSeedFlag, "seq-seed", 0, "Random seed for sequential tests")
	flag.Int64Var(&parallelSeedFlag, "parallel-seed", 0, "Random seed for parallel tests")
	flag.IntVar(&maxSeqRunsFlag, "max-seq-runs", -1, "Number of runs for the sequential tests")
	flag.IntVar(&maxParallelRunsFlag, "max-parallel-runs", -1, "Number of runs for the parallel tests")
	flag.IntVar(&maxActionsFlag, "max-actions", -1, "Number of actions to execute")
	flag.StringVar(&fizzbeeMbtBinFlag, "fizzbee-mbt-bin", "", "Command to run the test runner")

	flag.Parse()

	args := flag.Args()
	return args
}

// NetworkType represents the type of listener
type NetworkType string

const (
	NetworkUDS NetworkType = "unix"
	NetworkTCP NetworkType = "tcp"
)

// StartOptions holds parameters for the plugin server
type StartOptions struct {
	Network NetworkType // "unix" for UDS, "tcp" for TCP
	Address string      // socket path for UDS, host:port for TCP
}

type ActionFunc func(inst any, args []Arg) (any, error)

func RunTests(t *testing.T, m Model, actionsRegistry map[string]map[string]ActionFunc, options map[string]any) error {
	// 1. Create temp socket path
	tmpDir, err := os.MkdirTemp("", "fizzbee-mbt-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	socketPath := filepath.Join(tmpDir, "plugin.sock")

	// 2. Start gRPC server
	lis, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on UDS: %w", err)
	}

	server := grpc.NewServer()
	pb.RegisterFizzBeeMbtPluginServiceServer(server, NewFizzBeeMbtPluginServer(t, m, actionsRegistry))

	startedCh := make(chan struct{})

	// signal handler
	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, syscall.SIGINT, syscall.SIGTERM)

	// background goroutine: serve + stop handling
	go func() {
		close(startedCh) // signal readiness
		if err := server.Serve(lis); err != nil {
			log.Printf("server stopped: %v", err)
		}
	}()

	// 3. Wait for server started, then launch runner process
	<-startedCh

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runnerCmd := GetMbtBinPath()
	cmd := exec.CommandContext(ctx, runnerCmd)
	// append plugin address
	cmd.Args = append(cmd.Args, fmt.Sprintf("--plugin-addr=%s", socketPath))
	for optionName, optionValue := range options {
		if optionName == "max-actions" && maxActionsFlag >= 0 {
			optionValue = maxActionsFlag
		} else if optionName == "max-seq-runs" && maxSeqRunsFlag >= 0 {
			optionValue = maxSeqRunsFlag
		} else if optionName == "max-parallel-runs" && maxParallelRunsFlag >= 0 {
			optionValue = maxParallelRunsFlag
		}
		cmd.Args = append(cmd.Args, fmt.Sprintf("--%s=%v", optionName, optionValue))
	}
	if seqSeedFlag != 0 {
		cmd.Args = append(cmd.Args, fmt.Sprintf("--seq-seed=%d", seqSeedFlag))
	}
	if parallelSeedFlag != 0 {
		cmd.Args = append(cmd.Args, fmt.Sprintf("--parallel-seed=%d", parallelSeedFlag))
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start runner: %w", err)
	}

	// 4. Watch for signals
	go func() {
		<-stopCh
		log.Println("Interrupt received, stopping runner and server...")
		cancel() // kill child process
		server.GracefulStop()
	}()

	// 5. Wait for child completion
	err = cmd.Wait()
	if err != nil {
		log.Printf("Runner exited with error: %v", err)
		server.GracefulStop()
		return err
	}

	// 6. Graceful shutdown of server
	shutdownCh := make(chan struct{})
	go func() {
		server.GracefulStop()
		close(shutdownCh)
	}()

	select {
	case <-shutdownCh:
		log.Println("Server shut down gracefully.")
	case <-time.After(5 * time.Second):
		log.Println("Forcing server stop.")
		server.Stop()
	}

	return nil
}

// GetMbtBinPath returns the path to the fizzbee-mbt binary
func GetMbtBinPath() string {
	// 1. Command-line flag takes highest priority
	if fizzbeeMbtBinFlag != "" {
		return fizzbeeMbtBinFlag
	}

	// 2. Environment variable
	if envBin := os.Getenv("FIZZBEE_MBT_BIN"); envBin != "" {
		return envBin
	}

	// 3. Default fallback
	return "fizzbee-mbt"
}
