package liveness

// tarjan finds strongly connected components of subgraphs of a Graph,
// iteratively so deep graphs cannot overflow the goroutine stack. Only
// non-crash edges between nodes of the current subgraph are followed.
type tarjan struct {
	g       *Graph
	index   []int
	low     []int
	onStack []bool
	inSet   []int
	stamp   int
}

func newTarjan(g *Graph) *tarjan {
	return &tarjan{
		g:       g,
		index:   make([]int, g.NumNodes),
		low:     make([]int, g.NumNodes),
		onStack: make([]bool, g.NumNodes),
		inSet:   make([]int, g.NumNodes),
	}
}

func (t *tarjan) followable(edge int) bool {
	e := t.g.Edges[edge]
	return t.g.Actions[e.Action] != Crash && t.inSet[e.To] == t.stamp
}

// components returns the SCCs of the subgraph induced by nodes.
func (t *tarjan) components(nodes []int) [][]int {
	t.stamp++
	for _, n := range nodes {
		t.inSet[n] = t.stamp
		t.index[n] = -1
		t.onStack[n] = false
	}
	type frame struct{ node, next int }
	var (
		comps   [][]int
		stack   []int
		call    []frame
		counter int
	)
	visit := func(n int) {
		t.index[n], t.low[n] = counter, counter
		counter++
		stack = append(stack, n)
		t.onStack[n] = true
		call = append(call, frame{node: n})
	}
	for _, root := range nodes {
		if t.index[root] != -1 {
			continue
		}
		visit(root)
		for len(call) > 0 {
			top := len(call) - 1
			v := call[top].node
			out := t.g.out[v]
			if call[top].next < len(out) {
				edge := out[call[top].next]
				call[top].next++
				if !t.followable(edge) {
					continue
				}
				w := t.g.Edges[edge].To
				if t.index[w] == -1 {
					visit(w)
				} else if t.onStack[w] && t.index[w] < t.low[v] {
					t.low[v] = t.index[w]
				}
				continue
			}
			call = call[:top]
			if top > 0 {
				p := call[top-1].node
				if t.low[v] < t.low[p] {
					t.low[p] = t.low[v]
				}
			}
			if t.low[v] == t.index[v] {
				var comp []int
				for {
					w := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					t.onStack[w] = false
					comp = append(comp, w)
					if w == v {
						break
					}
				}
				comps = append(comps, comp)
			}
		}
	}
	return comps
}
