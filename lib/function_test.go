package lib

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
)

// makeFuncs runs src and returns its globals, the functions to encode.
func makeFuncs(t *testing.T, src string) starlark.StringDict {
	globals, err := starlark.ExecFile(&starlark.Thread{}, "f.star", src, nil)
	require.NoError(t, err)
	return globals
}

func encodeHash(v starlark.Value) string {
	e := NewStateEncoder()
	defer e.Release()
	e.WriteValue(v, 0)
	return string(e.Bytes())
}

func TestFunctionEncoding(t *testing.T) {
	g := makeFuncs(t, `
def make(x):
    return lambda v: v + x
inc1 = make(1)
inc1b = make(1)
inc2 = make(2)
double = lambda v: v * 2
withDefault = lambda v, k=3: v + k
`)
	json := func(name string) string {
		b, err := MarshalJSONStarlarkValue(g[name], 0)
		require.NoError(t, err)
		return string(b)
	}
	assert.Equal(t, `{"function":"lambda@3:12","captured":{"x":1}}`, json("inc1"))
	assert.Equal(t, `{"function":"lambda@8:15","defaults":[null,3]}`, json("withDefault"))

	// Same source position and captured values: the same value.
	assert.Equal(t, encodeHash(g["inc1"]), encodeHash(g["inc1b"]))
	// Different captured values or a different lambda: different values.
	assert.NotEqual(t, encodeHash(g["inc1"]), encodeHash(g["inc2"]))
	assert.NotEqual(t, encodeHash(g["inc1"]), encodeHash(g["double"]))
}
