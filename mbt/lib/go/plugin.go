package mbt

import (
	"fmt"
	"net"
	"os"
	"testing"

	pb "github.com/fizzbee-io/fizzbee/mbt/lib/go/internalpb"
	"google.golang.org/grpc"
)

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

// Start creates and starts the gRPC server on UDS
func Start(t *testing.T, m Model, actionsRegistry map[string]map[string]ActionFunc, opts StartOptions) error {
	if opts.Address == "" {
		return fmt.Errorf("SocketPath must be provided")
	}

	if opts.Network == NetworkUDS {
		// Remove UDS file if it already exists
		if _, err := os.Stat(opts.Address); err == nil {
			os.Remove(opts.Address)
		}
	}

	lis, err := net.Listen(string(opts.Network), opts.Address)
	if err != nil {
		return fmt.Errorf("failed to listen on %s %s: %w", opts.Network, opts.Address, err)
	}

	server := grpc.NewServer()

	// Register the plugin service
	pb.RegisterFizzBeeMbtPluginServiceServer(server, NewFizzBeeMbtPluginServer(t, m, actionsRegistry))

	fmt.Printf("Starting FizzBee MBT plugin server on %s: %s\n", opts.Network, opts.Address)
	return server.Serve(lis)
}
