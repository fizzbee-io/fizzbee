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

	// SymmetryKindRotational represents values in Z/limit (integers mod limit).
	// Operations: ==, !=, +, - (all mod limit). No ordering since the domain wraps.
	// Reduction: rotate all values by a constant to find the lexicographically smallest set.
	// Use cases: Ring positions, clock arithmetic, hash ring slots.
	SymmetryKindRotational SymmetryKind = "rotational"
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

	// LastAllocated tracks the last allocated value for rotational domains.
	// Map: DomainName -> last allocated ID
	LastAllocated map[string]int64

	// initialized tracks if we have loaded the state from Scanner yet.
	initialized bool
}

// NewSymmetryContext creates a new symmetry context with the given scanner function
func NewSymmetryContext(scanner func() map[string][]int64) *SymmetryContext {
	return &SymmetryContext{
		Scanner:       scanner,
		Cache:         make(map[string]map[int64]bool),
		LastAllocated: make(map[string]int64),
	}
}

// NewSymmetryContextWithLastAllocated creates a new symmetry context with pre-populated LastAllocated state.
func NewSymmetryContextWithLastAllocated(scanner func() map[string][]int64, lastAllocated map[string]int64) *SymmetryContext {
	la := make(map[string]int64, len(lastAllocated))
	for k, v := range lastAllocated {
		la[k] = v
	}
	return &SymmetryContext{
		Scanner:       scanner,
		Cache:         make(map[string]map[int64]bool),
		LastAllocated: la,
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
	Name        string
	Limit       int
	Kind        SymmetryKind
	Divergence  int
	Start       int
	Materialize bool
	Reflection  bool
}

// Starlark Interface for SymmetryDomain
var _ starlark.Value = (*SymmetryDomain)(nil)
var _ starlark.HasAttrs = (*SymmetryDomain)(nil)

func (d *SymmetryDomain) String() string {
	if d.Kind == SymmetryKindInterval {
		if d.Reflection {
			return fmt.Sprintf("symmetry.%s(name=%q, limit=%d, divergence=%d, start=%d, reflection=True)", d.Kind, d.Name, d.Limit, d.Divergence, d.Start)
		}
		return fmt.Sprintf("symmetry.%s(name=%q, limit=%d, divergence=%d, start=%d)", d.Kind, d.Name, d.Limit, d.Divergence, d.Start)
	}
	if d.Kind == SymmetryKindRotational {
		if d.Reflection {
			return fmt.Sprintf("symmetry.%s(name=%q, limit=%d, materialize=%t, reflection=True)", d.Kind, d.Name, d.Limit, d.Materialize)
		}
		return fmt.Sprintf("symmetry.%s(name=%q, limit=%d, materialize=%t)", d.Kind, d.Name, d.Limit, d.Materialize)
	}
	if d.Reflection {
		return fmt.Sprintf("symmetry.%s(name=%q, limit=%d, reflection=True)", d.Kind, d.Name, d.Limit)
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
		if d.Kind == SymmetryKindNominal || d.Kind == SymmetryKindRotational {
			return starlark.NewBuiltin("choices", d.choices), nil
		}
	case "choose":
		if d.Kind == SymmetryKindNominal || d.Kind == SymmetryKindRotational {
			return starlark.NewBuiltin("choose", d.choose), nil
		}
	case "min":
		if d.Kind == SymmetryKindOrdinal || d.Kind == SymmetryKindInterval {
			return starlark.NewBuiltin("min", d.min), nil
		}
	case "max":
		if d.Kind == SymmetryKindOrdinal || d.Kind == SymmetryKindInterval {
			return starlark.NewBuiltin("max", d.max), nil
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
		return []string{"fresh", "values", "segments", "name", "limit", "min", "max"}
	}
	if d.Kind == SymmetryKindInterval {
		return []string{"fresh", "values", "name", "limit", "divergence", "start", "min", "max"}
	}
	if d.Kind == SymmetryKindRotational {
		return []string{"fresh", "choices", "values", "name", "limit", "choose"}
	}
	return []string{"fresh", "choices", "values", "name", "limit", "choose"}
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

// ensureMaterialized populates the cache with all [0..limit-1] values for materialized rotational domains.
func (d *SymmetryDomain) ensureMaterialized(ctx *SymmetryContext) {
	if !d.Materialize || d.Kind != SymmetryKindRotational {
		return
	}
	ctx.EnsureDomainInit(d.Name)
	if len(ctx.Cache[d.Name]) >= d.Limit {
		return
	}
	for i := int64(0); i < int64(d.Limit); i++ {
		ctx.Cache[d.Name][i] = true
	}
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

	} else if d.Kind == SymmetryKindRotational {
		if d.Materialize {
			return nil, fmt.Errorf("fresh() not allowed on materialized rotational domain — all values already exist")
		}
		// For rotational symmetry, find next unused after last-allocated, wrapping mod limit.
		last, hasLast := ctx.LastAllocated[d.Name]
		if !hasLast {
			last = -1
		}
		limit := int64(d.Limit)
		found := false
		for i := int64(0); i < limit; i++ {
			candidate := (last + 1 + i) % limit
			if !ctx.Cache[d.Name][candidate] {
				nextID = candidate
				found = true
				break
			}
		}
		if !found {
			return nil, &DisableTransitionError{
				Message: fmt.Sprintf("symmetry limit reached for domain %q (limit %d)", d.Name, d.Limit),
			}
		}
		ctx.LastAllocated[d.Name] = nextID
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

	if d.Kind == SymmetryKindRotational {
		return NewRotationalSymmetricValue(d.Name, nextID, d.Limit), nil
	}
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
	d.ensureMaterialized(ctx)

	// Collect and Sort IDs for deterministic ordering
	var ids []int64
	for id := range ctx.Cache[d.Name] {
		ids = append(ids, id)
	}
	slices.Sort(ids)

	// Convert to SymmetricValue list
	elems := make([]starlark.Value, len(ids))
	for i, id := range ids {
		if d.Kind == SymmetryKindRotational {
			elems[i] = NewRotationalSymmetricValue(d.Name, id, d.Limit)
		} else {
			elems[i] = NewSymmetricValueWithKind(d.Name, id, d.Kind)
		}
	}

	return starlark.NewList(elems), nil
}

// choices returns a list of all currently active values plus one fresh value (if the limit allows).
// Usage: domain.choices()
func (d *SymmetryDomain) choices(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs("choices", args, kwargs); err != nil {
		return nil, err
	}

	if d.Materialize {
		// Materialized domains have all values already; skip fresh().
		return d.values(thread, b, nil, nil)
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

// choose returns a deterministic default value from the domain.
// Returns the lowest used value, or creates a fresh one if none exist.
// For nominal domains, the specific value is an implementation detail —
// all values are semantically equivalent under symmetry.
// Usage: domain.choose()
func (d *SymmetryDomain) choose(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs("choose", args, kwargs); err != nil {
		return nil, err
	}
	return d.getExtremeOrFresh(thread, b, args, kwargs, false)
}

// min returns the minimum used value, or creates a fresh one if none exist.
// Usage: domain.min()
func (d *SymmetryDomain) min(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs("min", args, kwargs); err != nil {
		return nil, err
	}
	return d.getExtremeOrFresh(thread, b, args, kwargs, false)
}

// max returns the maximum used value, or creates a fresh one if none exist.
// Usage: domain.max()
func (d *SymmetryDomain) max(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs("max", args, kwargs); err != nil {
		return nil, err
	}
	return d.getExtremeOrFresh(thread, b, args, kwargs, true)
}

// getExtremeOrFresh returns the min (or max) used value, or a fresh value if none exist.
func (d *SymmetryDomain) getExtremeOrFresh(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple, findMax bool) (starlark.Value, error) {
	ctxVal := thread.Local(SymmetryContextKey)
	if ctxVal == nil {
		return nil, fmt.Errorf("internal error: symmetry context not found in thread")
	}
	ctx := ctxVal.(*SymmetryContext)
	ctx.EnsureDomainInit(d.Name)
	d.ensureMaterialized(ctx)

	if len(ctx.Cache[d.Name]) == 0 {
		return d.fresh(thread, b, args, kwargs)
	}

	var result int64
	first := true
	for id := range ctx.Cache[d.Name] {
		if first || (findMax && id > result) || (!findMax && id < result) {
			result = id
			first = false
		}
	}
	if d.Kind == SymmetryKindRotational {
		return NewRotationalSymmetricValue(d.Name, result, d.Limit), nil
	}
	return NewSymmetricValueWithKind(d.Name, result, d.Kind), nil
}

// --- Module Construction ---

// SymmetryModule is the 'symmetry' module exposed to Starlark
var SymmetryModule = &starlarkstruct.Module{
	Name: "symmetry",
	Members: starlark.StringDict{
		"nominal":    starlark.NewBuiltin("nominal", makeNominal),
		"ordinal":    starlark.NewBuiltin("ordinal", makeOrdinal),
		"interval":   starlark.NewBuiltin("interval", makeInterval),
		"rotational": starlark.NewBuiltin("rotational", makeRotational),
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
	var reflection bool
	if err := starlark.UnpackArgs("ordinal", args, kwargs, "name", &name, "limit", &limit, "reflection?", &reflection); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("ordinal: limit must be positive, got %d", limit)
	}
	if name == "" {
		return nil, fmt.Errorf("ordinal: name cannot be empty")
	}
	return &SymmetryDomain{Name: name, Limit: limit, Kind: SymmetryKindOrdinal, Reflection: reflection}, nil
}

// makeInterval creates a new interval symmetry domain
// Usage: symmetry.interval(name="s", divergence=None, limit=None, start=0)
func makeInterval(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	var divergence starlark.Value = starlark.None
	var limit starlark.Value = starlark.None
	var start int = 0
	var reflection bool

	if err := starlark.UnpackArgs("interval", args, kwargs, "name", &name, "divergence?", &divergence, "limit?", &limit, "start?", &start, "reflection?", &reflection); err != nil {
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
		Reflection: reflection,
	}, nil
}

// makeRotational creates a new rotational symmetry domain
// Usage: symmetry.rotational(name="pos", limit=5)
func makeRotational(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	var limit int
	var materialize bool
	var reflection bool
	if err := starlark.UnpackArgs("rotational", args, kwargs, "name", &name, "limit", &limit, "materialize?", &materialize, "reflection?", &reflection); err != nil {
		return nil, err
	}
	if limit < 2 {
		return nil, fmt.Errorf("rotational: limit must be >= 2, got %d", limit)
	}
	if name == "" {
		return nil, fmt.Errorf("rotational: name cannot be empty")
	}
	return &SymmetryDomain{Name: name, Limit: limit, Kind: SymmetryKindRotational, Materialize: materialize, Reflection: reflection}, nil
}

// GetCanonicalRotations returns the pivot shift(s) that produce the
// lexicographically smallest signature for the used value set.
// Each returned value is a shift amount: to canonicalize, map each value v to (v + shift) % limit.
func GetCanonicalRotations(usedIDs []int64, limit int) []int64 {
	n := len(usedIDs)
	if n == 0 {
		return []int64{0}
	}

	lim := int64(limit)

	// For each pivot, compute the signature = sorted [(v - pivot) % limit for v in usedIDs]
	// We want the lexicographically smallest signature and return all pivots that produce it.
	type candidate struct {
		pivot     int64
		signature []int64
	}

	var best []candidate

	for _, pivot := range usedIDs {
		sig := make([]int64, n)
		for i, v := range usedIDs {
			sig[i] = ((v - pivot) % lim + lim) % lim
		}
		slices.Sort(sig)

		c := candidate{pivot: pivot, signature: sig}

		if best == nil {
			best = []candidate{c}
		} else {
			cmp := sliceCompare(sig, best[0].signature)
			if cmp < 0 {
				best = []candidate{c}
			} else if cmp == 0 {
				best = append(best, c)
			}
		}
	}

	shifts := make([]int64, len(best))
	for i, c := range best {
		shifts[i] = ((-c.pivot) % lim + lim) % lim
	}
	return shifts
}

// GetCanonicalRotationsWithReflection returns canonical shifts for both clockwise
// and counter-clockwise (reflected) orientations. Each returned RotationCandidate
// includes the shift amount and whether reflection was applied.
// When reflected, each value v is first mapped to (limit - v) % limit before shifting.
func GetCanonicalRotationsWithReflection(usedIDs []int64, limit int) []RotationCandidate {
	lim := int64(limit)
	n := len(usedIDs)
	if n == 0 {
		return []RotationCandidate{{Shift: 0, Reflected: false}}
	}

	// CW candidates (no reflection)
	cwShifts := GetCanonicalRotations(usedIDs, limit)
	cwSig := applyRotationSignature(usedIDs, cwShifts[0], lim)

	// CCW candidates (reflection: v -> (limit - v) % limit)
	reflected := make([]int64, n)
	for i, v := range usedIDs {
		reflected[i] = (lim - v) % lim
	}
	ccwShifts := GetCanonicalRotations(reflected, limit)
	ccwSig := applyRotationSignature(reflected, ccwShifts[0], lim)

	cmp := sliceCompare(cwSig, ccwSig)
	if cmp < 0 {
		// CW wins
		result := make([]RotationCandidate, len(cwShifts))
		for i, s := range cwShifts {
			result[i] = RotationCandidate{Shift: s, Reflected: false}
		}
		return result
	} else if cmp > 0 {
		// CCW wins
		result := make([]RotationCandidate, len(ccwShifts))
		for i, s := range ccwShifts {
			result[i] = RotationCandidate{Shift: s, Reflected: true}
		}
		return result
	}
	// Tied: return both
	result := make([]RotationCandidate, 0, len(cwShifts)+len(ccwShifts))
	for _, s := range cwShifts {
		result = append(result, RotationCandidate{Shift: s, Reflected: false})
	}
	for _, s := range ccwShifts {
		result = append(result, RotationCandidate{Shift: s, Reflected: true})
	}
	return result
}

// RotationCandidate represents a canonical rotation shift, possibly with reflection.
type RotationCandidate struct {
	Shift     int64
	Reflected bool
}

// applyRotationSignature returns the sorted signature for a given shift.
func applyRotationSignature(ids []int64, shift, lim int64) []int64 {
	sig := make([]int64, len(ids))
	for i, v := range ids {
		sig[i] = ((v + shift) % lim + lim) % lim
	}
	slices.Sort(sig)
	return sig
}

// GetOrdinalReflectionCandidates returns candidate mappings for ordinal with reflection.
// Forward: squeeze to 0..N-1 (identity rank). Reverse: map v → (limit-1-v), then squeeze.
// Returns forward candidates, reverse candidates. If one produces a smaller signature,
// only that one is returned. If tied, both are returned.
// Each candidate is a mapping from sorted used IDs to canonical values.
func GetOrdinalReflectionCandidates(usedIDs []int64, limit int) (forward []int64, reverse []int64, tied bool) {
	n := len(usedIDs)
	if n == 0 {
		return nil, nil, false
	}

	// Forward: squeeze to 0..N-1 (already sorted)
	fwd := make([]int64, n)
	for i := range usedIDs {
		fwd[i] = int64(i)
	}

	// Reverse: map each v → (limit-1 - rank), where rank is the index in sorted order
	// This means the highest-ranked value becomes 0, etc.
	rev := make([]int64, n)
	for i := range usedIDs {
		rev[i] = int64(n - 1 - i)
	}

	// Compare signatures (both are already sorted ascending for forward,
	// but reverse needs to be sorted to compare)
	fwdSig := make([]int64, n)
	copy(fwdSig, fwd)
	revSig := make([]int64, n)
	copy(revSig, rev)
	slices.Sort(fwdSig)
	slices.Sort(revSig)

	// Both signatures are identical: [0, 1, 2, ..., N-1]
	// So they're always tied for ordinal. The tie-breaking happens at the full state level.
	return fwd, rev, true
}

// GetIntervalReflectionCandidates returns candidate mappings for interval with reflection.
// Forward: v - min. Reverse: max - v.
// Returns forward mapped values, reverse mapped values, and whether they're tied.
// usedIDs must be sorted.
func GetIntervalReflectionCandidates(usedIDs []int64) (forward []int64, reverse []int64, tied bool) {
	n := len(usedIDs)
	if n == 0 {
		return nil, nil, false
	}

	minID := usedIDs[0]
	maxID := usedIDs[n-1]

	fwd := make([]int64, n)
	rev := make([]int64, n)
	for i, v := range usedIDs {
		fwd[i] = v - minID
		rev[i] = maxID - v
	}

	// Sort for comparison
	fwdSorted := make([]int64, n)
	copy(fwdSorted, fwd)
	slices.Sort(fwdSorted)
	revSorted := make([]int64, n)
	copy(revSorted, rev)
	slices.Sort(revSorted)

	cmp := sliceCompare(fwdSorted, revSorted)
	if cmp < 0 {
		return fwd, nil, false
	} else if cmp > 0 {
		return nil, rev, false
	}
	return fwd, rev, true
}

// sliceCompare compares two int64 slices lexicographically.
func sliceCompare(a, b []int64) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return 0
}
