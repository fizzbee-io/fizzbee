package liveness

import "fmt"

// Check returns a counterexample to the property encoded in g, or nil when
// every fair run from an initial node satisfies it.
//
// A stutter counterexample (staying forever at a node whose FailsIfStuck is
// set, where staying is fair) takes precedence over a loop, and the one
// closest to an initial node is returned. Otherwise the loop is found by
// splitting the reachable, loop-allowed nodes into strongly connected
// components and refining each until it is fair and accepting or empty.
func Check(g *Graph) (*Lasso, error) {
	if err := g.validate(); err != nil {
		return nil, err
	}
	g.buildOut()
	c := &checker{g: g, mark: make([]int, g.NumNodes)}
	c.bfs()

	if n := c.stutterNode(); n >= 0 {
		return &Lasso{Prefix: c.prefix(n), Start: n, Stutter: true}, nil
	}

	var nodes []int
	for n := 0; n < g.NumNodes; n++ {
		if c.depth[n] >= 0 && g.loopAllowed(n) {
			nodes = append(nodes, n)
		}
	}
	t := newTarjan(g)
	work := t.components(nodes)
	for len(work) > 0 {
		nodes := work[len(work)-1]
		work = work[:len(work)-1]
		comp := c.analyze(nodes)
		if len(comp.edges) == 0 || comp.accept != c.fullAccept() {
			continue
		}
		if remove := c.unfairNodes(comp); remove != nil {
			if rest := c.without(nodes, remove); len(rest) > 0 {
				work = append(work, t.components(rest)...)
			}
			continue
		}
		entry := nodes[0]
		for _, n := range nodes {
			if c.depth[n] < c.depth[entry] || (c.depth[n] == c.depth[entry] && n < entry) {
				entry = n
			}
		}
		loop, ok := c.stitch(comp, entry)
		if !ok {
			return nil, fmt.Errorf("internal error: cannot stitch a loop through component of node %d", entry)
		}
		return &Lasso{Prefix: c.prefix(entry), Start: entry, Loop: loop}, nil
	}
	return nil, nil
}

type checker struct {
	g      *Graph
	parent []int // BFS tree edge into each node, -1 at roots
	depth  []int // BFS depth, -1 when unreachable

	// mark[n] == stamp means n is in the component being analyzed.
	mark  []int
	stamp int
}

type component struct {
	nodes   []int
	edges   []int // non-crash edges with both ends in the component
	taken   []bool
	readers []int
	accept  uint64
}

func (c *checker) bfs() {
	g := c.g
	c.parent = make([]int, g.NumNodes)
	c.depth = make([]int, g.NumNodes)
	for n := range c.depth {
		c.depth[n], c.parent[n] = -1, -1
	}
	var queue []int
	for _, n := range g.Init {
		if c.depth[n] < 0 {
			c.depth[n] = 0
			queue = append(queue, n)
		}
	}
	for len(queue) > 0 {
		v := queue[0]
		queue = queue[1:]
		for _, e := range g.out[v] {
			w := g.Edges[e].To
			if c.depth[w] < 0 {
				c.depth[w] = c.depth[v] + 1
				c.parent[w] = e
				queue = append(queue, w)
			}
		}
	}
}

func (c *checker) prefix(n int) []int {
	var path []int
	for c.parent[n] >= 0 {
		e := c.parent[n]
		path = append(path, e)
		n = c.g.Edges[e].From
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}

// stuckIsFair reports whether a run may stay at n forever. When enabledness
// is not observed at n nothing forces it to move; otherwise any enabled weak
// or strong action is continuously enabled and never taken.
func (g *Graph) stuckIsFair(n int) bool {
	if !g.readsEnabled(n) {
		return true
	}
	for _, a := range g.enabled(n) {
		if g.isFair(a) {
			return false
		}
	}
	return true
}

func (c *checker) stutterNode() int {
	best := -1
	for n := 0; n < c.g.NumNodes; n++ {
		if c.depth[n] < 0 || !c.g.failsIfStuck(n) || !c.g.stuckIsFair(n) {
			continue
		}
		if best < 0 || c.depth[n] < c.depth[best] {
			best = n
		}
	}
	return best
}

func (c *checker) fullAccept() uint64 {
	return uint64(1)<<uint(c.g.NumAcceptSets) - 1
}

func (c *checker) analyze(nodes []int) *component {
	g := c.g
	c.stamp++
	for _, n := range nodes {
		c.mark[n] = c.stamp
	}
	comp := &component{nodes: nodes, taken: make([]bool, len(g.Actions))}
	for _, n := range nodes {
		comp.accept |= g.accept(n)
		if g.readsEnabled(n) {
			comp.readers = append(comp.readers, n)
		}
		for _, e := range g.out[n] {
			edge := g.Edges[e]
			if g.Actions[edge.Action] != Crash && c.mark[edge.To] == c.stamp {
				comp.edges = append(comp.edges, e)
				comp.taken[edge.Action] = true
			}
		}
	}
	return comp
}

// unfairNodes returns the nodes no fair loop inside comp can visit, or nil
// when the loop through every edge of comp is already fair.
//
// Weak: an action never taken in comp but enabled at all of its readers rules
// out every loop that visits a reader, so only reader-free loops remain.
// Strong: an action never taken in comp rules out every loop through a reader
// where it is enabled.
func (c *checker) unfairNodes(comp *component) []int {
	g := c.g
	for a, f := range g.Actions {
		if f != Weak || comp.taken[a] || len(comp.readers) == 0 {
			continue
		}
		all := true
		for _, n := range comp.readers {
			if !containsInt(g.enabled(n), a) {
				all = false
				break
			}
		}
		if all {
			return comp.readers
		}
	}
	var remove []int
	for _, n := range comp.readers {
		for _, a := range g.enabled(n) {
			if g.Actions[a] == Strong && !comp.taken[a] {
				remove = append(remove, n)
				break
			}
		}
	}
	return remove
}

func (c *checker) without(nodes, remove []int) []int {
	c.stamp++
	for _, n := range remove {
		c.mark[n] = c.stamp
	}
	var rest []int
	for _, n := range nodes {
		if c.mark[n] != c.stamp {
			rest = append(rest, n)
		}
	}
	return rest
}
