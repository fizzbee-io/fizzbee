package modelchecker

import (
	"bytes"
	"testing"

	ast "fizz/proto"
	"github.com/fizzbee-io/fizzbee/lib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
)

const (
	ordA = int64(2305843009213693951)
	ordB = int64(3458764513820540927)
	ordC = int64(4611686018427387903)
)

func ordinal(name string, id int64) *lib.SymmetricValue {
	return lib.NewSymmetricValueWithKind(name, id, lib.SymmetryKindOrdinal)
}

func nominal(name string, id int64) *lib.SymmetricValue {
	return lib.NewSymmetricValueWithKind(name, id, lib.SymmetryKindNominal)
}

// traceStep builds one failure-path link whose heap holds state.
func traceStep(t *testing.T, name string, state starlark.StringDict) *Link {
	file, err := parseAstFromString(ActionsWithMultipleBlocks)
	require.Nil(t, err)
	p := NewProcess("", []*ast.File{file}, nil)
	p.Heap.state = state
	return &Link{Name: name, Node: &Node{Process: p}}
}

func list(values ...starlark.Value) *starlark.List { return starlark.NewList(values) }

func printTrace(path []*Link) string {
	var out bytes.Buffer
	WriteFailurePathStates(&out, path)
	return out.String()
}

// Ordinal values are stored as sparse IDs. Traces must show them as ranks
// over the whole path: stable labels across steps, in the values' order.
func TestFailureTraceOrdinalRanksAcrossPath(t *testing.T) {
	path := []*Link{
		traceStep(t, "Init", starlark.StringDict{"events": list()}),
		traceStep(t, "A", starlark.StringDict{"events": list(ordinal("ts", ordB))}),
		traceStep(t, "B", starlark.StringDict{"events": list(ordinal("ts", ordB), ordinal("ts", ordC))}),
		// ordB dropped, ordA inserted before ordC
		traceStep(t, "C", starlark.StringDict{"events": list(ordinal("ts", ordA), ordinal("ts", ordC))}),
	}
	assert.Equal(t, `------
Init
--
state: {"events":[]}
------
A
--
state: {"events":["ts1"]}
------
B
--
state: {"events":["ts1","ts2"]}
------
C
--
state: {"events":["ts0","ts2"]}
------
`, printTrace(path))
}

// Only the ordinal values themselves change. A plain string with the same
// text as an ordinal value, a nominal value of another domain, and a value
// of another domain whose text starts with an ordinal's text all print as
// they are.
func TestFailureTraceLeavesOtherValuesAlone(t *testing.T) {
	path := []*Link{traceStep(t, "S", starlark.StringDict{
		"o":         ordinal("ts", ordA),
		"plain":     starlark.String("ts2305843009213693951"),
		"nominal":   nominal("key", 7),
		"other":     ordinal("tsx", ordB),
		"lookalike": nominal("ts2", 3058430092136939510),
	})}
	got := printTrace(path)
	assert.Contains(t, got, `"o":"ts0"`)
	assert.Contains(t, got, `"other":"tsx0"`)
	assert.Contains(t, got, `"plain":"ts2305843009213693951"`, "plain strings are never rewritten")
	assert.Contains(t, got, `"nominal":"key7"`)
	assert.Contains(t, got, `"lookalike":"ts23058430092136939510"`)
}

// The state, the returns and the role fields are all printed with ranks, and
// the original state is not modified.
func TestFailureTraceRanksInReturnsAndRoleFields(t *testing.T) {
	latest := ordinal("e", ordC)
	step := traceStep(t, "Worker#0.Run", starlark.StringDict{"latest": latest})
	step.Node.Returns = starlark.StringDict{"got": ordinal("e", ordB)}
	step.Node.Roles = []*lib.Role{{
		ID:     lib.NewModelValue("Worker", 0),
		Name:   "Worker",
		Params: lib.FromStringDict(starlark.String("params"), starlark.StringDict{}),
		Fields: lib.FromStringDict(starlark.String("fields"), starlark.StringDict{
			"epoch": ordinal("e", ordA),
			"note":  starlark.String("e2305843009213693951"),
		}),
	}}
	assert.Equal(t, `------
Worker#0.Run
--
state: {"latest":"e2"}
returns: {got: e1}
Worker#0: fields(epoch = e0, note = "e2305843009213693951")
------
`, printTrace([]*Link{step}))
	assert.Same(t, latest, step.Node.Heap.state["latest"])
	assert.Equal(t, ordC, latest.GetId())
}

func TestFailureTraceWithoutOrdinals(t *testing.T) {
	path := []*Link{traceStep(t, "S", starlark.StringDict{"k": nominal("key", 3), "s": starlark.String("x")})}
	assert.Equal(t, "------\nS\n--\nstate: {\"k\":\"key3\",\"s\":\"x\"}\n------\n", printTrace(path))
}

// Symmetric values compare by domain name and ID, not kind. A nominal value
// whose ID equals a rank must not merge with a ranked ordinal value of the
// same domain name, so a domain name shared across kinds keeps its raw IDs.
func TestFailureTraceDomainNameSharedAcrossKinds(t *testing.T) {
	set := starlark.NewSet(2)
	require.NoError(t, set.Insert(ordinal("ts", ordA)))
	require.NoError(t, set.Insert(nominal("ts", 0)))
	path := []*Link{traceStep(t, "S", starlark.StringDict{
		"both":  set,
		"other": ordinal("e", ordB),
	})}
	got := printTrace(path)
	assert.Contains(t, got, `"both":["ts0","ts2305843009213693951"]`, "both values, raw IDs")
	assert.Contains(t, got, `"other":"e0"`, "other ordinal domains are still ranked")
	assert.Equal(t, 2, set.Len(), "the original set is unchanged")
}

// Nil links are skipped rather than dereferenced.
func TestFailureTraceSkipsNilLinks(t *testing.T) {
	path := []*Link{nil, {Name: "no-node"}, traceStep(t, "S", starlark.StringDict{"o": ordinal("ts", ordA)})}
	assert.Equal(t, "------\nS\n--\nstate: {\"o\":\"ts0\"}\n------\n", printTrace(path))
}
