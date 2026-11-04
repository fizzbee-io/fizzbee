package modelchecker

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"unsafe"

	ast "fizz/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func lazyForkSpec(t *testing.T, name string, lazy bool) *Processor {
	base := "."
	if dir := os.Getenv("RUNFILES_DIR"); dir != "" {
		base = filepath.Join(dir, "_main", "modelchecker")
	}
	file, err := readAstFromFile(filepath.Join(base, "testdata", "lazyfork", name+".json"))
	require.Nil(t, err)
	noDeadlock := false
	stateConfig := &ast.StateSpaceOptions{
		Options:           &ast.Options{MaxActions: 4, MaxConcurrentActions: 2},
		DeadlockDetection: &noDeadlock,
	}
	p := NewProcessor([]*ast.File{file}, stateConfig, false, 0, "", "", false, nil, nil, "")
	p.SetExperimentalLazyFork(lazy)
	return p
}

// fieldDiffs lists the fields of two structs of the same type that differ,
// compared with reflect.DeepEqual field by field, unexported fields included.
// Every field is compared unless it is listed in skip, so a field added
// later is covered without changing this test.
func fieldDiffs(av, bv reflect.Value, skip map[string]bool) []string {
	var diffs []string
	for i := 0; i < av.NumField(); i++ {
		name := av.Type().Field(i).Name
		if skip[name] {
			continue
		}
		if !reflect.DeepEqual(fieldValue(av.Field(i)), fieldValue(bv.Field(i))) {
			diffs = append(diffs, name)
		}
	}
	return diffs
}

func fieldValue(f reflect.Value) any {
	return reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem().Interface()
}

// processDiffs compares a reused clone with a fresh fork. Threads point back
// at their own process, so they are compared one by one without that field.
func processDiffs(a, b *Process) []string {
	// Name is overwritten by materializeLazy right after the reset.
	diffs := fieldDiffs(reflect.ValueOf(a).Elem(), reflect.ValueOf(b).Elem(), map[string]bool{"Name": true, "Threads": true})
	if len(a.Threads) != len(b.Threads) {
		return append(diffs, "Threads (length)")
	}
	for i := range a.Threads {
		ta, tb := a.Threads[i], b.Threads[i]
		if (ta == nil) != (tb == nil) {
			diffs = append(diffs, "Threads (nil slot)")
			continue
		}
		if ta == nil {
			continue
		}
		for _, d := range fieldDiffs(reflect.ValueOf(ta).Elem(), reflect.ValueOf(tb).Elem(), map[string]bool{"Process": true}) {
			diffs = append(diffs, "Threads."+d)
		}
	}
	return diffs
}

// TestLazyReuseMatchesFreshFork checks that a reused clone, once reset, is
// indistinguishable from a fresh fork of the same parent, in every field of
// Process, and that the run finds the same states as without lazy forks.
func TestLazyReuseMatchesFreshFork(t *testing.T) {
	var checked int
	var failures []string
	lazyVerifyReuse = func(reused, fresh *Process) {
		checked++
		if d := processDiffs(reused, fresh); len(d) > 0 {
			failures = append(failures, reused.Name+": "+joinNames(d))
		}
	}
	defer func() { lazyVerifyReuse = nil }()

	p := lazyForkSpec(t, "Reuse", true)
	root, failed, err := p.Start()
	require.Nil(t, err)
	require.NotNil(t, root)
	assert.Nil(t, failed)
	assert.Greater(t, p.lazyReused, 0, "the spec should reuse clones")
	assert.Equal(t, p.lazyReused, checked)
	assert.Empty(t, failures, "reused clones differ from fresh forks in these fields")

	eager := lazyForkSpec(t, "Reuse", false)
	_, _, err = eager.Start()
	require.Nil(t, err)
	assert.Equal(t, len(eager.visited), len(p.visited))
}

func joinNames(names []string) string {
	out := ""
	for i, n := range names {
		if i > 0 {
			out += ", "
		}
		out += n
	}
	return out
}

// TestLazyShadowedBuiltinIsDirty: a state variable named max holds a lambda.
// max(...) in a require is then a call to the lambda, not to the builtin, so
// the statement is not pure and the clone is never reused.
func TestLazyShadowedBuiltinIsDirty(t *testing.T) {
	p := lazyForkSpec(t, "Shadow", true)
	_, failed, err := p.Start()
	require.Nil(t, err)
	assert.Nil(t, failed)
	assert.Equal(t, 0, p.lazyReused)
}

// TestPureSyntaxClassification pins down which expressions count as pure by
// their syntax, and which builtin names they assume.
func TestPureSyntaxClassification(t *testing.T) {
	tests := []struct {
		code  string
		pure  bool
		calls []string
	}{
		{"x", true, nil},
		{"a[b].c + 1 > len(d)", true, []string{"len"}},
		{"[n.val for n in nodes if n.val > 0]", true, nil},
		{"sorted(xs)", true, []string{"sorted"}},
		{"max(len(a), len(b))", true, []string{"max", "len", "len"}},
		{"xs.append(1)", false, nil},
		{"f(x)", false, nil},
		{"lambda x: x", false, nil},
		// sorted, min and max call their key function.
		{"sorted(xs, key=f)", false, nil},
		{"max(xs, key=lambda x: x)", false, nil},
		// The comprehension variable would shadow the builtin it calls.
		{"[len(x) for len in xs]", false, nil},
		{"math.floor(x)", false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			c := classifyExprCode(tt.code)
			assert.Equal(t, tt.pure, c.ok)
			if tt.pure {
				assert.Equal(t, tt.calls, c.calls)
			}
		})
	}
}
