package liveness_test

import (
	"github.com/fizzbee-io/fizzbee/modelchecker/liveness"
)

// oracle decides whether g has a counterexample by brute force, straight from
// the definitions and without sharing code with the engine. A loop that
// repeats forever corresponds to a strongly connected set of edges: every
// closed walk uses one, and every strongly connected edge set has a closed
// walk through all of its edges. Whether the loop is accepting and fair
// depends only on which nodes it visits and which actions it takes, so trying
// every subset of edges is exact. Only usable on graphs with few edges.
func oracle(g *liveness.Graph) bool {
	reach := make([]bool, g.NumNodes)
	var queue []int
	for _, n := range g.Init {
		if !reach[n] {
			reach[n] = true
			queue = append(queue, n)
		}
	}
	for len(queue) > 0 {
		v := queue[0]
		queue = queue[1:]
		for _, e := range g.Edges {
			if e.From == v && !reach[e.To] {
				reach[e.To] = true
				queue = append(queue, e.To)
			}
		}
	}

	reads := func(n, a int) bool {
		if g.ReadsEnabled != nil && !g.ReadsEnabled[n] {
			return false
		}
		weakHidden := g.WeakUnobserved != nil && g.WeakUnobserved[n]
		return !(g.Actions[a] == liveness.Weak && weakHidden)
	}
	enables := func(n, a int) bool {
		if g.Enabled == nil {
			return false
		}
		for _, b := range g.Enabled[n] {
			if a == b {
				return true
			}
		}
		return false
	}
	fair := func(a int) bool {
		return g.Actions[a] == liveness.Weak || g.Actions[a] == liveness.Strong
	}

	// Staying forever at n: weak and strong actions enabled at n are
	// continuously enabled and never taken, which only counts where n
	// observes them.
	for n := 0; n < g.NumNodes; n++ {
		if !reach[n] || g.FailsIfStuck == nil || !g.FailsIfStuck[n] {
			continue
		}
		stuckFair := true
		for a := range g.Actions {
			if fair(a) && enables(n, a) && reads(n, a) {
				stuckFair = false
			}
		}
		if stuckFair {
			return true
		}
	}

	var cand []liveness.Edge
	for _, e := range g.Edges {
		if g.Actions[e.Action] == liveness.Crash || !reach[e.From] {
			continue
		}
		if g.LoopAllowed != nil && (!g.LoopAllowed[e.From] || !g.LoopAllowed[e.To]) {
			continue
		}
		cand = append(cand, e)
	}
	if len(cand) > 20 {
		panic("oracle: too many edges")
	}
	for mask := 1; mask < 1<<len(cand); mask++ {
		var edges []liveness.Edge
		in := make([]bool, g.NumNodes)
		for i, e := range cand {
			if mask&(1<<i) != 0 {
				edges = append(edges, e)
				in[e.From], in[e.To] = true, true
			}
		}
		if !stronglyConnected(in, edges) {
			continue
		}
		var nodes []int
		for n, ok := range in {
			if ok {
				nodes = append(nodes, n)
			}
		}

		accepting := true
		for i := 0; i < g.NumAcceptSets; i++ {
			hit := false
			for _, n := range nodes {
				if g.Accept != nil && g.Accept[n]&(1<<uint(i)) != 0 {
					hit = true
				}
			}
			if !hit {
				accepting = false
			}
		}
		if !accepting {
			continue
		}

		taken := make([]bool, len(g.Actions))
		for _, e := range edges {
			taken[e.Action] = true
		}
		isFair := true
		for a := range g.Actions {
			if taken[a] || !fair(a) {
				continue
			}
			enabledSomewhere, disabledSomewhere := false, false
			for _, n := range nodes {
				if !reads(n, a) {
					continue
				}
				if enables(n, a) {
					enabledSomewhere = true
				} else {
					disabledSomewhere = true
				}
			}
			// Weak: enabled at every observation from some point on, never
			// taken. A loop with no observations constrains nothing.
			if g.Actions[a] == liveness.Weak && enabledSomewhere && !disabledSomewhere {
				isFair = false
			}
			// Strong: enabled at infinitely many observations, never taken.
			if g.Actions[a] == liveness.Strong && enabledSomewhere {
				isFair = false
			}
		}
		if isFair {
			return true
		}
	}
	return false
}

func stronglyConnected(in []bool, edges []liveness.Edge) bool {
	start := -1
	for n, ok := range in {
		if ok {
			start = n
			break
		}
	}
	reaches := func(forward bool) bool {
		seen := make([]bool, len(in))
		seen[start] = true
		for changed := true; changed; {
			changed = false
			for _, e := range edges {
				from, to := e.From, e.To
				if !forward {
					from, to = to, from
				}
				if seen[from] && !seen[to] {
					seen[to] = true
					changed = true
				}
			}
		}
		for n, ok := range in {
			if ok && !seen[n] {
				return false
			}
		}
		return true
	}
	return reaches(true) && reaches(false)
}
