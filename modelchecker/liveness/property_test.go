package liveness_test

import (
	"math/rand"
	"testing"

	"github.com/fizzbee-io/fizzbee/modelchecker/liveness"
)

// source is the randomness randomGraph draws from: a *rand.Rand for the
// property tests, or fuzzer bytes for FuzzCheck.
type source interface {
	Intn(n int) int
}

type byteSource struct {
	data []byte
}

func (s *byteSource) Intn(n int) int {
	if len(s.data) == 0 {
		return 0
	}
	b := int(s.data[0])
	s.data = s.data[1:]
	return b % n
}

func randomBools(r source, n int) []bool {
	out := make([]bool, n)
	for i := range out {
		out[i] = r.Intn(2) == 0
	}
	return out
}

// randomGraph returns a small graph with few enough edges for the oracle.
func randomGraph(r source) *liveness.Graph {
	n := 1 + r.Intn(6)
	g := &liveness.Graph{NumNodes: n}
	for a := 1 + r.Intn(4); a > 0; a-- {
		g.Actions = append(g.Actions, liveness.Fairness(r.Intn(4)))
	}
	for e := r.Intn(13); e > 0; e-- {
		g.Edges = append(g.Edges, liveness.Edge{From: r.Intn(n), To: r.Intn(n), Action: r.Intn(len(g.Actions))})
	}
	for i := 1 + r.Intn(2); i > 0; i-- {
		g.Init = append(g.Init, r.Intn(n))
	}
	if r.Intn(4) != 0 {
		g.ReadsEnabled = randomBools(r, n)
	}
	if r.Intn(3) == 0 {
		g.WeakUnobserved = randomBools(r, n)
	}

	switch r.Intn(6) {
	case 0:
		// Enabledness unrelated to the edges, as a caller may report it.
		g.Enabled = make([][]int, n)
		for v := range g.Enabled {
			for a := range g.Actions {
				if r.Intn(2) == 0 {
					g.Enabled[v] = append(g.Enabled[v], a)
				}
			}
		}
	case 1, 2:
		g.EnabledFromEdges()
		for v := range g.Enabled {
			if r.Intn(3) == 0 {
				g.Enabled[v] = append(g.Enabled[v], r.Intn(len(g.Actions)))
			}
		}
	default:
		g.EnabledFromEdges()
	}

	switch r.Intn(4) {
	case 0:
		g.SetAlwaysEventually(randomBools(r, n))
	case 1:
		g.SetEventuallyAlways(randomBools(r, n))
	default:
		g.NumAcceptSets = r.Intn(3)
		if g.NumAcceptSets > 0 {
			g.Accept = make([]uint64, n)
			for v := range g.Accept {
				// Bit 2 lies outside the sets and must be ignored.
				g.Accept[v] = uint64(r.Intn(1 << uint(g.NumAcceptSets+1)))
			}
		}
		if r.Intn(2) == 0 {
			g.LoopAllowed = randomBools(r, n)
		}
		if r.Intn(2) == 0 {
			g.FailsIfStuck = randomBools(r, n)
		}
	}
	return g
}

// checkAgainstOracle runs Check, validates any lasso it returns, compares the
// verdict with the oracle and returns whether a counterexample exists.
func checkAgainstOracle(t *testing.T, g *liveness.Graph) bool {
	t.Helper()
	l, err := liveness.Check(g)
	if err != nil {
		t.Fatalf("Check: %v\ngraph: %+v", err, *g)
	}
	if l != nil {
		if err := liveness.ValidateLasso(g, l); err != nil {
			t.Fatalf("invalid lasso %+v: %v\ngraph: %+v", *l, err, *g)
		}
	}
	if want := oracle(g); (l != nil) != want {
		t.Fatalf("Check found counterexample %v, oracle says %v\nlasso: %+v\ngraph: %+v", l != nil, want, l, *g)
	}
	return l != nil
}

func iterations(t *testing.T) int {
	if testing.Short() {
		return 2000
	}
	return 30000
}

func TestMatchesOracle(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	found := 0
	for i := 0; i < iterations(t); i++ {
		if checkAgainstOracle(t, randomGraph(r)) {
			found++
		}
	}
	// Guard against a generator that only produces trivial graphs.
	if found == 0 || found == iterations(t) {
		t.Fatalf("generator is degenerate: %d of %d graphs have counterexamples", found, iterations(t))
	}
}

// permute relabels nodes and shuffles edge and Enabled order.
func permute(g *liveness.Graph, r *rand.Rand) *liveness.Graph {
	p := r.Perm(g.NumNodes)
	h := &liveness.Graph{NumNodes: g.NumNodes, Actions: g.Actions, NumAcceptSets: g.NumAcceptSets}
	for _, n := range g.Init {
		h.Init = append(h.Init, p[n])
	}
	for _, i := range r.Perm(len(g.Edges)) {
		e := g.Edges[i]
		h.Edges = append(h.Edges, liveness.Edge{From: p[e.From], To: p[e.To], Action: e.Action})
	}
	if g.Enabled != nil {
		h.Enabled = make([][]int, g.NumNodes)
		for n, as := range g.Enabled {
			for _, i := range r.Perm(len(as)) {
				h.Enabled[p[n]] = append(h.Enabled[p[n]], as[i])
			}
		}
	}
	permBools := func(s []bool) []bool {
		if s == nil {
			return nil
		}
		out := make([]bool, len(s))
		for n, v := range s {
			out[p[n]] = v
		}
		return out
	}
	h.ReadsEnabled = permBools(g.ReadsEnabled)
	h.WeakUnobserved = permBools(g.WeakUnobserved)
	h.LoopAllowed = permBools(g.LoopAllowed)
	h.FailsIfStuck = permBools(g.FailsIfStuck)
	if g.Accept != nil {
		h.Accept = make([]uint64, g.NumNodes)
		for n, v := range g.Accept {
			h.Accept[p[n]] = v
		}
	}
	return h
}

func TestPermutationInvariant(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	for i := 0; i < iterations(t); i++ {
		g := randomGraph(r)
		if checkAgainstOracle(t, g) != checkAgainstOracle(t, permute(g, r)) {
			t.Fatalf("verdict changed under permutation\ngraph: %+v", *g)
		}
	}
}

// addUnreachable appends nodes that no initial node can reach. Their edges may
// lead back into the original graph, but never the other way.
func addUnreachable(g *liveness.Graph, r *rand.Rand) *liveness.Graph {
	old, extra := g.NumNodes, 1+r.Intn(4)
	h := *g
	h.NumNodes = old + extra
	h.Edges = append([]liveness.Edge(nil), g.Edges...)
	for i := r.Intn(8); i > 0; i-- {
		h.Edges = append(h.Edges, liveness.Edge{From: old + r.Intn(extra), To: r.Intn(h.NumNodes), Action: r.Intn(len(g.Actions))})
	}
	growBools := func(s []bool) []bool {
		if s == nil {
			return nil
		}
		return append(append([]bool(nil), s...), randomBools(r, extra)...)
	}
	h.ReadsEnabled = growBools(g.ReadsEnabled)
	h.WeakUnobserved = growBools(g.WeakUnobserved)
	h.LoopAllowed = growBools(g.LoopAllowed)
	h.FailsIfStuck = growBools(g.FailsIfStuck)
	if g.Enabled != nil {
		h.Enabled = append([][]int(nil), g.Enabled...)
		for i := 0; i < extra; i++ {
			h.Enabled = append(h.Enabled, []int{r.Intn(len(g.Actions))})
		}
	}
	if g.Accept != nil {
		h.Accept = append([]uint64(nil), g.Accept...)
		for i := 0; i < extra; i++ {
			h.Accept = append(h.Accept, ^uint64(0))
		}
	}
	return &h
}

func TestUnreachableNodesIgnored(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	for i := 0; i < iterations(t); i++ {
		g := randomGraph(r)
		if checkAgainstOracle(t, g) != checkAgainstOracle(t, addUnreachable(g, r)) {
			t.Fatalf("unreachable nodes changed the verdict\ngraph: %+v", *g)
		}
	}
}

// Making an action fairer removes fair runs, so it can only remove
// counterexamples.
func TestFairnessMonotone(t *testing.T) {
	r := rand.New(rand.NewSource(4))
	for i := 0; i < iterations(t); i++ {
		g := randomGraph(r)
		before := checkAgainstOracle(t, g)
		for a, f := range g.Actions {
			if f != liveness.Unfair && f != liveness.Weak {
				continue
			}
			h := *g
			h.Actions = append([]liveness.Fairness(nil), g.Actions...)
			h.Actions[a] = f + 1
			if after := checkAgainstOracle(t, &h); after && !before {
				t.Fatalf("making action %d %v created a counterexample\ngraph: %+v", a, h.Actions[a], *g)
			}
		}
	}
}

func FuzzCheck(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{2, 1, 1, 3, 0, 1, 0, 1, 0, 0, 0, 0, 1})
	f.Add([]byte("liveness fuzz seed with several bytes of structure"))
	f.Fuzz(func(t *testing.T, data []byte) {
		checkAgainstOracle(t, randomGraph(&byteSource{data: data}))
	})
}
