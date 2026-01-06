# FizzBee Model Checker Library

This package provides a minimal library interface for running FizzBee model checker simulations from Go code.

## Features

- Run single simulation with specific seed
- No command-line flags or filesystem dependencies
- In-memory state graph results
- Pre-initialization hook support

## Usage

### For Bazel Projects

In your `WORKSPACE` file, add the fizzbee repository:

```starlark
# Option 1: Using http_archive (for released versions)
load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

http_archive(
    name = "fizzbee",
    urls = ["https://github.com/fizzbee-io/fizzbee/archive/refs/tags/v0.x.x.tar.gz"],
    strip_prefix = "fizzbee-0.x.x",
    # sha256 = "...",
)

# Option 2: Using local_repository (for development)
local_repository(
    name = "fizzbee",
    path = "/path/to/fizzbee",
)

# Option 3: Using git_repository (for latest)
load("@bazel_tools//tools/build_defs/repo:git.bzl", "git_repository")

git_repository(
    name = "fizzbee",
    remote = "https://github.com/fizzbee-io/fizzbee.git",
    branch = "main",  # or specific tag/commit
)
```

In your `BUILD` file:

```starlark
load("@rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "mylib",
    srcs = ["main.go"],
    deps = [
        "@fizzbee//pkg/modelchecker",
    ],
)
```

### For Standard Go Projects

```bash
go get github.com/fizzbee-io/fizzbee/pkg/modelchecker
```

## Example

```go
package main

import (
    "fmt"
    "os"

    "github.com/fizzbee-io/fizzbee/pkg/modelchecker"
)

func main() {
    // Load your spec (JSON AST format)
    specData, err := os.ReadFile("spec_ast.json")
    if err != nil {
        panic(err)
    }

    // Configure the simulation
    config := &modelchecker.SimulationConfig{
        Seed:        12345,
        PreinitHook: "x = 10",  // Optional: set initial values
    }

    // Run the simulation
    result := modelchecker.RunSimulation(specData, config)
    if result.Error != nil {
        panic(result.Error)
    }

    // Check results
    if result.FailedNode != nil {
        fmt.Println("Invariant violation found!")
        fmt.Printf("Failed at state: %v\n", result.FailedNode)
    } else {
        fmt.Println("No violations found")
    }

    // Access the state graph
    fmt.Printf("Initial state: %v\n", result.InitNode)
}
```

## API Reference

### `RunSimulation(specData []byte, config *SimulationConfig) *SimulationResult`

Runs a single model checking simulation.

**Parameters:**
- `specData`: JSON AST representation of your FizzBee specification
- `config`: Configuration options (can be nil for defaults)

**Returns:**
- `SimulationResult` containing the state graph and any failures

### `SimulationConfig`

Configuration for simulation runs:

```go
type SimulationConfig struct {
    Seed        int64   // Random seed (0 = use timestamp)
    PreinitHook string  // Starlark code to run before init
    Options     *ast.StateSpaceOptions  // State space options
    DirPath     string  // Directory for loading modules
}
```

### `SimulationResult`

Results from a simulation:

```go
type SimulationResult struct {
    InitNode   *modelchecker.Node  // Root of state graph
    FailedNode *modelchecker.Node  // Node where invariant failed (nil if success)
    Error      error               // Any error during simulation
}
```

## Advanced Usage

### With State Space Options

```go
import ast "fizz/proto"

options := &ast.StateSpaceOptions{
    MaxActions: proto.Int64(1000),
    MaxConcurrentActions: proto.Int32(10),
}

config := &modelchecker.SimulationConfig{
    Seed:    42,
    Options: options,
}

result := modelchecker.RunSimulation(specData, config)
```

### Loading Spec from JSON

```go
file, err := modelchecker.LoadSpecFromJSON(specData)
if err != nil {
    panic(err)
}
// Use file.Actions, file.Invariants, etc.
```

## Notes

- This library uses the existing `modelchecker.Processor` under the hood
- State graphs are kept in memory - for large state spaces, consider running as CLI
- For multiple simulation runs with different seeds, call `RunSimulation` multiple times
- The library is safe for concurrent use (each call creates a new Processor)

## Differences from CLI

The library interface:
- ✅ Does NOT touch the filesystem (except for module loading if DirPath is set)
- ✅ Does NOT parse command-line flags
- ✅ Returns in-memory results instead of writing files
- ✅ Allows programmatic access to the state graph
- ❌ Does NOT support composition or refinement checking (yet)
- ❌ Does NOT generate DOT files or HTML reports automatically
- ❌ Does NOT create output directories

If you need these features, you can:
1. Use the existing `modelchecker` package directly (more complex API)
2. Use the CLI tool and parse its output files
3. Contribute to extend this library!
