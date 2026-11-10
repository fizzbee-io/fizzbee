package lib

import (
	"encoding/json"
	"fmt"

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
type CardinalityCounter struct {
	value uint8
}

const CardinalityCounterType = "cardinality_counter"

const (
	cardZero uint8 = iota
	cardOne
	cardMany
)

// NewCardinalityCounter returns the abstraction of the concrete count n.
// A negative count is a bug in the caller, so it panics; Starlark input is
// checked by cardinality.counter before it gets here.
func NewCardinalityCounter(n int64) *CardinalityCounter {
	switch {
	case n < 0:
		panic(fmt.Sprintf("cardinality counter: negative count %d", n))
	case n == 0:
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

// Inc returns the counter after adding one.
func (c *CardinalityCounter) Inc() *CardinalityCounter {
	if c.value == cardZero {
		return &CardinalityCounter{value: cardOne}
	}
	return &CardinalityCounter{value: cardMany}
}

// Plus returns the abstract sum of two counts: ZERO is the identity, and
// any sum of two non-ZERO counts is MANY.
func (c *CardinalityCounter) Plus(o *CardinalityCounter) *CardinalityCounter {
	switch {
	case c.value == cardZero:
		return o
	case o.value == cardZero:
		return c
	default:
		return &CardinalityCounter{value: cardMany}
	}
}

// Dec returns every counter reachable by removing one.
func (c *CardinalityCounter) Dec() ([]*CardinalityCounter, error) {
	switch c.value {
	case cardOne:
		return []*CardinalityCounter{{value: cardZero}}, nil
	case cardMany:
		return []*CardinalityCounter{{value: cardOne}, {value: cardMany}}, nil
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
		return starlark.NewBuiltin("inc", func(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 0); err != nil {
				return nil, err
			}
			return c.Inc(), nil
		}), nil
	case "dec":
		return starlark.NewBuiltin("dec", func(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 0); err != nil {
				return nil, err
			}
			out, err := c.Dec()
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
