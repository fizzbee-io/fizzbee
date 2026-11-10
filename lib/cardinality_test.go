package lib

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

// abstractOf is the abstraction of a concrete count.
func abstractOf(n int) string {
	switch n {
	case 0:
		return "0"
	case 1:
		return "1"
	}
	return "MANY"
}

// Every operation must return exactly the abstractions of the concrete
// results, over all concrete counts the input stands for (checked up to 10).
func TestCardinalityCounterMatchesConcrete(t *testing.T) {
	for n := 0; n <= 10; n++ {
		c := NewCardinalityCounter(int64(n))
		require.Equal(t, abstractOf(n), c.String(), "counter(%d)", n)

		assert.Equal(t, abstractOf(n+1), c.Inc().String(), "inc of %d", n)
		assert.Equal(t, n == 0, c.IsZero())
		assert.Equal(t, n == 1, c.IsOne())
		assert.Equal(t, n >= 2, c.IsMany())
		assert.Equal(t, starlark.Bool(n != 0), c.Truth())

		decs, err := c.Dec()
		if n == 0 {
			assert.Error(t, err)
			continue
		}
		require.NoError(t, err)
		got := map[string]bool{}
		for _, d := range decs {
			got[d.String()] = true
		}
		// What the decrement must offer: the abstraction of m-1 for every
		// concrete m with the same abstraction as n.
		want := map[string]bool{}
		for m := 1; m <= 12; m++ {
			if abstractOf(m) == abstractOf(n) {
				want[abstractOf(m-1)] = true
			}
		}
		assert.Equal(t, want, got, "dec of %s", c)
	}
}

func TestCardinalityCounterEqualityAndHash(t *testing.T) {
	values := []*CardinalityCounter{NewCardinalityCounter(0), NewCardinalityCounter(1), NewCardinalityCounter(2), NewCardinalityCounter(7)}
	for _, x := range values {
		for _, y := range values {
			eq, err := starlark.Equal(x, y)
			require.NoError(t, err)
			assert.Equal(t, x.String() == y.String(), eq, "%s == %s", x, y)
			hx, _ := x.Hash()
			hy, _ := y.Hash()
			if eq {
				assert.Equal(t, hx, hy)
				assert.Equal(t, encode(x), encode(y))
			} else {
				assert.NotEqual(t, encode(x), encode(y))
			}
		}
	}
	// Ordering and arithmetic are errors, not silent results.
	_, err := starlark.Compare(syntax.LT, values[0], values[1])
	assert.Error(t, err)
	_, err = starlark.Binary(syntax.PLUS, values[0], starlark.MakeInt(1))
	assert.Error(t, err)
	// Equality with an int is False, never an error: Starlark decides it.
	eq, err := starlark.Equal(values[1], starlark.MakeInt(1))
	require.NoError(t, err)
	assert.False(t, eq)
}

// A counter never hashes like the int or string it prints as.
func TestCardinalityCounterHashIsTypeTagged(t *testing.T) {
	assert.NotEqual(t, encode(NewCardinalityCounter(1)), encode(starlark.MakeInt(1)))
	assert.NotEqual(t, encode(NewCardinalityCounter(1)), encode(starlark.String("1")))
	assert.NotEqual(t, encode(NewCardinalityCounter(5)), encode(starlark.String("MANY")))
}

func TestCardinalityCounterCodecRoundTrip(t *testing.T) {
	ctx := NewCodecContext()
	in := []starlark.Value{
		NewCardinalityCounter(0), NewCardinalityCounter(1), NewCardinalityCounter(3),
		starlark.NewList([]starlark.Value{NewCardinalityCounter(1), starlark.MakeInt(1)}),
	}
	out, _ := roundTripValues(t, ctx, in...)
	for i := range in {
		eq, err := starlark.Equal(in[i], out[i])
		require.NoError(t, err)
		assert.True(t, eq, fmt.Sprint(in[i]))
	}
	assert.Equal(t, CardinalityCounterType, out[0].Type())
}

// Counters nested in lists and dicts still hash apart from the strings and
// ints they print as. JSON shows them as strings, for display only.
func TestCardinalityCounterNestedHashIsTypeTagged(t *testing.T) {
	list := func(v starlark.Value) starlark.Value { return starlark.NewList([]starlark.Value{v}) }
	dict := func(k, v starlark.Value) starlark.Value {
		d := starlark.NewDict(1)
		require.NoError(t, d.SetKey(k, v))
		return d
	}
	many, one := NewCardinalityCounter(5), NewCardinalityCounter(1)
	assert.NotEqual(t, encode(list(many)), encode(list(starlark.String("MANY"))))
	assert.NotEqual(t, encode(list(one)), encode(list(starlark.MakeInt(1))))
	assert.NotEqual(t, encode(dict(starlark.String("k"), many)), encode(dict(starlark.String("k"), starlark.String("MANY"))))
	assert.NotEqual(t, encode(dict(one, starlark.None)), encode(dict(starlark.MakeInt(1), starlark.None)))
	assert.Equal(t, encode(list(NewCardinalityCounter(2))), encode(list(NewCardinalityCounter(9))))

	assert.Equal(t, `["MANY"]`, string(mustJSON(t, list(many))))
}

func TestNewCardinalityCounterRejectsNegative(t *testing.T) {
	assert.Panics(t, func() { NewCardinalityCounter(-1) })
}

// The API as specs use it, through Starlark.
func TestCardinalityCounterStarlarkAPI(t *testing.T) {
	run := func(src string) error {
		thread := &starlark.Thread{Name: "test"}
		_, err := starlark.ExecFileOptions(&syntax.FileOptions{GlobalReassign: true}, thread, "test.star", src,
			starlark.StringDict{"cardinality": CardinalityModule})
		return err
	}
	require.NoError(t, run(`
def check(cond, msg):
    if not cond:
        fail(msg)

c = cardinality.counter()
check(c.is_zero(), "default is zero")
check(not c, "zero is falsy")
c = c.inc()
check(c.is_one(), "inc of 0")
c = c.inc()
check(c.is_many(), "inc of 1")
check(bool(c), "MANY is truthy")
choices = c.dec()
check(len(choices) == 2, "dec of MANY has two results")
check(choices[0].is_one() and choices[1].is_many(), "dec of MANY is [1, MANY]")
check(len(cardinality.counter(1).dec()) == 1, "dec of 1 has one result")
check(cardinality.counter(1).dec()[0].is_zero(), "dec of 1 is 0")
check(cardinality.counter(5) == cardinality.counter(2), "MANY equals MANY")
check(cardinality.counter(1) != cardinality.counter(2), "1 is not MANY")
check(not (cardinality.counter(2) == 2), "never equal to an int")
check({cardinality.counter(3): "x"}[cardinality.counter(7)] == "x", "equal counters are the same dict key")
check(str(cardinality.counter(4)) == "MANY", "str")
`))

	for _, tc := range []struct{ src, err string }{
		{`cardinality.counter(-1)`, "must be non-negative"},
		{`cardinality.counter().dec()`, "dec() on 0"},
		{`cardinality.counter().inc(1)`, "inc: got 1 arguments, want 0"},
		{`cardinality.counter(1).dec(1)`, "dec: got 1 arguments, want 0"},
		{`cardinality.counter().is_zero(1)`, "is_zero: got 1 arguments, want 0"},
		{`cardinality.counter(1) + 1`, "use inc() or oneof c.dec()"},
		{`cardinality.counter(1) < cardinality.counter(2)`, "use is_zero(), is_one() or is_many()"},
		{`cardinality.counter(1) < 2`, "cardinality_counter < int not implemented"},
		{`int(cardinality.counter(1))`, "cannot convert cardinality_counter to int"},
	} {
		err := run("x = " + tc.src)
		if assert.Error(t, err, tc.src) {
			assert.Contains(t, err.Error(), tc.err, tc.src)
		}
	}
}

// Plus is the abstraction of adding two concrete counts.
func TestCardinalityCounterPlus(t *testing.T) {
	for a := int64(0); a <= 3; a++ {
		for b := int64(0); b <= 3; b++ {
			got := NewCardinalityCounter(a).Plus(NewCardinalityCounter(b))
			assert.Equal(t, NewCardinalityCounter(a+b).String(), got.String(), "%d + %d", a, b)
		}
	}
}
