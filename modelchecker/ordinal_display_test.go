package modelchecker

import (
	"strings"
	"testing"

	"github.com/fizzbee-io/fizzbee/lib"
	ast "fizz/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
)

// Ordinal values are stored as sparse IDs. Traces must show them as ranks
// over the whole path: stable labels across steps, in the values' order.
func TestOrdinalDisplayReplacer(t *testing.T) {
	file, err := parseAstFromString(ActionsWithMultipleBlocks)
	require.Nil(t, err)

	ord := func(id int64) *lib.SymmetricValue {
		return lib.NewSymmetricValueWithKind(lib.SymmetryPrefix+"ts", id, lib.SymmetryKindOrdinal)
	}
	nominal := lib.NewSymmetricValueWithKind(lib.SymmetryPrefix+"key", 7, lib.SymmetryKindNominal)
	step := func(values ...starlark.Value) *Link {
		p := NewProcess("", []*ast.File{file}, nil)
		p.Heap.state = starlark.StringDict{
			"events": starlark.NewList(values),
			"k":      nominal,
		}
		return &Link{Node: &Node{Process: p}}
	}
	const a, b, c = int64(2305843009213693951), int64(3458764513820540927), int64(4611686018427387903)
	path := []*Link{
		step(),
		step(ord(b)),
		step(ord(b), ord(c)),
		step(ord(a), ord(c)), // b dropped, a inserted before c
	}

	r := OrdinalDisplayReplacer(path)
	require.NotNil(t, r)
	show := func(l *Link) string {
		return strings.ReplaceAll(r.Replace(l.Node.Heap.ToJson()), lib.SymmetryPrefix, "")
	}
	assert.Equal(t, `{"events":["ts1"],"k":"key7"}`, show(path[1]))
	assert.Equal(t, `{"events":["ts1","ts2"],"k":"key7"}`, show(path[2]))
	assert.Equal(t, `{"events":["ts0","ts2"],"k":"key7"}`, show(path[3]))

	assert.Nil(t, OrdinalDisplayReplacer(path[:1]), "no ordinal values, no replacer")
}
