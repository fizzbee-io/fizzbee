// Package modelchecker provides a simple library interface for running FizzBee simulations.
package modelchecker

import (
	ast "fizz/proto"

	"github.com/fizzbee-io/fizzbee/modelchecker"
	"google.golang.org/protobuf/encoding/protojson"
)

// VisitedMapTracking controls whether the visited map should track visited states
type VisitedMapTracking int

const (
	// VisitedMapTrackingDefault uses the current logic based on liveness settings
	VisitedMapTrackingDefault VisitedMapTracking = 0
	// VisitedMapTrackingEnabled always tracks visited states (don't clear the map)
	VisitedMapTrackingEnabled VisitedMapTracking = 1
	// VisitedMapTrackingDisabled always clears the map (no loop detection)
	VisitedMapTrackingDisabled VisitedMapTracking = 2
)

// SimulationConfig contains configuration for a single simulation run
type SimulationConfig struct {
	// Seed for random number generation (0 = use timestamp)
	Seed int64

	// PreinitHook is Starlark code to run before initialization
	PreinitHook string

	// Options for state space exploration (nil = use defaults)
	Options *ast.StateSpaceOptions

	// DirPath is the directory for loading modules (empty = no modules)
	DirPath string

	// VisitedMapTracking controls loop detection behavior
	// Default: uses liveness-based logic
	// Enabled: always track visited states
	// Disabled: always clear visited map (no loop detection)
	VisitedMapTracking VisitedMapTracking
}

// SimulationResult contains the results of a simulation run
type SimulationResult struct {
	// InitNode is the root node of the state graph
	InitNode *modelchecker.Node

	// FailedNode is the node where an invariant failed (nil if no failure)
	FailedNode *modelchecker.Node

	// Error is any error that occurred during simulation
	Error error
}

// RunSimulation performs a single simulation run on the given specification.
// This is a minimal wrapper around the existing Processor for library use.
//
// Example:
//
//	specData := []byte(`{"actions": [...]}`) // JSON AST
//	result := modelchecker.RunSimulation(specData, &modelchecker.SimulationConfig{
//	    Seed: 12345,
//	    PreinitHook: "initial_value = 10",
//	})
//	if result.FailedNode != nil {
//	    fmt.Println("Invariant violation found!")
//	}
func RunSimulation(specData []byte, config *SimulationConfig) *SimulationResult {
	if config == nil {
		config = &SimulationConfig{}
	}

	// Parse the spec
	file := &ast.File{}
	err := protojson.Unmarshal(specData, file)
	if err != nil {
		return &SimulationResult{Error: err}
	}

	// Use default options if not provided
	options := config.Options
	if options == nil {
		options = &ast.StateSpaceOptions{}
	}

	// Create processor for simulation mode
	processor := modelchecker.NewProcessorWithVisitedTracking(
		[]*ast.File{file},  // files
		options,            // options
		true,               // simulation = true
		config.Seed,        // seed
		config.DirPath,     // dirPath
		"random",           // strategy (doesn't matter much in simulation mode)
		false,              // test = false
		nil,                // hashes (for composition, not needed)
		nil,                // trace (not needed for basic simulation)
		config.PreinitHook, // preinitHookContent

		int(config.VisitedMapTracking), // visitedMapTracking
	)

	// Run the simulation
	initNode, failedNode, err := processor.StartSimulation()

	return &SimulationResult{
		InitNode:   initNode,
		FailedNode: failedNode,
		Error:      err,
	}
}

// LoadSpecFromJSON is a convenience function to load a spec from JSON bytes
func LoadSpecFromJSON(data []byte) (*ast.File, error) {
	file := &ast.File{}
	err := protojson.Unmarshal(data, file)
	return file, err
}
