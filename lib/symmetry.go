package lib

import (
	"errors"
	"fmt"
	"math"
	"slices"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

const (
	ordinalMax int64 = math.MaxInt64 / 2 // Use half of int64 range to keep ordinal values positive
	ordinalMid int64 = ordinalMax / 2
)

// midpoint returns lo + (hi - lo) / 2. Returns error if hi - lo < 2.
func midpoint(lo, hi int64) (int64, error) {
	if hi <= lo || hi-lo < 2 {
		return 0, fmt.Errorf("no space between %d and %d", lo, hi)
	}
	return lo + (hi-lo)/2, nil
}

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
	Scanner func() map[string][]int64

	// Cache stores the set of active IDs for the current execution.
	// It is populated initially by Scanner, and updated by Fresh().
	// Map: DomainName -> ID -> Exists
	Cache map[string]map[int64]bool

	// initialized tracks if we have loaded the state from Scanner yet.
	initialized bool
}

// NewSymmetryContext creates a new symmetry context with the given scanner function
func NewSymmetryContext(scanner func() map[string][]int64) *SymmetryContext {
	return &SymmetryContext{
		Scanner: scanner,
		Cache:   make(map[string]map[int64]bool),
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
			ctx.Cache[name] = make(map[int64]bool)
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
		ctx.Cache[name] = make(map[int64]bool)
	}
}

// SymmetryDomain represents a declared symmetry set (e.g., USERS, TIMES)
// This type is stateless - it only contains configuration, not runtime state.
// Runtime state is managed via the SymmetryContext in thread-local storage.
type SymmetryDomain struct {
	Name       string
	Limit      int
	Kind       SymmetryKind
	Divergence int
	Start      int
}

// Starlark Interface for SymmetryDomain
var _ starlark.Value = (*SymmetryDomain)(nil)
var _ starlark.HasAttrs = (*SymmetryDomain)(nil)

func (d *SymmetryDomain) String() string {
	if d.Kind == SymmetryKindInterval {
		return fmt.Sprintf("symmetry.%s(name=%q, limit=%d, divergence=%d, start=%d)", d.Kind, d.Name, d.Limit, d.Divergence, d.Start)
	}
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
	case "choices":
		if d.Kind == SymmetryKindNominal {
			return starlark.NewBuiltin("choices", d.choices), nil
		}
	case "values":
		return starlark.NewBuiltin("values", d.values), nil
	case "segments":
		if d.Kind == SymmetryKindOrdinal {
			return starlark.NewBuiltin("segments", d.segments), nil
		}
		return nil, nil // segments only available for ordinal
	case "name":
		return starlark.String(d.Name), nil
	case "limit":
		return starlark.MakeInt(d.Limit), nil
	case "divergence":
		if d.Kind == SymmetryKindInterval {
			return starlark.MakeInt(d.Divergence), nil
		}
	case "start":
		if d.Kind == SymmetryKindInterval {
			return starlark.MakeInt(d.Start), nil
		}
	}
	return nil, nil
}

func (d *SymmetryDomain) AttrNames() []string {
	if d.Kind == SymmetryKindOrdinal {
		return []string{"fresh", "values", "segments", "name", "limit"}
	}
	if d.Kind == SymmetryKindInterval {
		return []string{"fresh", "values", "name", "limit", "divergence", "start"}
	}
	return []string{"fresh", "choices", "values", "name", "limit"}
}

// segments returns a list of Segment objects representing gaps between active values.
// Usage: domain.segments(after=None, before=None)
func (d *SymmetryDomain) segments(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var after, before starlark.Value
	if err := starlark.UnpackArgs("segments", args, kwargs, "after?", &after, "before?", &before); err != nil {
		return nil, err
	}

	// Retrieve Context
	ctxVal := thread.Local(SymmetryContextKey)
	if ctxVal == nil {
		return nil, fmt.Errorf("internal error: symmetry context not found in thread")
	}
	ctx := ctxVal.(*SymmetryContext)
	ctx.EnsureDomainInit(d.Name)

	// Collect and Sort IDs
	var used []int64
	for id := range ctx.Cache[d.Name] {
		used = append(used, id)
	}
	slices.Sort(used)

	var segments []starlark.Value

	// Helper to extract int64 ID from value
	getID := func(v starlark.Value) (int64, bool) {
		if sv, ok := v.(SymmetricValue); ok {
			if sv.prefix == d.Name {
				return sv.id, true
			}
		}
		return 0, false
	}

	var afterID int64
	hasAfter := after != nil && after != starlark.None
	if hasAfter {
		if id, ok := getID(after); ok {
			afterID = id
		} else {
			return nil, fmt.Errorf("segments: 'after' must be a value from domain %s", d.Name)
		}
	}

	var beforeID int64 = ordinalMax
	hasBefore := before != nil && before != starlark.None
	if hasBefore {
		if id, ok := getID(before); ok {
			beforeID = id
		} else {
			return nil, fmt.Errorf("segments: 'before' must be a value from domain %s", d.Name)
		}
	}

	if len(used) == 0 {
		segments = append(segments, &Segment{Domain: d, IsHead: true, IsTail: true})
	} else {
		// Head Segment: (0, used[0])
		if !hasAfter {
			if !hasBefore || used[0] <= beforeID {
				segments = append(segments, &Segment{Domain: d, Right: used[0], IsHead: true})
			}
		}

		// Body Segments: (used[i], used[i+1])
		for i := 0; i < len(used)-1; i++ {
			left, right := used[i], used[i+1]
			isAfter := !hasAfter || left >= afterID
			isBefore := !hasBefore || right <= beforeID

			if isAfter && isBefore {
				segments = append(segments, &Segment{Domain: d, Left: left, Right: right})
			}
		}

		// Tail Segment: (used[last], ordinalMax)
		last := used[len(used)-1]
		if !hasBefore {
			if !hasAfter || last >= afterID {
				segments = append(segments, &Segment{Domain: d, Left: last, IsTail: true})
			}
		}
	}

	return starlark.NewList(segments), nil
}

// Segment represents an interval in an Ordinal Symmetry domain
type Segment struct {
	Domain *SymmetryDomain
	Left   int64
	Right  int64
	IsHead bool
	IsTail bool
}

var _ starlark.Value = (*Segment)(nil)
var _ starlark.HasAttrs = (*Segment)(nil)

func (s *Segment) String() string {
	leftStr := fmt.Sprintf("%d", s.Left)
	if s.IsHead {
		leftStr = "-inf"
	}
	rightStr := fmt.Sprintf("%d", s.Right)
	if s.IsTail {
		rightStr = "+inf"
	}
	return fmt.Sprintf("<segment %s (%s, %s)>", s.Domain.Name, leftStr, rightStr)
}

func (s *Segment) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`{"domain":"%s","left":%d,"right":%d,"is_head":%t,"is_tail":%t}`,
		s.Domain.Name, s.Left, s.Right, s.IsHead, s.IsTail)), nil
}
func (s *Segment) Type() string         { return "symmetry_segment" }
func (s *Segment) Freeze()              {}
func (s *Segment) Truth() starlark.Bool { return starlark.True }
func (s *Segment) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: symmetry_segment")
}

func (s *Segment) Attr(name string) (starlark.Value, error) {
	if name == "fresh" {
		return starlark.NewBuiltin("fresh", s.fresh), nil
	}
	return nil, nil
}

func (s *Segment) AttrNames() []string {
	return []string{"fresh"}
}

func (s *Segment) fresh(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs("fresh", args, kwargs); err != nil {
		return nil, err
	}

	// Check Limit
	// We need to check the domain limit via context
	ctxVal := thread.Local(SymmetryContextKey)
	if ctxVal == nil {
		return nil, fmt.Errorf("internal error: symmetry context not found in thread")
	}
	ctx := ctxVal.(*SymmetryContext)
	ctx.EnsureDomainInit(s.Domain.Name)

	if len(ctx.Cache[s.Domain.Name]) >= s.Domain.Limit {
		return nil, &DisableTransitionError{
			Message: fmt.Sprintf("symmetry limit reached for domain %q (limit %d)", s.Domain.Name, s.Domain.Limit),
		}
	}

	// Calculate new ID using midpoint
	var newID int64
	var err error

	if s.IsHead && s.IsTail {
		// Empty domain: midpoint(0, ordinalMax)
		newID, err = midpoint(0, ordinalMax)
	} else if s.IsHead {
		// (0, Right)
		newID, err = midpoint(0, s.Right)
	} else if s.IsTail {
		// (Left, ordinalMax)
		newID, err = midpoint(s.Left, ordinalMax)
	} else {
		// (Left, Right)
		newID, err = midpoint(s.Left, s.Right)
	}

	if err != nil {
		return nil, &DisableTransitionError{
			Message: fmt.Sprintf("ordinal symmetry collision: %v", err),
		}
	}

	// Check if ID already exists (should not happen in valid segments)
	if ctx.Cache[s.Domain.Name][newID] {
		return nil, fmt.Errorf("internal error: generated ordinal ID %d already exists", newID)
	}

	ctx.Cache[s.Domain.Name][newID] = true

	return NewSymmetricValueWithKind(s.Domain.Name, newID, s.Domain.Kind), nil
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

	// 4. Allocate the next ID
	var nextID int64

	if d.Kind == SymmetryKindNominal {
		// For nominal symmetry, use the smallest unused ID for canonical forms.
		for ctx.Cache[d.Name][nextID] {
			nextID++
		}
	} else if d.Kind == SymmetryKindInterval {
		// Interval: pessimistic allocation (max + 1), starting from d.Start.
		hasValues := false
		var minID, maxID int64
		for id := range ctx.Cache[d.Name] {
			if !hasValues {
				minID, maxID = id, id
				hasValues = true
			} else {
				if id > maxID {
					maxID = id
				}
				if id < minID {
					minID = id
				}
			}
		}

		if !hasValues {
			nextID = int64(d.Start)
		} else {
			nextID = maxID + 1
		}

		// Eager divergence check: ensure (nextID - minID) <= Divergence.
		if d.Divergence > 0 && hasValues {
			if nextID-minID > int64(d.Divergence) {
				return nil, &DisableTransitionError{
					Message: fmt.Sprintf("symmetry divergence reached for domain %q (divergence %d, spread %d)", d.Name, d.Divergence, nextID-minID),
				}
			}
		}

	} else if d.Kind == SymmetryKindOrdinal {
		// For ordinal symmetry, use midpoint-based allocation (tail segment logic).
		// Find current max
		hasValues := false
		var maxID int64
		for id := range ctx.Cache[d.Name] {
			if !hasValues || id > maxID {
				maxID = id
				hasValues = true
			}
		}

		var err error
		if !hasValues {
			// Empty domain: midpoint(0, ordinalMax)
			nextID, err = midpoint(0, ordinalMax)
		} else {
			// Append after max: midpoint(maxID, ordinalMax)
			nextID, err = midpoint(maxID, ordinalMax)
		}
		if err != nil {
			return nil, fmt.Errorf("ordinal symmetry overflow: %v", err)
		}
	}

	// 5. Update Transient Cache
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
	var ids []int64
	for id := range ctx.Cache[d.Name] {
		ids = append(ids, id)
	}
	slices.Sort(ids)

	// Convert to SymmetricValue list
	elems := make([]starlark.Value, len(ids))
	for i, id := range ids {
		elems[i] = NewSymmetricValueWithKind(d.Name, id, d.Kind)
	}

	return starlark.NewList(elems), nil
}

// choices returns a list of all currently active values plus one fresh value (if the limit allows).
// Usage: domain.choices()
func (d *SymmetryDomain) choices(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs("choices", args, kwargs); err != nil {
		return nil, err
	}

	// Attempt to generate a fresh value.
	// We ignore DisableTransitionError (limit reached), but propagate other errors.
	_, err := d.fresh(thread, b, nil, nil)
	if err != nil && !IsDisableTransitionError(err) {
		return nil, err
	}

	// Return all values (which now includes the fresh one if it was generated).
	return d.values(thread, b, nil, nil)
}

// --- Module Construction ---

// SymmetryModule is the 'symmetry' module exposed to Starlark
var SymmetryModule = &starlarkstruct.Module{
	Name: "symmetry",
	Members: starlark.StringDict{
		"nominal":  starlark.NewBuiltin("nominal", makeNominal),
		"ordinal":  starlark.NewBuiltin("ordinal", makeOrdinal),
		"interval": starlark.NewBuiltin("interval", makeInterval),
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

// makeOrdinal creates a new ordinal symmetry domain
// Usage: symmetry.ordinal(name="ts", limit=5)
func makeOrdinal(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	var limit int
	if err := starlark.UnpackArgs("ordinal", args, kwargs, "name", &name, "limit", &limit); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("ordinal: limit must be positive, got %d", limit)
	}
	if name == "" {
		return nil, fmt.Errorf("ordinal: name cannot be empty")
	}
	return &SymmetryDomain{Name: name, Limit: limit, Kind: SymmetryKindOrdinal}, nil
}

// makeInterval creates a new interval symmetry domain
// Usage: symmetry.interval(name="s", divergence=None, limit=None, start=0)
func makeInterval(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	var divergence starlark.Value = starlark.None
	var limit starlark.Value = starlark.None
	var start int = 0

	if err := starlark.UnpackArgs("interval", args, kwargs, "name", &name, "divergence?", &divergence, "limit?", &limit, "start?", &start); err != nil {
		return nil, err
	}

	if name == "" {
		return nil, fmt.Errorf("interval: name cannot be empty")
	}

	divergenceInt := -1
	limitInt := -1

	if divergence != starlark.None {
		if d, err := starlark.AsInt32(divergence); err == nil {
			divergenceInt = d
		} else {
			return nil, fmt.Errorf("interval: divergence must be an integer")
		}
	}

	if limit != starlark.None {
		if l, err := starlark.AsInt32(limit); err == nil {
			limitInt = l
		} else {
			return nil, fmt.Errorf("interval: limit must be an integer")
		}
	}

	// 1. Requirement: At least one of divergence or limit must be provided.
	if divergenceInt == -1 && limitInt == -1 {
		return nil, fmt.Errorf("interval: at least one of divergence or limit must be provided")
	}

	// 2. Derivation
	if divergenceInt != -1 && limitInt == -1 {
		// Only divergence provided: limit = divergence + 1
		limitInt = divergenceInt + 1
	} else if limitInt != -1 && divergenceInt == -1 {
		// Only limit provided: divergence = limit - 1
		divergenceInt = limitInt - 1
	}

	// 3. Validation
	if limitInt > divergenceInt+1 {
		return nil, fmt.Errorf("interval: limit %d cannot fit in divergence %d", limitInt, divergenceInt)
	}
	if limitInt <= 0 {
		return nil, fmt.Errorf("interval: limit must be > 0, got %d", limitInt)
	}
	if divergenceInt < 0 {
		return nil, fmt.Errorf("interval: divergence must be >= 0, got %d", divergenceInt)
	}

	return &SymmetryDomain{
		Name:       name,
		Limit:      limitInt,
		Divergence: divergenceInt,
		Start:      start,
		Kind:       SymmetryKindInterval,
	}, nil
}
