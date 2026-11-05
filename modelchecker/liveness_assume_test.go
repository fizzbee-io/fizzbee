package modelchecker

import (
	"math/rand"
	"testing"

	ast "fizz/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestApplyAssumptionsRestores checks, on random graphs, that an assertion's
// @assume changes exactly the links of the actions it names, to the level it
// names, and that restoring puts every link back, so one assertion's
// assumptions never reach another assertion's check.
func TestApplyAssumptionsRestores(t *testing.T) {
	actions := []string{"", "A", "B", "Role.C", "Role.D"}
	levels := []ast.FairnessLevel{
		ast.FairnessLevel_FAIRNESS_LEVEL_UNKNOWN,
		ast.FairnessLevel_FAIRNESS_LEVEL_UNFAIR,
		ast.FairnessLevel_FAIRNESS_LEVEL_WEAK,
		ast.FairnessLevel_FAIRNESS_LEVEL_STRONG,
		ast.FairnessLevel_FAIRNESS_LEVEL_FAULT,
	}
	r := rand.New(rand.NewSource(1))
	for iter := 0; iter < 500; iter++ {
		nodes := make([]*Node, 1+r.Intn(8))
		for i := range nodes {
			nodes[i] = &Node{}
		}
		for e := r.Intn(20); e > 0; e-- {
			from, to := nodes[r.Intn(len(nodes))], nodes[r.Intn(len(nodes))]
			name := "thread-0"
			if r.Intn(5) == 0 {
				name = "crash"
			}
			l := &Link{Node: to, Name: name, Action: actions[r.Intn(len(actions))], Fairness: levels[r.Intn(len(levels))]}
			from.Outbound = append(from.Outbound, l)
			// Inbound links are separate objects in the explored graph.
			in := *l
			in.Node = from
			to.Inbound = append(to.Inbound, &in)
		}
		a := &ast.Assumption{}
		lists := []*[]string{&a.Finite, &a.WeakFair, &a.StrongFair, &a.Unfair}
		for _, name := range actions[1:] {
			if k := r.Intn(len(lists) + 1); k < len(lists) {
				*lists[k] = append(*lists[k], name)
			}
		}
		assumed := assumedFairness(a)

		type snap struct {
			l *Link
			f ast.FairnessLevel
		}
		var before []snap
		for _, n := range nodes {
			for _, l := range append(append([]*Link{}, n.Outbound...), n.Inbound...) {
				before = append(before, snap{l, l.Fairness})
			}
		}

		restore := applyAssumptions(nodes, a)
		for _, s := range before {
			want := s.f
			if f, ok := assumed[s.l.Action]; ok && s.l.Action != "" && !s.l.IsCrashLink() {
				want = f
			}
			require.Equal(t, want, s.l.Fairness, "iteration %d, link %s/%q", iter, s.l.Name, s.l.Action)
		}
		restore()
		for _, s := range before {
			require.Equal(t, s.f, s.l.Fairness, "iteration %d: not restored", iter)
		}
	}
}

func TestValidateAssumptions(t *testing.T) {
	liveness := func(a *ast.Assumption) *ast.Invariant {
		return &ast.Invariant{Name: "P", TemporalOperators: []string{"eventually", "always"}, Block: &ast.Block{}, Assumption: a}
	}
	file := func(inv *ast.Invariant) *ast.File {
		return &ast.File{
			Actions:    []*ast.Action{{Name: "Init"}, {Name: "Delete"}},
			Roles:      []*ast.Role{{Name: "User", Actions: []*ast.Action{{Name: "Init"}, {Name: "Restore"}}}},
			Invariants: []*ast.Invariant{inv},
		}
	}
	assert.NoError(t, ValidateAssumptions(file(liveness(&ast.Assumption{Finite: []string{"Delete"}, WeakFair: []string{"User.Restore"}}))))
	assert.ErrorContains(t, ValidateAssumptions(file(liveness(&ast.Assumption{Finite: []string{"Restore"}}))), "not an action")
	assert.ErrorContains(t, ValidateAssumptions(file(liveness(&ast.Assumption{WeakFair: []string{"Init"}}))), "not an action")
	assert.ErrorContains(t, ValidateAssumptions(file(liveness(&ast.Assumption{WeakFair: []string{"User.Init"}}))), "not an action")
	assert.ErrorContains(t, ValidateAssumptions(file(liveness(&ast.Assumption{Finite: []string{"Delete"}, Unfair: []string{"Delete"}}))), "in both")
	assert.ErrorContains(t, ValidateAssumptions(file(liveness(&ast.Assumption{Finite: []string{"Delete", "Delete"}}))), "more than once")
	// An empty list next to a non-empty one is fine; an assumption with only
	// empty lists assumes nothing.
	assert.NoError(t, ValidateAssumptions(file(liveness(&ast.Assumption{Finite: []string{}, WeakFair: []string{"User.Restore"}}))))
	assert.False(t, hasAssumptions(liveness(&ast.Assumption{Finite: []string{}})))
	formula := &ast.Invariant{Name: "F", Formula: &ast.Formula{}, Block: &ast.Block{}, Assumption: &ast.Assumption{Finite: []string{"Delete"}}}
	assert.NoError(t, ValidateAssumptions(file(formula)))
	safety := &ast.Invariant{Name: "S", TemporalOperators: []string{"always"}, Block: &ast.Block{}, Assumption: &ast.Assumption{Finite: []string{"Delete"}}}
	assert.ErrorContains(t, ValidateAssumptions(file(safety)), "only to liveness")
}
