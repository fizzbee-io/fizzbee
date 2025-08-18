package mbt

import "errors"

// ErrNotImplemented is returned by unimplemented stub methods.
var ErrNotImplemented = errors.New("not implemented")

// Arg represents a choice variable passed to action methods.
type Arg struct {
	Name  string
	Value any
}

// Role is the base interface that all role interfaces should embed.
type Role interface {
	// At present, no methods are defined here, but this can be extended
}

type Model interface {
	StateGetter
	Init() error
	Cleanup() error
}

// StateGetter is an interface for roles that can return their current state.
// When implemented, test can use these to assert the next state of the role.
// If both GetState() and SnapshotState() methods are implemented, SnapshotState takes precedence.
type StateGetter interface {
	// GetState returns the current state without guaranteeing thread safety.
	GetState() (map[string]any, error)
}

// SnapshotStateGetter is an interface for roles that can return a consistent snapshot of their state.
// When implemented, this method would be used concurrently with the role's actions, to test the intermediate states.
// Sometimes, implementing these would be hard, so it is not required but recommended.
// If both GetState() and SnapshotState() methods are implemented, SnapshotState takes precedence.
type SnapshotStateGetter interface {
	// SnapshotState returns a consistent, concurrency-safe snapshot of the state.
	SnapshotState() (map[string]any, error)
}
