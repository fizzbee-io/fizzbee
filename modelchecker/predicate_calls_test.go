package modelchecker

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	ast "fizz/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func predicateCallsSpec(t *testing.T, name string) *Processor {
	base := "."
	if dir := os.Getenv("RUNFILES_DIR"); dir != "" {
		base = filepath.Join(dir, "_main", "modelchecker")
	}
	file, err := readAstFromFile(filepath.Join(base, "testdata", "predicate_calls", name+".json"))
	require.Nil(t, err)
	noDeadlock := false
	stateConfig := &ast.StateSpaceOptions{
		Options:           &ast.Options{MaxActions: 10},
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
