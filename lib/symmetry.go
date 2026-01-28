package lib

import (
	"errors"
	"fmt"
	"sort"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// SymmetryKind represents the type of symmetry for a domain.
// Different kinds have different operations and canonicalization strategies.
type SymmetryKind string

const (
	// SymmetryKindNominal represents unordered, distinct identifiers.
	// Operations: == and != only. Reduction: permutation of active set.
	// Use cases: User IDs, Session Tokens, Request IDs.
	SymmetryKindNominal SymmetryKind = "nominal"

	// SymmetryKindOrdinal represents ordered, dense values where only relative order matters.
	// Operations: ==, !=, <, >, <=, >=. Reduction: rank squashing.
	// Use cases: Physical Time, Logical Timestamps, Priority Ranks.
	SymmetryKindOrdinal SymmetryKind = "ordinal"

	// SymmetryKindInterval represents ordered values where distance is significant.
	// Operations: ==, !=, <, >, <=, >=, +, -. Reduction: zero-shifting.
	// Use cases: TCP Sequence Numbers, Raft Log Indices, Clock Drift.
	SymmetryKindInterval SymmetryKind = "interval"
)

// String returns the string representation of the SymmetryKind
func (k SymmetryKind) String() string {
	return string(k)
}

// SymmetryContextKey is the key used to store the context in the starlark Thread
const SymmetryContextKey = "symmetry_context"

// DisableTransitionError is returned when a symmetry operation cannot proceed
// (e.g., fresh() called when limit is reached). The modelchecker should
// disable the current transition rather than treating this as a fatal error.
type DisableTransitionError struct {
	Message string
}

func (e *DisableTransitionError) Error() string {
	return e.Message
}

// IsDisableTransitionError checks if an error is a DisableTransitionError.
// Uses errors.As to handle wrapped errors (e.g., from starlark).
func IsDisableTransitionError(err error) bool {
	var dte *DisableTransitionError
	return errors.As(err, &dte)
}

// SymmetryContext holds the state of used values for the current transition execution.
// It bridges the gap between the persistent state (Scanner) and the transient state (Cache).
type SymmetryContext struct {
	// Scanner is a callback that returns ALL currently used values from the process state.
	// It returns a map of DomainName -> List of IDs.
	Scanner func() map[string][]int

	// Cache stores the set of active IDs for the current execution.
	// It is populated initially by Scanner, and updated by Fresh().
	// Map: DomainName -> ID -> Exists
	Cache map[string]map[int]bool

	// initialized tracks if we have loaded the state from Scanner yet.
	initialized bool
}

// NewSymmetryContext creates a new symmetry context with the given scanner function
func NewSymmetryContext(scanner func() map[string][]int) *SymmetryContext {
	return &SymmetryContext{
		Scanner: scanner,
		Cache:   make(map[string]map[int]bool),
	}
}

// ensureLoaded guarantees that the Cache contains the baseline state from the process.
func (ctx *SymmetryContext) ensureLoaded() {
	if ctx.initialized {
		return
	}
	// Fetch baseline state from the process
	allUsed := ctx.Scanner()
	for name, ids := range allUsed {
		if ctx.Cache[name] == nil {
			ctx.Cache[name] = make(map[int]bool)
		}
		for _, id := range ids {
			ctx.Cache[name][id] = true
		}
	}
	ctx.initialized = true
}

// EnsureDomainInit makes sure the map entry exists for a domain, even if empty
func (ctx *SymmetryContext) EnsureDomainInit(name string) {
	ctx.ensureLoaded()
	if ctx.Cache[name] == nil {
		ctx.Cache[name] = make(map[int]bool)
	}
}

// SymmetryDomain represents a declared symmetry set (e.g., USERS, TIMES)
// This type is stateless - it only contains configuration, not runtime state.
// Runtime state is managed via the SymmetryContext in thread-local storage.
type SymmetryDomain struct {
	Name  string
	Limit int
	Kind  SymmetryKind
}

// Starlark Interface for SymmetryDomain
var _ starlark.Value = (*SymmetryDomain)(nil)
var _ starlark.HasAttrs = (*SymmetryDomain)(nil)

func (d *SymmetryDomain) String() string {
	return fmt.Sprintf("symmetry.%s(name=%q, limit=%d)", d.Kind, d.Name, d.Limit)
}

func (d *SymmetryDomain) Type() string         { return "symmetry_domain" }
func (d *SymmetryDomain) Freeze()              {}
func (d *SymmetryDomain) Truth() starlark.Bool { return starlark.True }

func (d *SymmetryDomain) Hash() (uint32, error) {
	return starlark.String(d.Name).Hash()
}

// Attr exposes methods like .fresh() and .values()
func (d *SymmetryDomain) Attr(name string) (starlark.Value, error) {
	switch name {
	case "fresh":
		return starlark.NewBuiltin("fresh", d.fresh), nil
	case "values":
		return starlark.NewBuiltin("values", d.values), nil
	case "name":
		return starlark.String(d.Name), nil
	case "limit":
		return starlark.MakeInt(d.Limit), nil
	}
	return nil, nil
}

func (d *SymmetryDomain) AttrNames() []string {
	return []string{"fresh", "values", "name", "limit"}
}

// fresh allocates a new deterministic value from this domain.
// Returns a DisableTransitionError if the limit is reached.
// Usage: domain.fresh()
func (d *SymmetryDomain) fresh(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs("fresh", args, kwargs); err != nil {
		return nil, err
	}

	// 1. Retrieve Context from thread-local storage
	ctxVal := thread.Local(SymmetryContextKey)
	if ctxVal == nil {
		return nil, fmt.Errorf("internal error: symmetry context not found in thread")
	}
	ctx := ctxVal.(*SymmetryContext)

	// 2. Ensure domain is initialized in cache
	ctx.EnsureDomainInit(d.Name)

	// 3. Check Limit - if reached, disable the transition
	if len(ctx.Cache[d.Name]) >= d.Limit {
		return nil, &DisableTransitionError{
			Message: fmt.Sprintf("symmetry limit reached for domain %q (limit %d)", d.Name, d.Limit),
		}
	}

	// 4. Allocate the smallest unused ID
	// For nominal symmetry, we must use the smallest unused ID to ensure canonical forms.
	// This is critical for symmetry reduction: states {id0, id1} and {id1, id2} should
	// be recognized as equivalent, which only happens if we always allocate canonically.
	nextID := 0
	for ctx.Cache[d.Name][nextID] {
		nextID++
	}

	// 5. Update Transient Cache (so subsequent fresh() calls in same expression see this)
	ctx.Cache[d.Name][nextID] = true

	return NewSymmetricValueWithKind(d.Name, nextID, d.Kind), nil
}

// values returns a list of all currently active values in this domain.
// Usage: domain.values()
func (d *SymmetryDomain) values(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs("values", args, kwargs); err != nil {
		return nil, err
	}

	// Retrieve Context
	ctxVal := thread.Local(SymmetryContextKey)
	if ctxVal == nil {
		return nil, fmt.Errorf("internal error: symmetry context not found in thread")
	}
	ctx := ctxVal.(*SymmetryContext)

	// Ensure domain is initialized
	ctx.EnsureDomainInit(d.Name)

	// Collect and Sort IDs for deterministic ordering
	var ids []int
	for id := range ctx.Cache[d.Name] {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	// Convert to SymmetricValue list
	elems := make([]starlark.Value, len(ids))
	for i, id := range ids {
		elems[i] = NewSymmetricValueWithKind(d.Name, id, d.Kind)
	}

	return starlark.NewList(elems), nil
}

// --- Module Construction ---

// SymmetryModule is the 'symmetry' module exposed to Starlark
var SymmetryModule = &starlarkstruct.Module{
	Name: "symmetry",
	Members: starlark.StringDict{
		"nominal": starlark.NewBuiltin("nominal", makeNominal),
	},
}

// makeNominal creates a new nominal symmetry domain
// Usage: symmetry.nominal(name="id", limit=3)
func makeNominal(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	var limit int
	if err := starlark.UnpackArgs("nominal", args, kwargs, "name", &name, "limit", &limit); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("nominal: limit must be positive, got %d", limit)
	}
	if name == "" {
		return nil, fmt.Errorf("nominal: name cannot be empty")
	}
	return &SymmetryDomain{Name: name, Limit: limit, Kind: SymmetryKindNominal}, nil
}
