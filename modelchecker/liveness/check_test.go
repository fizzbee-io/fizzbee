package liveness_test

import (
	"testing"

	"github.com/fizzbee-io/fizzbee/modelchecker/liveness"
)

func mustCheck(t *testing.T, g *liveness.Graph) *liveness.Lasso {
	t.Helper()
	l, err := liveness.Check(g)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if l != nil {
		if err := liveness.ValidateLasso(g, l); err != nil {
			t.Fatalf("invalid lasso %+v: %v", *l, err)
		}
	}
	if want := oracle(g); (l != nil) != want {
		t.Fatalf("Check found counterexample %v, oracle says %v", l != nil, want)
	}
	return l
}

func loopActions(g *liveness.Graph, l *liveness.Lasso) map[int]bool {
	out := map[int]bool{}
	for _, e := range l.Loop {
		out[g.Edges[e].Action] = true
	}
	return out
}

// A goes to B or C and both return to A. Under weak fairness on "go to C",
// A -> B -> A forever is fair because the action is disabled at B, so
// "always eventually C" fails. Strong fairness rules that loop out.
func TestWeakVersusStrong(t *testing.T) {
	const a, b, c = 0, 1, 2
	for _, tc := range []struct {
		toC  liveness.Fairness
		fail bool
	}{
		{liveness.Unfair, true},
		{liveness.Weak, true},
		{liveness.Strong, false},
	} {
		g := &liveness.Graph{
			NumNodes: 3,
			Init:     []int{a},
			Actions:  []liveness.Fairness{liveness.Weak, liveness.Weak, tc.toC},
			Edges: []liveness.Edge{
				{From: a, To: b, Action: 0},
				{From: b, To: a, Action: 1},
				{From: a, To: c, Action: 2},
				{From: c, To: a, Action: 1},
			},
		}
		g.EnabledFromEdges()
		g.SetAlwaysEventually([]bool{false, false, true})
		l := mustCheck(t, g)
		if (l != nil) != tc.fail {
			t.Errorf("toC %v: got counterexample %v, want %v", tc.toC, l != nil, tc.fail)
		}
	}
}

// Node 0 has two weak self-loops X and Y. Taking only X leaves Y enabled
// forever, and the other way round, so neither simple cycle is fair, but
// alternating them is. The checker must find the alternating loop.
func TestAlternatingCycles(t *testing.T) {
	g := &liveness.Graph{
		NumNodes: 1,
		Init:     []int{0},
		Actions:  []liveness.Fairness{liveness.Weak, liveness.Weak},
		Edges:    []liveness.Edge{{From: 0, To: 0, Action: 0}, {From: 0, To: 0, Action: 1}},
	}
	g.EnabledFromEdges()
	g.SetAlwaysEventually([]bool{false})
	l := mustCheck(t, g)
	if l == nil {
		t.Fatal("no counterexample")
	}
	if acts := loopActions(g, l); !acts[0] || !acts[1] {
		t.Errorf("loop takes actions %v, want both", acts)
	}
}

func TestCrashLinks(t *testing.T) {
	t.Run("cannot close a loop", func(t *testing.T) {
		g := &liveness.Graph{
			NumNodes: 2,
			Init:     []int{0},
			Actions:  []liveness.Fairness{liveness.Weak, liveness.Crash},
			Edges:    []liveness.Edge{{From: 0, To: 1, Action: 0}, {From: 1, To: 0, Action: 1}},
		}
		g.EnabledFromEdges()
		g.SetAlwaysEventually([]bool{false, true})
		g.FailsIfStuck = nil
		if l := mustCheck(t, g); l != nil {
			t.Errorf("unexpected counterexample %+v", *l)
		}
	})
	t.Run("may appear in the prefix", func(t *testing.T) {
		g := &liveness.Graph{
			NumNodes: 2,
			Init:     []int{0},
			Actions:  []liveness.Fairness{liveness.Weak, liveness.Crash},
			Edges:    []liveness.Edge{{From: 0, To: 1, Action: 1}, {From: 1, To: 1, Action: 0}},
		}
		g.EnabledFromEdges()
		g.SetAlwaysEventually([]bool{true, false})
		l := mustCheck(t, g)
		if l == nil || len(l.Prefix) != 1 || l.Prefix[0] != 0 {
			t.Errorf("want a counterexample through the crash link, got %+v", l)
		}
	})
	t.Run("never creates an obligation", func(t *testing.T) {
		// A crash link enabled everywhere does not make a loop unfair.
		g := &liveness.Graph{
			NumNodes: 2,
			Init:     []int{0},
			Actions:  []liveness.Fairness{liveness.Weak, liveness.Crash},
			Edges:    []liveness.Edge{{From: 0, To: 0, Action: 0}, {From: 0, To: 1, Action: 1}},
		}
		g.EnabledFromEdges()
		g.SetAlwaysEventually([]bool{false, true})
		if l := mustCheck(t, g); l == nil || l.Stutter {
			t.Errorf("want a loop counterexample, got %+v", l)
		}
	})
}

// Staying forever at a node that fails the property is a counterexample
// exactly when staying is fair: no weak or strong action is enabled there, or
// enabledness is not observed there.
func TestStutter(t *testing.T) {
	for _, tc := range []struct {
		name  string
		f     liveness.Fairness
		reads bool
		fail  bool
	}{
		{"unfair action enabled", liveness.Unfair, true, true},
		{"weak action enabled", liveness.Weak, true, false},
		{"strong action enabled", liveness.Strong, true, false},
		{"weak action enabled, not observed", liveness.Weak, false, true},
		{"crash action enabled", liveness.Crash, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := &liveness.Graph{
				NumNodes:     2,
				Init:         []int{0},
				Actions:      []liveness.Fairness{tc.f},
				Edges:        []liveness.Edge{{From: 0, To: 1, Action: 0}},
				ReadsEnabled: []bool{tc.reads, true},
			}
			g.EnabledFromEdges()
			g.SetAlwaysEventually([]bool{false, true})
			l := mustCheck(t, g)
			if (l != nil) != tc.fail {
				t.Fatalf("got counterexample %+v, want %v", l, tc.fail)
			}
			if l != nil && (!l.Stutter || l.Start != 0) {
				t.Errorf("want stutter at node 0, got %+v", *l)
			}
		})
	}
	t.Run("deadlock", func(t *testing.T) {
		g := &liveness.Graph{NumNodes: 1, Init: []int{0}}
		g.SetAlwaysEventually([]bool{false})
		if l := mustCheck(t, g); l == nil || !l.Stutter {
			t.Errorf("want stutter, got %+v", l)
		}
	})
	t.Run("nearest", func(t *testing.T) {
		g := &liveness.Graph{
			NumNodes: 4,
			Init:     []int{0},
			Actions:  []liveness.Fairness{liveness.Unfair},
			Edges:    []liveness.Edge{{From: 0, To: 1}, {From: 1, To: 2}, {From: 0, To: 3}},
		}
		g.EnabledFromEdges()
		g.SetAlwaysEventually([]bool{true, true, false, false})
		if l := mustCheck(t, g); l == nil || l.Start != 3 || len(l.Prefix) != 1 {
			t.Errorf("want stutter at node 3 after one step, got %+v", l)
		}
	})
}

func TestEventuallyAlways(t *testing.T) {
	// 0 -> 1 is weak; 1 loops. Staying at 0 via its unfair self-loop leaves
	// the weak action enabled forever, so "eventually always at 1" holds,
	// unless node 0 does not observe enabledness.
	for _, reads := range []bool{true, false} {
		g := &liveness.Graph{
			NumNodes:     2,
			Init:         []int{0},
			Actions:      []liveness.Fairness{liveness.Unfair, liveness.Weak},
			Edges:        []liveness.Edge{{From: 0, To: 0, Action: 0}, {From: 0, To: 1, Action: 1}, {From: 1, To: 1, Action: 1}},
			ReadsEnabled: []bool{reads, true},
		}
		g.EnabledFromEdges()
		g.SetEventuallyAlways([]bool{false, true})
		if l := mustCheck(t, g); (l != nil) == reads {
			t.Errorf("reads=%v: got counterexample %+v", reads, l)
		}
	}
	// Bouncing between a q node and a not-q node forever violates it.
	g := &liveness.Graph{
		NumNodes: 2,
		Init:     []int{0},
		Actions:  []liveness.Fairness{liveness.Weak},
		Edges:    []liveness.Edge{{From: 0, To: 1}, {From: 1, To: 0}},
	}
	g.EnabledFromEdges()
	g.SetEventuallyAlways([]bool{true, false})
	if l := mustCheck(t, g); l == nil {
		t.Error("no counterexample")
	}
}

// Two acceptance sets on different nodes of one component: the loop must
// visit both, as for the product with a generalized Büchi automaton.
func TestGeneralizedAcceptance(t *testing.T) {
	g := &liveness.Graph{
		NumNodes:      3,
		Init:          []int{0},
		Actions:       []liveness.Fairness{liveness.Weak},
		Edges:         []liveness.Edge{{From: 0, To: 1}, {From: 1, To: 0}, {From: 0, To: 2}, {From: 2, To: 0}},
		Accept:        []uint64{0, 1, 2},
		NumAcceptSets: 2,
	}
	g.EnabledFromEdges()
	l := mustCheck(t, g)
	if l == nil {
		t.Fatal("no counterexample")
	}
	if len(l.Loop) != 4 {
		t.Errorf("want a loop through both 1 and 2, got %+v", *l)
	}
}

// The component check is iterative, so long chains must not exhaust the
// stack, and the engine must stay linear-ish on them.
func TestLongCycle(t *testing.T) {
	const n = 200000
	g := &liveness.Graph{NumNodes: n, Init: []int{0}, Actions: []liveness.Fairness{liveness.Weak}}
	for i := 0; i < n; i++ {
		g.Edges = append(g.Edges, liveness.Edge{From: i, To: (i + 1) % n})
	}
	g.EnabledFromEdges()
	q := make([]bool, n)
	g.SetAlwaysEventually(q)
	l, err := liveness.Check(g)
	if err != nil || l == nil || len(l.Loop) != n {
		t.Fatalf("want the whole cycle, got err=%v loop length %d", err, len(l.Loop))
	}
	if err := liveness.ValidateLasso(g, l); err != nil {
		t.Fatal(err)
	}
	q[n/2] = true
	g.SetAlwaysEventually(q)
	if l, err := liveness.Check(g); err != nil || l != nil {
		t.Fatalf("want no counterexample, got %+v, %v", l, err)
	}
}

func TestInvalidGraph(t *testing.T) {
	for _, g := range []*liveness.Graph{
		{NumNodes: 1, Init: []int{1}},
		{NumNodes: 1, Edges: []liveness.Edge{{From: 0, To: 0, Action: 0}}},
		{NumNodes: 2, Accept: []uint64{0}},
		{NumNodes: 1, NumAcceptSets: 65},
	} {
		if _, err := liveness.Check(g); err == nil {
			t.Errorf("no error for %+v", *g)
		}
	}
}
