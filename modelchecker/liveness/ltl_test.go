package liveness_test

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/fizzbee-io/fizzbee/modelchecker/liveness"
)

func randomFormula(r *rand.Rand, depth int) *liveness.Formula {
	if depth == 0 || r.Intn(4) == 0 {
		if r.Intn(8) == 0 {
			return liveness.True()
		}
		return liveness.Atom(r.Intn(2))
	}
	sub := func() *liveness.Formula { return randomFormula(r, depth-1) }
	switch r.Intn(9) {
	case 0:
		return liveness.Not(sub())
	case 1:
		return liveness.And(sub(), sub())
	case 2:
		return liveness.Or(sub(), sub())
	case 3, 4:
		return liveness.Until(sub(), sub())
	case 5:
		return liveness.Eventually(sub())
	case 6:
		return liveness.Always(sub())
	case 7:
		return liveness.Implies(sub(), sub())
	default:
		return liveness.LeadsTo(sub(), sub())
	}
}

func formulaString(f *liveness.Formula) string {
	switch f.Op {
	case liveness.OpAtom:
		return fmt.Sprintf("p%d", f.Atom)
	case liveness.OpTrue:
		return "true"
	case liveness.OpNot:
		return "!" + formulaString(f.Args[0])
	case liveness.OpUntil:
		return "(" + formulaString(f.Args[0]) + " U " + formulaString(f.Args[1]) + ")"
	}
	op := " & "
	if f.Op == liveness.OpOr {
		op = " | "
	}
	s := "("
	for i, g := range f.Args {
		if i > 0 {
			s += op
		}
		s += formulaString(g)
	}
	return s + ")"
}

// holds evaluates f on the word prefix·loop^ω straight from the semantics:
// position i's successor is i+1, except that the last one returns to the
// start of the loop. Until is the least fixpoint of its expansion.
func holds(f *liveness.Formula, prefix, loop []uint64) bool {
	word := append(append([]uint64{}, prefix...), loop...)
	n := len(word)
	succ := func(i int) int {
		if i == n-1 {
			return len(prefix)
		}
		return i + 1
	}
	var truth func(f *liveness.Formula) []bool
	truth = func(f *liveness.Formula) []bool {
		out := make([]bool, n)
		switch f.Op {
		case liveness.OpAtom:
			for i := range out {
				out[i] = word[i]&(1<<uint(f.Atom)) != 0
			}
		case liveness.OpTrue:
			for i := range out {
				out[i] = true
			}
		case liveness.OpNot:
			a := truth(f.Args[0])
			for i := range out {
				out[i] = !a[i]
			}
		case liveness.OpAnd, liveness.OpOr:
			for i := range out {
				out[i] = f.Op == liveness.OpAnd
			}
			for _, g := range f.Args {
				a := truth(g)
				for i := range out {
					if f.Op == liveness.OpAnd {
						out[i] = out[i] && a[i]
					} else {
						out[i] = out[i] || a[i]
					}
				}
			}
		case liveness.OpUntil:
			a, b := truth(f.Args[0]), truth(f.Args[1])
			for changed := true; changed; {
				changed = false
				for i := range out {
					if !out[i] && (b[i] || a[i] && out[succ(i)]) {
						out[i], changed = true, true
					}
				}
			}
		}
		return out
	}
	return truth(f)[0]
}

// wordGraph is a single run: a path of nodes, either looping back or ending
// at a node the run stays at forever. Some nodes are unlabeled, as steps
// inside an action are.
type wordGraph struct {
	g       *liveness.Graph
	labeled []bool
	letter  []uint64
	canStop []bool
	// The word up to repeated letters, which LTL without next can't see.
	prefix, loop []uint64
	hasWord      bool
}

func randomWordGraph(r *rand.Rand) wordGraph {
	n := 1 + r.Intn(6)
	w := wordGraph{
		g:       &liveness.Graph{NumNodes: n, Init: []int{0}, Actions: []liveness.Fairness{liveness.Unfair}},
		labeled: make([]bool, n),
		letter:  make([]uint64, n),
		canStop: make([]bool, n),
	}
	for i := 0; i < n; i++ {
		w.labeled[i] = r.Intn(4) != 0
		w.letter[i] = uint64(r.Intn(4))
		if i+1 < n {
			w.g.Edges = append(w.g.Edges, liveness.Edge{From: i, To: i + 1})
		}
	}
	loopStart := -1
	if r.Intn(3) != 0 {
		loopStart = r.Intn(n)
		w.g.Edges = append(w.g.Edges, liveness.Edge{From: n - 1, To: loopStart})
	} else {
		w.canStop[n-1] = true
	}
	w.g.EnabledFromEdges()

	var labels []uint64
	for i := 0; i < n; i++ {
		if i == loopStart {
			w.prefix = labels
			labels = nil
		}
		if w.labeled[i] {
			labels = append(labels, w.letter[i])
		}
	}
	if loopStart < 0 {
		w.prefix = labels
		labels = nil
	}
	w.loop = labels
	switch {
	case len(w.prefix) == 0 && len(w.loop) == 0:
		// No labeled node, so no word.
	case len(w.loop) == 0:
		w.loop = []uint64{w.prefix[len(w.prefix)-1]}
		w.hasWord = true
	default:
		w.hasWord = true
	}
	return w
}

// The product has an accepting run exactly when the run's word satisfies the
// formula, with unlabeled nodes repeating the letter before them.
func TestProductMatchesSemantics(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	satisfied := 0
	for i := 0; i < iterations(t); i++ {
		w := randomWordGraph(r)
		f := randomFormula(r, 3)
		p, err := liveness.NewProduct(w.g, f, w.labeled, w.letter, w.canStop)
		if err != nil {
			t.Fatal(err)
		}
		l, err := liveness.Check(p.Graph)
		if err != nil {
			t.Fatal(err)
		}
		want := w.hasWord && holds(f, w.prefix, w.loop)
		if (l != nil) != want {
			t.Fatalf("formula %s on word %v·(%v)^ω (hasWord %v): product found run %v, want %v\ngraph %+v labeled %v letters %v",
				formulaString(f), w.prefix, w.loop, w.hasWord, l != nil, want, *w.g, w.labeled, w.letter)
		}
		if l != nil {
			if err := liveness.ValidateLasso(p.Graph, l); err != nil {
				t.Fatalf("invalid lasso: %v", err)
			}
			satisfied++
		}
	}
	if satisfied == 0 || satisfied == iterations(t) {
		t.Fatalf("generator is degenerate: %d of %d formulas hold", satisfied, iterations(t))
	}
}

// On the classic forms, the product with the negated formula must agree with
// the engine's direct encodings, fairness included.
func TestProductMatchesClassicForms(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	for i := 0; i < iterations(t); i++ {
		g := randomGraph(r)
		q := randomBools(r, g.NumNodes)
		eventuallyAlways := r.Intn(2) == 0
		var f *liveness.Formula
		if eventuallyAlways {
			g.SetEventuallyAlways(q)
			f = liveness.Not(liveness.Eventually(liveness.Always(liveness.Atom(0))))
		} else {
			g.SetAlwaysEventually(q)
			f = liveness.Not(liveness.Always(liveness.Eventually(liveness.Atom(0))))
		}
		want := checkAgainstOracle(t, g)

		labeled := make([]bool, g.NumNodes)
		letter := make([]uint64, g.NumNodes)
		canStop := make([]bool, g.NumNodes)
		for n := range labeled {
			labeled[n], canStop[n] = true, true
			if q[n] {
				letter[n] = 1
			}
		}
		plain := *g
		plain.Accept, plain.NumAcceptSets, plain.LoopAllowed, plain.FailsIfStuck = nil, 0, nil, nil
		p, err := liveness.NewProduct(&plain, f, labeled, letter, canStop)
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Graph.Edges) <= 16 {
			if got := checkAgainstOracle(t, p.Graph); got != want {
				t.Fatalf("eventuallyAlways=%v q=%v: product says %v, direct encoding %v\ngraph %+v", eventuallyAlways, q, got, want, *g)
			}
			continue
		}
		l, err := liveness.Check(p.Graph)
		if err != nil {
			t.Fatal(err)
		}
		if (l != nil) != want {
			t.Fatalf("eventuallyAlways=%v q=%v: product says %v, direct encoding %v\ngraph %+v", eventuallyAlways, q, l != nil, want, *g)
		}
	}
}

// Product edges and nodes map back onto the state graph.
func TestProductMapsToGraph(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	for i := 0; i < 500; i++ {
		w := randomWordGraph(r)
		p, err := liveness.NewProduct(w.g, randomFormula(r, 3), w.labeled, w.letter, w.canStop)
		if err != nil {
			t.Fatal(err)
		}
		for e, pe := range p.Graph.Edges {
			ge := w.g.Edges[p.Edge[e]]
			if p.Node[pe.From] != ge.From || p.Node[pe.To] != ge.To || pe.Action != ge.Action {
				t.Fatalf("product edge %d %+v maps to graph edge %+v", e, pe, ge)
			}
		}
	}
}

// Fairness is judged on the state graph's actions, even where the product
// drops an action's edges. Here A leads to p, so the product with
// always !p has no A edges, yet A stays enabled. Node 0 loops through node 2,
// where A isn't enabled, so only strong fairness forces A.
func TestProductFairnessOnDroppedEdges(t *testing.T) {
	const b, a = 0, 1
	for _, tc := range []struct {
		fairness liveness.Fairness
		loop     bool // whether 0 loops through 2 rather than on itself
		want     bool // whether there's a fair run where p never holds
	}{
		{liveness.Unfair, false, true},
		{liveness.Weak, false, false},
		{liveness.Strong, false, false},
		{liveness.Unfair, true, true},
		{liveness.Weak, true, true},
		{liveness.Strong, true, false},
	} {
		g := &liveness.Graph{NumNodes: 3, Init: []int{0}, Actions: []liveness.Fairness{liveness.Unfair, tc.fairness}}
		g.Edges = []liveness.Edge{{From: 0, To: 1, Action: a}, {From: 1, To: 1, Action: b}}
		if tc.loop {
			g.Edges = append(g.Edges, liveness.Edge{From: 0, To: 2, Action: b}, liveness.Edge{From: 2, To: 0, Action: b})
		} else {
			g.Edges = append(g.Edges, liveness.Edge{From: 0, To: 0, Action: b})
		}
		g.EnabledFromEdges()
		labeled := []bool{true, true, true}
		letter := []uint64{0, 1, 0}
		p, err := liveness.NewProduct(g, liveness.Not(liveness.Eventually(liveness.Atom(0))), labeled, letter, make([]bool, 3))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range p.Graph.Edges {
			if e.Action == a {
				t.Fatalf("%+v: the product kept an edge of A", tc)
			}
		}
		l, err := liveness.Check(p.Graph)
		if err != nil {
			t.Fatal(err)
		}
		if (l != nil) != tc.want {
			t.Errorf("fairness %v, loop %v: found a run %v, want %v", tc.fairness, tc.loop, l != nil, tc.want)
		}
		if got := checkAgainstOracle(t, p.Graph); got != tc.want {
			t.Errorf("fairness %v, loop %v: oracle says %v, want %v", tc.fairness, tc.loop, got, tc.want)
		}
	}
}
