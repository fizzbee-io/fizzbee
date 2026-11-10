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
