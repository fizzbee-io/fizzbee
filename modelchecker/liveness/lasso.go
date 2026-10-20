package liveness

import (
	"errors"
	"fmt"
)

// Lasso is a counterexample: Prefix leads from an initial node to Start, and
// the run then follows Loop forever. Prefix and Loop hold edge indices. When
// Stutter is set, Loop is empty and the run stays at Start forever instead.
type Lasso struct {
	Prefix  []int
	Start   int
	Loop    []int
	Stutter bool
}

// stitch builds a loop from entry back to entry inside comp that visits
// every acceptance set and meets every fairness obligation. comp must be the
// component analyze last marked, and must have passed unfairNodes.
func (c *checker) stitch(comp *component, entry int) ([]int, bool) {
	g := c.g
	var (
		loop    []int
		cur     = entry
		visited = map[int]bool{entry: true}
		taken   = map[int]bool{}
	)
	follow := func(path []int, ok bool) bool {
		if !ok {
			return false
		}
		for _, e := range path {
			loop = append(loop, e)
			taken[g.Edges[e].Action] = true
			cur = g.Edges[e].To
			visited[cur] = true
		}
		return true
	}
	visitedAny := func(pred func(int) bool) bool {
		for n := range visited {
			if pred(n) {
				return true
			}
		}
		return false
	}

	for i := 0; i < g.NumAcceptSets; i++ {
		bit := uint64(1) << uint(i)
		in := func(n int) bool { return g.accept(n)&bit != 0 }
		if !visitedAny(in) && !follow(c.pathToNode(cur, in)) {
			return nil, false
		}
	}

	for a, f := range g.Actions {
		if f != Weak && f != Strong {
			continue
		}
		enabledAtReader := false
		for _, n := range comp.readers {
			if containsInt(g.enabled(n), a) {
				enabledAtReader = true
				break
			}
		}
		if !enabledAtReader || taken[a] {
			continue
		}
		if comp.taken[a] {
			if !follow(c.pathToEdge(cur, func(e int) bool { return g.Edges[e].Action == a })) {
				return nil, false
			}
			continue
		}
		// Only a weak action can get here untaken: unfairNodes removed every
		// reader enabling an untaken strong action. Visiting a reader where
		// it is disabled breaks its continuous enabledness.
		disabled := func(n int) bool { return g.readsEnabled(n) && !containsInt(g.enabled(n), a) }
		if !visitedAny(disabled) && !follow(c.pathToNode(cur, disabled)) {
			return nil, false
		}
	}

	if len(loop) == 0 && !follow(c.pathToEdge(cur, func(int) bool { return true })) {
		return nil, false
	}
	if !follow(c.pathToNode(cur, func(n int) bool { return n == entry })) {
		return nil, false
	}
	return loop, true
}

// pathToEdge returns the shortest path from 'from' inside the marked
// component whose last edge satisfies pred.
func (c *checker) pathToEdge(from int, pred func(int) bool) ([]int, bool) {
	g := c.g
	prev := map[int]int{from: -1}
	queue := []int{from}
	for len(queue) > 0 {
		v := queue[0]
		queue = queue[1:]
		for _, e := range g.out[v] {
			edge := g.Edges[e]
			if g.Actions[edge.Action] == Crash || c.mark[edge.To] != c.stamp {
				continue
			}
			if pred(e) {
				path := []int{e}
				for n := v; prev[n] >= 0; n = g.Edges[prev[n]].From {
					path = append(path, prev[n])
				}
				for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
					path[i], path[j] = path[j], path[i]
				}
				return path, true
			}
			if _, seen := prev[edge.To]; !seen {
				prev[edge.To] = e
				queue = append(queue, edge.To)
			}
		}
	}
	return nil, false
}

func (c *checker) pathToNode(from int, pred func(int) bool) ([]int, bool) {
	if pred(from) {
		return nil, true
	}
	return c.pathToEdge(from, func(e int) bool { return pred(c.g.Edges[e].To) })
}

// ValidateLasso checks l against the definition of a counterexample directly,
// without the component refinement Check uses, so tests can confirm every
// lasso Check returns.
func ValidateLasso(g *Graph, l *Lasso) error {
	if err := g.validate(); err != nil {
		return err
	}
	if l == nil {
		return errors.New("nil lasso")
	}
	if l.Start < 0 || l.Start >= g.NumNodes {
		return fmt.Errorf("start node %d out of range", l.Start)
	}
	end, err := walk(g, l.Prefix, -1)
	if err != nil {
		return fmt.Errorf("prefix: %w", err)
	}
	if len(l.Prefix) == 0 {
		end = l.Start
		if !containsInt(g.Init, l.Start) {
			return fmt.Errorf("empty prefix but start node %d is not initial", l.Start)
		}
	} else if !containsInt(g.Init, g.Edges[l.Prefix[0]].From) {
		return fmt.Errorf("prefix starts at non-initial node %d", g.Edges[l.Prefix[0]].From)
	}
	if end != l.Start {
		return fmt.Errorf("prefix ends at %d, not start node %d", end, l.Start)
	}

	if l.Stutter {
		if len(l.Loop) != 0 {
			return errors.New("stutter lasso has a loop")
		}
		if !g.failsIfStuck(l.Start) {
			return fmt.Errorf("staying at node %d does not violate the property", l.Start)
		}
		if !g.stuckIsFair(l.Start) {
			return fmt.Errorf("staying at node %d is unfair", l.Start)
		}
		return nil
	}

	if len(l.Loop) == 0 {
		return errors.New("empty loop")
	}
	end, err = walk(g, l.Loop, l.Start)
	if err != nil {
		return fmt.Errorf("loop: %w", err)
	}
	if end != l.Start {
		return fmt.Errorf("loop ends at %d, not start node %d", end, l.Start)
	}
	nodes := []int{l.Start}
	for _, e := range l.Loop {
		if g.Actions[g.Edges[e].Action] == Crash {
			return fmt.Errorf("loop uses crash edge %d", e)
		}
		nodes = append(nodes, g.Edges[e].To)
	}
	var accept uint64
	for _, n := range nodes {
		if !g.loopAllowed(n) {
			return fmt.Errorf("loop visits node %d, which loops may not visit", n)
		}
		accept |= g.accept(n)
	}
	if want := g.acceptMask(); accept != want {
		return fmt.Errorf("loop visits acceptance sets %b, want %b", accept, want)
	}
	return fairLoop(g, nodes, l.Loop)
}

// walk follows path from 'from' (or from the first edge's source when from is
// -1) and returns the node it ends at.
func walk(g *Graph, path []int, from int) (int, error) {
	cur := from
	for i, e := range path {
		if e < 0 || e >= len(g.Edges) {
			return 0, fmt.Errorf("edge %d out of range", e)
		}
		if i == 0 && cur < 0 {
			cur = g.Edges[e].From
		}
		if g.Edges[e].From != cur {
			return 0, fmt.Errorf("edge %d starts at %d, not %d", e, g.Edges[e].From, cur)
		}
		cur = g.Edges[e].To
	}
	return cur, nil
}

// fairLoop applies the fairness definitions to a loop visiting nodes and
// taking edges forever. Enabledness is only observed at readers. Weak: an
// action enabled at every reader the loop visits must be taken. Strong: an
// action enabled at any reader the loop visits must be taken.
func fairLoop(g *Graph, nodes, edges []int) error {
	taken := map[int]bool{}
	for _, e := range edges {
		taken[g.Edges[e].Action] = true
	}
	var readers []int
	for _, n := range nodes {
		if g.readsEnabled(n) && !containsInt(readers, n) {
			readers = append(readers, n)
		}
	}
	for a, f := range g.Actions {
		if taken[a] || len(readers) == 0 {
			continue
		}
		count := 0
		for _, n := range readers {
			if containsInt(g.enabled(n), a) {
				count++
			}
		}
		switch {
		case f == Weak && count == len(readers):
			return fmt.Errorf("weak action %d is enabled at every reader of the loop but never taken", a)
		case f == Strong && count > 0:
			return fmt.Errorf("strong action %d is enabled in the loop but never taken", a)
		}
	}
	return nil
}
