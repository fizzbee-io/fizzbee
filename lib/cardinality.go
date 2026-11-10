package lib

import (
	"encoding/json"
	"fmt"
	"sync/atomic"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/syntax"
)

// CardinalityCounter is the abstract counter of the cardinality package. It
// has three values, each standing for a set of concrete counts:
//
//	ZERO = {0}, ONE = {1}, MANY = {2, 3, ...}
//
// It is immutable: inc() returns a new counter, dec() returns the list of
// counters the decrement can reach (ONE -> [ZERO], MANY -> [ONE, MANY]) for
// the spec to choose from with oneof.
//
// Besides the value, a counter carries scratch fields that the liveness
// tracking uses within one step (see modelchecker/cardinality.go).
// They never affect equality, hashing, serialization or symmetry.
type CardinalityCounter struct {
	value uint8

	// Scratch for liveness tracking, valid only while epoch is the epoch
	// of the running step: the counter of the step's start state this one
	// was derived from with inc() and dec(), and the net change since.
	// The epoch makes the scratch go stale when the step ends: a counter
	// kept in a local variable across a yield is never seen by the
	// tracker, so its scratch can't be cleared any other way.
	epoch uint64
	base  *CardinalityCounter
	delta int
}

const CardinalityCounterType = "cardinality_counter"

const (
	cardZero uint8 = iota
	cardOne
	cardMany
)

var cardinalityUsed atomic.Bool

// CardinalityUsed reports whether the spec created any counter, so the
// checker only pays for location tracking when it is needed.
func CardinalityUsed() bool { return cardinalityUsed.Load() }

// ResetCardinalityUsed clears CardinalityUsed for a new model-checking run.
func ResetCardinalityUsed() { cardinalityUsed.Store(false) }

// NewCardinalityCounter returns the abstraction of the concrete count n.
func NewCardinalityCounter(n int64) *CardinalityCounter {
	cardinalityUsed.Store(true)
	switch {
	case n <= 0:
		return &CardinalityCounter{value: cardZero}
	case n == 1:
		return &CardinalityCounter{value: cardOne}
	default:
		return &CardinalityCounter{value: cardMany}
	}
}

// IsZero etc. report the abstract value.
func (c *CardinalityCounter) IsZero() bool { return c.value == cardZero }
func (c *CardinalityCounter) IsOne() bool  { return c.value == cardOne }
func (c *CardinalityCounter) IsMany() bool { return c.value == cardMany }

// CardinalityStepKey is the starlark thread local holding the running
// step's *CardinalityStep.
const CardinalityStepKey = "fizz_cardinality_step"

// CardinalityStep is what inc() and dec() need to know about the running
// step (from one yield point to the next): an epoch unique to the step, and
// the counters of the step's start state, each mapped to the location it
// was found at ("" when it was found at more than one).
type CardinalityStep struct {
	Epoch uint64
	Start map[*CardinalityCounter]string
}

var cardinalityEpochs atomic.Uint64

// NewCardinalityStep returns a step with a fresh epoch.
func NewCardinalityStep(start map[*CardinalityCounter]string) *CardinalityStep {
	return &CardinalityStep{Epoch: cardinalityEpochs.Add(1), Start: start}
}

// Lineage returns the start-state counter c was derived from in step and
// the net change since, or ok=false when c has no known lineage in step: a
// new counter, or one carried over from an earlier step.
func (c *CardinalityCounter) Lineage(step *CardinalityStep) (base *CardinalityCounter, delta int, ok bool) {
	if step == nil || c.epoch != step.Epoch || c.base == nil {
		return nil, 0, false
	}
	return c.base, c.delta, true
}

// derive returns a counter with the given value that changed c by d.
func (c *CardinalityCounter) derive(step *CardinalityStep, value uint8, d int) *CardinalityCounter {
	out := &CardinalityCounter{value: value}
	if step == nil {
		return out
	}
	out.epoch = step.Epoch
	if _, ok := step.Start[c]; ok {
		out.base, out.delta = c, d
	} else if c.epoch == step.Epoch && c.base != nil {
		out.base, out.delta = c.base, c.delta+d
	}
	return out
}

// Inc returns the counter after adding one.
func (c *CardinalityCounter) Inc(step *CardinalityStep) *CardinalityCounter {
	if c.value == cardZero {
		return c.derive(step, cardOne, 1)
	}
	return c.derive(step, cardMany, 1)
}

// Dec returns every counter reachable by removing one.
func (c *CardinalityCounter) Dec(step *CardinalityStep) ([]*CardinalityCounter, error) {
	switch c.value {
	case cardOne:
		return []*CardinalityCounter{c.derive(step, cardZero, -1)}, nil
	case cardMany:
		return []*CardinalityCounter{c.derive(step, cardOne, -1), c.derive(step, cardMany, -1)}, nil
	default:
		return nil, fmt.Errorf("cardinality counter: dec() on 0")
	}
}

func (c *CardinalityCounter) String() string {
	switch c.value {
	case cardZero:
		return "0"
	case cardOne:
		return "1"
	default:
		return "MANY"
	}
}

// ValueCode is the abstract value as a small int, for encoders.
func (c *CardinalityCounter) ValueCode() uint8 { return c.value }

// CardinalityCounterFromCode is the inverse of ValueCode.
func CardinalityCounterFromCode(code uint8) (*CardinalityCounter, error) {
	if code > cardMany {
		return nil, fmt.Errorf("bad cardinality counter code %d", code)
	}
	cardinalityUsed.Store(true)
	return &CardinalityCounter{value: code}, nil
}

func (c *CardinalityCounter) Type() string { return CardinalityCounterType }
func (c *CardinalityCounter) Freeze()      {}

// Truth is "not zero", which is exact.
func (c *CardinalityCounter) Truth() starlark.Bool { return c.value != cardZero }

func (c *CardinalityCounter) Hash() (uint32, error) {
	return 0x9e3779b1 ^ uint32(c.value), nil
}

func (c *CardinalityCounter) CompareSameType(op syntax.Token, y_ starlark.Value, depth int) (bool, error) {
	y := y_.(*CardinalityCounter)
	switch op {
	case syntax.EQL:
		return c.value == y.value, nil
	case syntax.NEQ:
		return c.value != y.value, nil
	default:
		return false, fmt.Errorf("cardinality counters can't be ordered with %s; use is_zero(), is_one() or is_many()", op)
	}
}

// Binary rejects arithmetic with a hint at the methods.
func (c *CardinalityCounter) Binary(op syntax.Token, y starlark.Value, side starlark.Side) (starlark.Value, error) {
	return nil, fmt.Errorf("cardinality counters don't support %s; use inc() or oneof c.dec()", op)
}

func (c *CardinalityCounter) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.String())
}

func (c *CardinalityCounter) Attr(name string) (starlark.Value, error) {
	switch name {
	case "inc":
		return starlark.NewBuiltin("inc", func(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 0); err != nil {
				return nil, err
			}
			return c.Inc(threadCardinalityStep(t)), nil
		}), nil
	case "dec":
		return starlark.NewBuiltin("dec", func(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 0); err != nil {
				return nil, err
			}
			out, err := c.Dec(threadCardinalityStep(t))
			if err != nil {
				return nil, err
			}
			vals := make([]starlark.Value, len(out))
			for i, o := range out {
				vals[i] = o
			}
			return starlark.NewList(vals), nil
		}), nil
	case "is_zero", "is_one", "is_many":
		return starlark.NewBuiltin(name, func(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 0); err != nil {
				return nil, err
			}
			switch name {
			case "is_zero":
				return starlark.Bool(c.IsZero()), nil
			case "is_one":
				return starlark.Bool(c.IsOne()), nil
			default:
				return starlark.Bool(c.IsMany()), nil
			}
		}), nil
	}
	return nil, nil
}

func (c *CardinalityCounter) AttrNames() []string {
	return []string{"dec", "inc", "is_many", "is_one", "is_zero"}
}

func threadCardinalityStep(t *starlark.Thread) *CardinalityStep {
	if t == nil {
		return nil
	}
	step, _ := t.Local(CardinalityStepKey).(*CardinalityStep)
	return step
}

var _ starlark.Value = (*CardinalityCounter)(nil)
var _ starlark.Comparable = (*CardinalityCounter)(nil)
var _ starlark.HasAttrs = (*CardinalityCounter)(nil)
var _ starlark.HasBinary = (*CardinalityCounter)(nil)

// CardinalityModule is the 'cardinality' module exposed to Starlark.
var CardinalityModule = &starlarkstruct.Module{
	Name: "cardinality",
	Members: starlark.StringDict{
		"counter": starlark.NewBuiltin("counter", makeCardinalityCounter),
	},
}

// makeCardinalityCounter: cardinality.counter(n=0)
func makeCardinalityCounter(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var n int64
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "n?", &n); err != nil {
		return nil, err
	}
	if n < 0 {
		return nil, fmt.Errorf("cardinality.counter: n must be non-negative, got %d", n)
	}
	return NewCardinalityCounter(n), nil
}

// CloneCardinalityCounter returns a new object with c's value and none of
// its scratch, so two locations never share one counter object.
func CloneCardinalityCounter(c *CardinalityCounter) *CardinalityCounter {
	return &CardinalityCounter{value: c.value}
}
