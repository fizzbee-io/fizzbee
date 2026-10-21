package modelchecker

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"testing"

	ast "fizz/proto"
	"github.com/fizzbee-io/fizzbee/lib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
)

func predicateCallsSpec(t *testing.T, name string) *Processor {
	base := "."
	if dir := os.Getenv("RUNFILES_DIR"); dir != "" {
		base = filepath.Join(dir, "_main", "modelchecker")
	}
	file, err := readAstFromFile(filepath.Join(base, "testdata", "predicate_calls", name+".json"))
	require.Nil(t, err)
	noDeadlock, noCrash := false, false
	stateConfig := &ast.StateSpaceOptions{
		Options:           &ast.Options{MaxActions: 10, MaxConcurrentActions: 1, CrashOnYield: &noCrash},
		DeadlockDetection: &noDeadlock,
	}
	return NewProcessor([]*ast.File{file}, stateConfig, false, 0, "", "", false, nil, nil, "")
}

// TestPredicateCalls checks fizz funcs called inside assertion expressions:
// role methods, top-level funcs, defaults, nesting, short-circuiting,
// comprehensions and isolation between calls. Every assertion holds.
func TestPredicateCalls(t *testing.T) {
	p := predicateCallsSpec(t, "Calls")
	root, failed, err := p.Start()
	require.Nil(t, err)
	require.NotNil(t, root)
	assert.Nil(t, failed)
}

// TestPredicateCallErrors checks that a func that can't run as a predicate
// fails with an error naming the func and the reason.
func TestPredicateCallErrors(t *testing.T) {
	saved := maxPredicateSteps
	maxPredicateSteps = 1000
	defer func() { maxPredicateSteps = saved }()

	for spec, want := range map[string]string{
		"Oneof":       "helper: a fizz func called in an expression must be deterministic",
		"Require":     "helper: a require statement failed",
		"EndlessLoop": "assertion ran more than 1000 statements",
		"DivByZero":   "helper: Line 3: Error evaluating expr: 1 / 0: floating-point division by zero",
	} {
		t.Run(spec, func(t *testing.T) {
			p := predicateCallsSpec(t, spec)
			msg := func() (msg string) {
				defer func() {
					if r := recover(); r != nil {
						msg = fmt.Sprint(r)
					}
				}()
				p.Start()
				return ""
			}()
			assert.Contains(t, msg, want)
		})
	}
}

// TestPredicateCallBinding checks argument binding for calls in expressions,
// and that a failed call leaves the calling process unchanged, including
// builtins such as next_states that are set aside while cloning.
func TestPredicateCallBinding(t *testing.T) {
	p := predicateCallsSpec(t, "Calls")
	root, failed, err := p.Start()
	require.Nil(t, err)
	require.Nil(t, failed)
	caller := root.Process.CloneForAssert(nil, 0)
	caller.enablePredicateCalls()
	probe := starlark.NewBuiltin("next_states", func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
		return starlark.None, nil
	})
	caller.Heap.state["next_states"] = probe
	before := maps.Clone(caller.Heap.state)
	counter := caller.Heap.state["counters"].(*starlark.List).Index(1).(*lib.Role)

	kw := func(k string, v int) starlark.Tuple { return starlark.Tuple{starlark.String(k), starlark.MakeInt(v)} }
	one, two := starlark.MakeInt(1), starlark.MakeInt(2)
	for name, tc := range map[string]struct {
		recv    *lib.Role
		fn      string
		args    starlark.Tuple
		kwargs  []starlark.Tuple
		want    string
		wantErr string
	}{
		"Positional":        {nil, "ident", starlark.Tuple{one}, nil, "1", ""},
		"Keyword":           {nil, "ident", nil, []starlark.Tuple{kw("i", 2)}, "2", ""},
		"Default":           {counter, "plus", starlark.Tuple{one}, nil, "1", ""},
		"DefaultOverridden": {counter, "plus", starlark.Tuple{one}, []starlark.Tuple{kw("m", 2)}, "3", ""},
		"TooMany":           {nil, "ident", starlark.Tuple{one, two}, nil, "", "got 2 arguments, want at most 1"},
		"Missing":           {nil, "ident", nil, nil, "", "missing argument i"},
		"UnknownKeyword":    {nil, "ident", nil, []starlark.Tuple{kw("j", 1)}, "", "unexpected keyword argument j"},
		"Duplicate":         {nil, "ident", starlark.Tuple{one}, []starlark.Tuple{kw("i", 1)}, "", "got multiple values for argument i"},
		"NotAFunc":          {nil, "nope", nil, nil, "", "nope is not a fizz func"},
		"Fails":             {nil, "boom", nil, nil, "", "division by zero"},
	} {
		t.Run(name, func(t *testing.T) {
			v, err := caller.callFizzFunc(nil, tc.recv, tc.fn, tc.args, tc.kwargs)
			if tc.wantErr != "" {
				require.NotNil(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			} else {
				require.Nil(t, err)
				assert.Equal(t, tc.want, v.String())
			}
			assert.Equal(t, len(before), len(caller.Heap.state))
			for k, v := range before {
				assert.True(t, v == caller.Heap.state[k], "heap variable %s changed", k)
			}
		})
	}
}
