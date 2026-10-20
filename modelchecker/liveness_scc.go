package modelchecker

import (
	"fmt"
	"slices"
	"strings"

	ast "fizz/proto"
	"github.com/fizzbee-io/fizzbee/modelchecker/liveness"
)

// sccGraph is the explored state graph converted for the liveness engine.
// Node i of the engine graph is nodes[i], and edge e is links[e].
type sccGraph struct {
	g          *liveness.Graph
	nodes      []*Node
	links      []*Link
	root       int
	index      map[*Node]int
	actionKeys []string // action identities, for diagnostics
}

// isObservationNode reports whether n is a state between actions, where the
// enabledness of actions is observed for fairness.
func isObservationNode(n *Node) bool {
	return n.Name == "init" || n.Name == "yield" || n.Name == "crash"
}

// isFairChoiceNode reports whether n is a fair `any` choice inside an
// action. Its options are strongly fair, so their enabledness is observed
// there, but weak actions' enabledness is not.
func isFairChoiceNode(n *Node) bool {
	if isObservationNode(n) || len(n.Outbound) == 0 || !strings.HasPrefix(n.Outbound[0].Name, "Any:") {
		return false
	}
	f := n.Outbound[0].ChoiceFairness
	return f != ast.FairnessLevel_FAIRNESS_LEVEL_UNKNOWN && f != ast.FairnessLevel_FAIRNESS_LEVEL_UNFAIR
}

func newSccGraph(root *Node, nodes []*Node) *sccGraph {
	index := make(map[*Node]int, len(nodes))
	for i, n := range nodes {
		index[n] = i
	}
	s := &sccGraph{
		nodes: nodes,
		index: index,
		g: &liveness.Graph{
			NumNodes:       len(nodes),
			Enabled:        make([][]int, len(nodes)),
			ReadsEnabled:   make([]bool, len(nodes)),
			WeakUnobserved: make([]bool, len(nodes)),
		},
	}
	if r, ok := index[root]; ok {
		s.root = r
		s.g.Init = []int{r}
	}
	actions := map[string]int{}
	actionOf := func(key string, f liveness.Fairness) int {
		key = fmt.Sprintf("%s#%v", key, f)
		if a, ok := actions[key]; ok {
			return a
		}
		actions[key] = len(s.g.Actions)
		s.g.Actions = append(s.g.Actions, f)
		s.actionKeys = append(s.actionKeys, key)
		return actions[key]
	}

	for i, node := range nodes {
		observes := isObservationNode(node)
		choice := isFairChoiceNode(node)
		s.g.ReadsEnabled[i] = observes || choice
		s.g.WeakUnobserved[i] = choice
		for _, link := range node.Outbound {
			to, ok := index[link.Node]
			if !ok {
				continue
			}
			var a int
			switch {
			case link.IsCrashLink():
				a = actionOf(link.Name, liveness.Crash)
			case observes || choice:
				// Same identity and fairness the cycle finder uses, including
				// strong fairness for fair choices and thread switches.
				name, f, _ := fairnessLinkName(node, link)
				a = actionOf(name, toLivenessFairness(f))
			default:
				// Steps inside an action are neither observed nor counted as
				// taking an observed action.
				a = actionOf("internal|"+link.Name, liveness.Unfair)
			}
			s.g.Edges = append(s.g.Edges, liveness.Edge{From: i, To: to, Action: a})
			s.links = append(s.links, link)
			if !slices.Contains(s.g.Enabled[i], a) {
				s.g.Enabled[i] = append(s.g.Enabled[i], a)
			}
		}
	}
	return s
}

func toLivenessFairness(f ast.FairnessLevel) liveness.Fairness {
	switch f {
	case ast.FairnessLevel_FAIRNESS_LEVEL_STRONG:
		return liveness.Strong
	case ast.FairnessLevel_FAIRNESS_LEVEL_WEAK:
		return liveness.Weak
	}
	return liveness.Unfair
}

// configure sets the engine graph up for one property. q[i] says whether
// nodes[i] satisfies the property's state predicate; staying forever is only
// possible at observation nodes.
func (s *sccGraph) configure(q []bool, eventuallyAlways bool) {
	if eventuallyAlways {
		s.g.SetEventuallyAlways(q)
	} else {
		s.g.SetAlwaysEventually(q)
	}
	for i, n := range s.nodes {
		s.g.FailsIfStuck[i] = isObservationNode(n) && !q[i]
	}
}

// check runs the engine for one property and returns the counterexample in
// the cycle finder's failure path format, or nil.
func (s *sccGraph) check(q []bool, eventuallyAlways bool) ([]*Link, error) {
	s.configure(q, eventuallyAlways)
	l, err := liveness.Check(s.g)
	if err != nil || l == nil {
		return nil, err
	}
	path := []*Link{InitNodeToLink(s.nodes[s.root])}
	for _, e := range l.Prefix {
		path = append(path, s.links[e])
	}
	for _, e := range l.Loop {
		path = append(path, s.links[e])
	}
	if l.Stutter {
		path = append(path, &Link{Node: s.nodes[l.Start], Name: "stutter"})
	}
	return path, nil
}

// livenessOperators returns which supported liveness form an invariant has.
func livenessOperators(invariant *ast.Invariant) (alwaysEventually, eventuallyAlways bool) {
	if invariant.Block == nil {
		if invariant.Always && invariant.Eventually {
			return true, false
		}
		return false, invariant.Eventually && invariant.GetNested().GetAlways()
	}
	ops := invariant.TemporalOperators
	if len(ops) < 2 || !slices.Contains(ops, "eventually") {
		return false, false
	}
	return ops[0] == "always" && ops[1] == "eventually", ops[0] == "eventually" && ops[1] == "always"
}

// strictLivenessPredicate is the state predicate the strict cycle finder
// checks for invariant j of file i. Only nodes between actions, or with no
// running threads, count as satisfying or violating it.
func strictLivenessPredicate(nodes []*Node, i, j int, eventuallyAlways bool) []bool {
	q := make([]bool, len(nodes))
	for k, n := range nodes {
		relevant := n.Process.GetThreadsCount() == 0 || n.Name == "yield"
		witness := n.Process.Witness[i][j]
		if eventuallyAlways {
			q[k] = !relevant || witness
		} else {
			q[k] = relevant && witness
		}
	}
	return q
}

// CheckStrictLivenessScc is CheckStrictLiveness on the SCC-based engine.
func CheckStrictLivenessScc(root *Node, nodes []*Node) ([]*Link, *InvariantPosition, error) {
	process := root.Process
	if len(process.Files) > 1 {
		panic("Invariant checking not supported for multiple files yet")
	}
	s := newSccGraph(root, nodes)
	for i, file := range process.Files {
		for j, invariant := range file.Invariants {
			alwaysEventually, eventuallyAlways := livenessOperators(invariant)
			if !alwaysEventually && !eventuallyAlways {
				continue
			}
			if eventuallyAlways {
				fmt.Println("Checking eventually always", invariant.Name)
			} else {
				fmt.Println("Checking always eventually", invariant.Name)
			}
			path, err := s.check(strictLivenessPredicate(nodes, i, j, eventuallyAlways), eventuallyAlways)
			if err != nil {
				return nil, nil, fmt.Errorf("liveness %s: %w", invariant.Name, err)
			}
			if path != nil {
				return path, NewInvariantPosition(i, j), nil
			}
		}
	}
	return nil, nil, nil
}

// toLasso maps a failure path in the cycle finder's format (an Init link,
// then real links, ending either with a "stutter" link or with a link back
// to a node already on the path) onto the engine graph.
func (s *sccGraph) toLasso(path []*Link) (*liveness.Lasso, error) {
	// The cycle finder rebuilds prefix links as copies, so match each step
	// by its endpoints and name rather than by pointer.
	type step struct {
		from, to int
		name     string
	}
	edgeOf := make(map[step]int, len(s.links))
	for e, l := range s.links {
		k := step{s.g.Edges[e].From, s.g.Edges[e].To, l.Name}
		if _, ok := edgeOf[k]; !ok {
			edgeOf[k] = e
		}
	}
	if len(path) == 0 {
		return nil, fmt.Errorf("empty path")
	}
	stutter := path[len(path)-1].Name == "stutter"
	if stutter {
		path = path[:len(path)-1]
	}
	var edges []int
	nodes := []*Node{path[0].Node}
	for i, l := range path[1:] {
		from, okFrom := s.index[nodes[len(nodes)-1]]
		to, okTo := s.index[l.Node]
		e, ok := edgeOf[step{from, to, l.Name}]
		if !okFrom || !okTo || !ok {
			return nil, fmt.Errorf("step %d (%s) is not a link of the explored graph", i+1, l.Name)
		}
		edges = append(edges, e)
		nodes = append(nodes, l.Node)
	}
	l := &liveness.Lasso{Stutter: stutter}
	last := nodes[len(nodes)-1]
	if stutter {
		l.Prefix, l.Start = edges, slices.Index(s.nodes, last)
		return l, nil
	}
	start := slices.Index(nodes, last)
	if start == len(nodes)-1 {
		return nil, fmt.Errorf("path does not end in a loop")
	}
	l.Prefix, l.Start, l.Loop = edges[:start], slices.Index(s.nodes, last), edges[start:]
	return l, nil
}

// cycleFinderLoop returns l's loop in the form the cycle finder passes to
// isFairCycle: a link into the loop's start node, then the loop's links,
// the last of which returns to that node. The cycle finder's first link is
// the one it arrived by; any link into the start node serves, since
// isFairCycle reads only its target there.
func (s *sccGraph) cycleFinderLoop(l *liveness.Lasso) []*Link {
	loop := []*Link{s.links[l.Loop[len(l.Loop)-1]]}
	for _, e := range l.Loop {
		loop = append(loop, s.links[e])
	}
	return loop
}

// CheckStrictLivenessShadow runs the cycle finder and the SCC engine on
// every liveness property, prints how they compare, and returns the cycle
// finder's result. Each counterexample either one reports is checked against
// the fairness definition on the same graph, so a disagreement shows which
// side is wrong.
func CheckStrictLivenessShadow(root *Node, nodes []*Node) ([]*Link, *InvariantPosition) {
	process := root.Process
	if len(process.Files) > 1 {
		panic("Invariant checking not supported for multiple files yet")
	}
	s := newSccGraph(root, nodes)
	var failurePath []*Link
	var failed *InvariantPosition
	verdict := func(path []*Link) string {
		if path == nil {
			return "PASS"
		}
		l, err := s.toLasso(path)
		if err == nil {
			err = liveness.ValidateLasso(s.g, l)
		}
		if err != nil {
			return "FAIL (counterexample invalid: " + err.Error() + ")"
		}
		if l.Stutter {
			return "FAIL (counterexample valid, stutter)"
		}
		// Also judge the loop by the cycle finder's own fairness check, so a
		// disagreement separates a search miss from a definition mismatch.
		if fair, _ := isFairCycle(s.cycleFinderLoop(l), false); !fair {
			return "FAIL (counterexample valid, but unfair by the cycle finder's check)"
		}
		return "FAIL (counterexample valid)"
	}
	for i, file := range process.Files {
		for j, invariant := range file.Invariants {
			alwaysEventually, eventuallyAlways := livenessOperators(invariant)
			if !alwaysEventually && !eventuallyAlways {
				continue
			}
			predicate := func(n *Node) (bool, bool) {
				return n.Process.GetThreadsCount() == 0 || n.Name == "yield", n.Process.Witness[i][j]
			}
			q := strictLivenessPredicate(nodes, i, j, eventuallyAlways)
			var oldPath []*Link
			var live bool
			if eventuallyAlways {
				oldPath, live = EventuallyAlwaysFinal(nodes, root, predicate)
			} else {
				oldPath, live = AlwaysEventuallyFinal(nodes, root, predicate)
			}
			if live {
				oldPath = nil
			}
			newPath, err := s.check(q, eventuallyAlways)
			newVerdict := verdict(newPath)
			if err != nil {
				newVerdict = "ERROR " + err.Error()
			}
			oldVerdict := verdict(oldPath)
			tag := "agree"
			if (oldPath == nil) != (newPath == nil) || err != nil {
				tag = "DISAGREE"
			}
			fmt.Printf("Liveness shadow %s %s: cycle finder %s, scc %s\n", tag, invariant.Name, oldVerdict, newVerdict)
			if oldPath != nil && failed == nil {
				failurePath, failed = oldPath, NewInvariantPosition(i, j)
			}
		}
	}
	return failurePath, failed
}
