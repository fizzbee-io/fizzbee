package modelchecker

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	ast "fizz/proto"
	"github.com/fizzbee-io/fizzbee/lib"
	"github.com/fizzbee-io/fizzbee/modelchecker/liveness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
)

// checkFormulaSpec checks each assertion of testdata/formulas/<name> on its
// own: the ones named Pass... hold, the ones named Fail... don't, except
// those in holds, and every failure comes with a counterexample that starts
// at Init. For the classic shapes, the direct check must agree. It returns
// the file, with its for headers expanded.
func checkFormulaSpec(t *testing.T, name string, holds ...string) *ast.File {
	file, _, _ := checkFormulaSpecWith(t, name, false, holds...)
	return file
}

// checkFormulaSpecWith is checkFormulaSpec with crashes on yield set as
// given. It also returns the explored nodes and each assertion's notes.
func checkFormulaSpecWith(t *testing.T, name string, crashOnYield bool, holds ...string) (*ast.File, []*Node, map[string][]string) {
	base := "."
	if dir := os.Getenv("RUNFILES_DIR"); dir != "" {
		base = filepath.Join(dir, "_main", "modelchecker")
	}
	file, err := readAstFromFile(filepath.Join(base, "testdata", "formulas", name+".json"))
	require.Nil(t, err)
	noDeadlock := false
	stateConfig := &ast.StateSpaceOptions{
		Options:           &ast.Options{MaxActions: 10, MaxConcurrentActions: 1, CrashOnYield: &crashOnYield},
		DeadlockDetection: &noDeadlock,
	}
	p := NewProcessor([]*ast.File{file}, stateConfig, false, 0, "", "", false, nil, nil, "")
	root, failed, err := p.Start()
	require.Nil(t, err)
	require.Nil(t, failed)
	nodes, _, _, _ := GetAllNodes(root, 10)

	s := newSccGraph(root, nodes)
	notes := map[string][]string{}
	for j, inv := range file.Invariants {
		t.Run(inv.Name, func(t *testing.T) {
			var path []*Link
			if inv.Formula != nil || classicFormula(inv) != nil {
				var err error
				_, path, notes[inv.Name], err = s.checkFormulaAssertion(inv, j, true)
				require.Nil(t, err)
				if inv.Formula == nil {
					// A classic shape: the engine's direct check, which the
					// default engine runs, must agree with the product.
					_, eventuallyAlways := livenessOperators(inv)
					direct, err := s.check(strictLivenessPredicate(nodes, 0, j, eventuallyAlways), eventuallyAlways)
					require.Nil(t, err)
					assert.Equal(t, path == nil, direct == nil, "the product and the direct check disagree")
				}
			} else {
				// Plain `always P` compiles to a safety assertion, which the
				// exploration already checked.
				assert.Equal(t, []string{"always"}, inv.TemporalOperators)
			}
			wantFail := strings.HasPrefix(inv.Name, "Fail") && !slices.Contains(holds, inv.Name)
			if !wantFail {
				assert.Nil(t, path, "unexpected counterexample")
				return
			}
			require.NotNil(t, path, "expected a counterexample")
			assert.Equal(t, root, path[0].Node)
			for i := 1; i < len(path); i++ {
				if path[i].Name == "stutter" {
					continue
				}
				assert.True(t, slices.ContainsFunc(path[i-1].Node.Outbound, func(l *Link) bool { return l.Node == path[i].Node }),
					"step %d (%s) doesn't follow a link", i, path[i].Name)
			}
		})
	}
	return file, nodes, notes
}

func TestFormulaAssertions(t *testing.T) {
	file := checkFormulaSpec(t, "Counter", "FailFor[1]", "FailFor[2]")
	var names []string
	for _, inv := range file.Invariants {
		names = append(names, inv.Name)
	}
	// The header for clauses expand in place, one instance per binding.
	for _, want := range []string{"PassFor[1]", "PassFor[2]", "PassPairs[0, 1]", "PassPairs[1, 2]", "FailFor[1]", "FailFor[3]",
		"PassNested[1, 0]", "PassNested[2, 1]", "PassRecords[record(at = 2)]", "PassConstantKept[3]", "PassShadow[0]"} {
		assert.Contains(t, names, want)
	}
	assert.NotContains(t, names, "PassFor[3]")
	assert.NotContains(t, names, "PassNested[0, 0]")
	assert.NotContains(t, names, "PassFor")
}

// Fairness is the state graph's: whether a formula holds can depend on
// whether an action is weakly or strongly fair, and on whether a run may
// stop.
func TestFormulaFairness(t *testing.T) {
	checkFormulaSpec(t, "Fairness")
	checkFormulaSpec(t, "Stutter")
}

// until, leadsto and implies stay usable as names outside formulas.
func TestFormulaSoftKeywords(t *testing.T) {
	checkFormulaSpec(t, "SoftKeywords")
}

// possibly Q holds where some path, of zero or more steps over fair and
// unfair links, reaches Q. The surrounding operators keep their fair-run
// meaning.
func TestFormulaPossibly(t *testing.T) {
	file, _, notes := checkFormulaSpecWith(t, "Possibly", false)
	for _, inv := range file.Invariants {
		if inv.Name == "PassAlias" {
			// exists eventually is the same operator.
			assert.Equal(t, ast.Formula_POSSIBLY, inv.Formula.Op)
		}
	}
	// The note names the first step from which the ticket can't be booked
	// again: flying, the second step after Init.
	require.Len(t, notes["FailRecoverable"], 1)
	assert.Contains(t, notes["FailRecoverable"][0], "is false in the state after Fly (step 2; Init is step 0)")
	assert.Empty(t, notes["FailNotGuaranteed"])
}

// possibly doesn't follow crash links, so a state reached only by a crash
// isn't possible.
func TestFormulaPossiblyIgnoresCrashes(t *testing.T) {
	_, nodes, _ := checkFormulaSpecWith(t, "PossiblyCrash", true)
	// The state is in the graph, reached by a crash.
	assert.True(t, slices.ContainsFunc(nodes, func(n *Node) bool {
		server := n.Process.Heap.state["server"].(*lib.Role)
		warmed, _ := server.Fields.Attr("warmed")
		cache, _ := server.Fields.Attr("cache")
		return warmed == starlark.True && cache == starlark.MakeInt(0) &&
			slices.ContainsFunc(n.Inbound, func(l *Link) bool { return l.IsCrashLink() })
	}), "no crash leads to a warm server with an empty cache")
}

// Each possibly is one more atom, so the atoms and the uses of possibly
// together can't pass 64.
func TestFormulaPossiblyAtomLimit(t *testing.T) {
	possibly := func(atom int32) *ast.Formula {
		return &ast.Formula{Op: ast.Formula_POSSIBLY, Args: []*ast.Formula{{Op: ast.Formula_ATOM, Atom: atom}}}
	}
	s := &sccGraph{g: &liveness.Graph{}}
	for _, tc := range []struct {
		last int32 // the highest atom index: last+1 atoms
		ok   bool
	}{{61, true}, {62, false}} {
		f := &ast.Formula{Op: ast.Formula_AND, Args: []*ast.Formula{possibly(0), possibly(tc.last)}}
		derived, err := s.derivePossibly(f, nil, nil)
		if tc.ok {
			require.Nil(t, err)
			assert.Equal(t, map[*ast.Formula]int{f.Args[0]: int(tc.last) + 1, f.Args[1]: int(tc.last) + 2}, derived)
		} else {
			require.NotNil(t, err)
			assert.Contains(t, err.Error(), "63 atoms and 2 uses of possibly; at most 64")
		}
	}
}

func TestExpandAssertionHeaderErrors(t *testing.T) {
	assertion := func(name string, clauses ...*ast.ForClause) *ast.Invariant {
		return &ast.Invariant{Name: name, Block: &ast.Block{}, ForClauses: clauses}
	}
	clause := func(iterable string, filters ...string) *ast.ForClause {
		return &ast.ForClause{Targets: []string{"v"}, IterablePyExpr: iterable, FilterPyExprs: filters}
	}
	pairs := func(iterable string) *ast.ForClause {
		return &ast.ForClause{Targets: []string{"a", "b"}, IterablePyExpr: iterable}
	}
	globals := starlark.StringDict{"VALUES": starlark.NewList([]starlark.Value{starlark.MakeInt(1), starlark.String("1")})}
	for name, tc := range map[string]struct {
		invariants []*ast.Invariant
		want       string
	}{
		"Collision":   {[]*ast.Invariant{assertion("A", clause("VALUES"))}, "instance name A[1] is used twice"},
		"NotConstant": {[]*ast.Invariant{assertion("A", clause("state_var"))}, "can use only constants"},
		"NotIterable": {[]*ast.Invariant{assertion("A", clause("3"))}, "isn't iterable"},
		"BadFilter":   {[]*ast.Invariant{assertion("A", clause("range(2)", "1 / 0"))}, "if 1 / 0"},
		"NotATuple":   {[]*ast.Invariant{assertion("A", pairs(`["ab"]`))}, `can't unpack "ab" into a, b`},
		"WrongLength": {[]*ast.Invariant{assertion("A", pairs("[(1, 2, 3)]"))}, "can't unpack (1, 2, 3) into a, b"},
		"Empty":       {[]*ast.Invariant{assertion("A", clause("range(2)", "v > 5")), assertion("B")}, ""},
	} {
		t.Run(name, func(t *testing.T) {
			file := &ast.File{Invariants: tc.invariants}
			_, err := ExpandAssertionHeaders(file, globals, NewModelChecker("test"))
			if tc.want == "" {
				require.Nil(t, err)
				require.Len(t, file.Invariants, 1)
				assert.Equal(t, "B", file.Invariants[0].Name)
				return
			}
			require.NotNil(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}
