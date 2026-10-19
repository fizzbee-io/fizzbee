// Package liveness checks liveness properties on an explored state graph.
//
// A violation is a lasso: a finite prefix from an initial node into a loop
// that repeats forever, where the loop is accepting and satisfies every
// fairness constraint. Instead of enumerating cycles, the checker works on
// strongly connected components (the Emerson-Lei emptiness check), so the
// answer does not depend on the order actions or links were explored in.
//
// The package knows nothing about Starlark, protos or FizzBee nodes. Callers
// convert their graph into a Graph of integer ids, so the same engine can
// later run on the product of the state graph and a property automaton.
package liveness

import "fmt"

type Fairness uint8

const (
	Unfair Fairness = iota
	Weak
	Strong
	// Crash marks injected fault links. They may appear in a counterexample's
	// prefix but never in its loop, and they never create a fairness
	// obligation.
	Crash
)

func (f Fairness) String() string {
	switch f {
	case Unfair:
		return "unfair"
	case Weak:
		return "weak"
	case Strong:
		return "strong"
	case Crash:
		return "crash"
	}
	return fmt.Sprintf("Fairness(%d)", uint8(f))
}

// Edge is a link between two nodes, labelled with the action it takes.
type Edge struct {
	From, To int
	Action   int
}

// Graph is the input to Check. Nodes are the integers 0..NumNodes-1, which
// may be state graph nodes or (node, automaton state) pairs. The optional
// per-node slices may be nil, which means the documented default.
type Graph struct {
	NumNodes int
	Init     []int
	Edges    []Edge

	// Actions[a] is the fairness of action a. Every Edge.Action and every
	// action listed in Enabled must be a valid index.
	Actions []Fairness

	// Enabled[n] lists the actions enabled at node n. Only weak and strong
	// actions matter; others are ignored. EnabledFromEdges fills it with
	// the actions of each node's outgoing links.
	Enabled [][]int

	// ReadsEnabled[n] reports whether enabledness is observed at node n.
	// Weak and strong fairness only look at these nodes. Nil means every node.
	ReadsEnabled []bool

	// Accept[n] is a bitmask of the acceptance sets node n belongs to. An
	// accepting loop visits a node of every one of the NumAcceptSets sets.
	// Bits at or above NumAcceptSets are ignored.
	Accept        []uint64
	NumAcceptSets int

	// LoopAllowed, when non-nil, restricts which nodes a loop may visit.
	// Prefixes may use any node.
	LoopAllowed []bool

	// FailsIfStuck[n] reports whether staying at node n forever violates the
	// property. The engine decides whether staying there is fair: it is, when
	// no weak or strong action is enabled at n. Nil means never.
	FailsIfStuck []bool

	out [][]int
}

// EnabledFromEdges sets Enabled[n] to the distinct actions of n's outgoing
// edges, which is what "enabled" means for a state graph.
func (g *Graph) EnabledFromEdges() {
	g.Enabled = make([][]int, g.NumNodes)
	for _, e := range g.Edges {
		if e.From < 0 || e.From >= g.NumNodes {
			continue
		}
		if !containsInt(g.Enabled[e.From], e.Action) {
			g.Enabled[e.From] = append(g.Enabled[e.From], e.Action)
		}
	}
}

// SetAlwaysEventually configures the graph to check "always eventually q".
// A violation is a fair loop that never visits q, or getting stuck outside q.
func (g *Graph) SetAlwaysEventually(q []bool) {
	g.NumAcceptSets = 0
	g.Accept = nil
	g.LoopAllowed = make([]bool, g.NumNodes)
	g.FailsIfStuck = make([]bool, g.NumNodes)
	for n := 0; n < g.NumNodes; n++ {
		g.LoopAllowed[n] = !q[n]
		g.FailsIfStuck[n] = !q[n]
	}
}

// SetEventuallyAlways configures the graph to check "eventually always q".
// A violation is a fair loop that visits a not-q node infinitely often, or
// getting stuck outside q.
func (g *Graph) SetEventuallyAlways(q []bool) {
	g.NumAcceptSets = 1
	g.LoopAllowed = nil
	g.Accept = make([]uint64, g.NumNodes)
	g.FailsIfStuck = make([]bool, g.NumNodes)
	for n := 0; n < g.NumNodes; n++ {
		if !q[n] {
			g.Accept[n] = 1
		}
		g.FailsIfStuck[n] = !q[n]
	}
}

func (g *Graph) validate() error {
	if g.NumNodes < 0 {
		return fmt.Errorf("negative node count %d", g.NumNodes)
	}
	if g.NumAcceptSets < 0 || g.NumAcceptSets > 64 {
		return fmt.Errorf("acceptance set count %d out of range 0..64", g.NumAcceptSets)
	}
	for a, f := range g.Actions {
		if f > Crash {
			return fmt.Errorf("action %d has unknown fairness %d", a, f)
		}
	}
	for _, n := range g.Init {
		if n < 0 || n >= g.NumNodes {
			return fmt.Errorf("init node %d out of range", n)
		}
	}
	for i, e := range g.Edges {
		if e.From < 0 || e.From >= g.NumNodes || e.To < 0 || e.To >= g.NumNodes {
			return fmt.Errorf("edge %d (%d->%d) out of range", i, e.From, e.To)
		}
		if e.Action < 0 || e.Action >= len(g.Actions) {
			return fmt.Errorf("edge %d has unknown action %d", i, e.Action)
		}
	}
	for name, s := range map[string]int{
		"Enabled":      len(g.Enabled),
		"ReadsEnabled": len(g.ReadsEnabled),
		"Accept":       len(g.Accept),
		"LoopAllowed":  len(g.LoopAllowed),
		"FailsIfStuck": len(g.FailsIfStuck),
	} {
		if s != 0 && s != g.NumNodes {
			return fmt.Errorf("%s has %d entries, want %d", name, s, g.NumNodes)
		}
	}
	for n, as := range g.Enabled {
		for _, a := range as {
			if a < 0 || a >= len(g.Actions) {
				return fmt.Errorf("node %d enables unknown action %d", n, a)
			}
		}
	}
	return nil
}

func (g *Graph) buildOut() {
	g.out = make([][]int, g.NumNodes)
	for i, e := range g.Edges {
		g.out[e.From] = append(g.out[e.From], i)
	}
}

func (g *Graph) readsEnabled(n int) bool {
	return g.ReadsEnabled == nil || g.ReadsEnabled[n]
}

func (g *Graph) loopAllowed(n int) bool {
	return g.LoopAllowed == nil || g.LoopAllowed[n]
}

func (g *Graph) failsIfStuck(n int) bool {
	return g.FailsIfStuck != nil && g.FailsIfStuck[n]
}

func (g *Graph) accept(n int) uint64 {
	if g.Accept == nil {
		return 0
	}
	return g.Accept[n] & g.acceptMask()
}

// acceptMask has one bit set for each acceptance set.
func (g *Graph) acceptMask() uint64 {
	return uint64(1)<<uint(g.NumAcceptSets) - 1
}

func (g *Graph) enabled(n int) []int {
	if g.Enabled == nil {
		return nil
	}
	return g.Enabled[n]
}

func (g *Graph) isFair(a int) bool {
	return g.Actions[a] == Weak || g.Actions[a] == Strong
}

func containsInt(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
