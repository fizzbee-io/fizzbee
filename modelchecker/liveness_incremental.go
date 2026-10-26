package modelchecker

import (
	"fmt"

	ast "fizz/proto"
)

// Incremental liveness checks the liveness assertions on the part of the
// graph explored so far, each time the visited count doubles, so a violation
// is reported without waiting for the whole state space.
//
// Only closed nodes take part: nodes whose links are all known. A yield node
// is open while any action it scheduled is still queued, since that action's
// link is added only once the action runs. A link from a closed node to an
// open one is not an edge of the partial graph, but its action still counts
// as enabled for fairness. Every edge, enabled action and state label among
// closed nodes is then final, so a fair lasso through closed nodes is a
// counterexample in the full graph too. Formulas with possibly are left to
// the final check, since a later state can make a possibly true.
//
// This relies on how Start's default queue adds links. A node's links are
// added either while the node itself is processed (steps inside an action
// run inline in expandToYield) or when a queued action it scheduled runs;
// enqueueScheduled counts those, and Start marks each one done after
// expanding it. Crash nodes are the exception: when a later state turns out
// to be a duplicate of one, processNode renames it (to yield, or to the
// action's name), which changes how it is labeled, and may add a crash link
// to it. So crash nodes always count as open. An explorer that adds links in
// any other way, such as startProcessedQueue or simulation, must not run
// these checks without its own tracking; main.go refuses those modes, and
// the dfs and random strategies too, since there a closed node may only be
// reachable through open ones.

// incrementalLivenessFirstCheck is the visited count of the first partial
// check. Later checks run each time the count doubles.
const incrementalLivenessFirstCheck = 1000

// SetIncrementalLiveness turns incremental liveness checking on. It applies
// to the default exhaustive search only.
func (p *Processor) SetIncrementalLiveness(v bool) {
	p.incrementalLiveness = v
	p.nextIncrementalCheck = incrementalLivenessFirstCheck
}

// IncrementalLivenessFailure returns the counterexample a partial check
// found, if any. Exploration stops when one is found.
func (p *Processor) IncrementalLivenessFailure() ([]*Link, *InvariantPosition) {
	return p.incrementalFailurePath, p.incrementalFailedInvariant
}

// IncrementalLivenessError returns the error a partial check hit, if any.
func (p *Processor) IncrementalLivenessError() error {
	return p.incrementalErr
}

// trackScheduled records that node's parent has one more queued action.
func (p *Processor) trackScheduled(node *Node) {
	parent := p.scheduledBy(node)
	if parent == nil {
		return
	}
	if p.pendingActions == nil {
		p.pendingActions = make(map[*Node]int)
	}
	p.pendingActions[parent]++
}

// scheduledBy returns the node that scheduled a queued node, if tracked.
// Read it before processing the queued node: a node that turns out to be a
// duplicate loses its inbound links.
func (p *Processor) scheduledBy(node *Node) *Node {
	if !p.incrementalLiveness || len(node.Inbound) == 0 {
		return nil
	}
	return node.Inbound[0].Node
}

// trackDone records that an action parent scheduled has been taken off the
// queue and fully processed, whatever the outcome.
func (p *Processor) trackDone(parent *Node) {
	if parent == nil {
		return
	}
	if p.pendingActions[parent] <= 1 {
		delete(p.pendingActions, parent)
	} else {
		p.pendingActions[parent]--
	}
}

// isOpen reports whether some links or the labels of node are not final
// yet. A crash node can still be renamed by a later duplicate.
func (p *Processor) isOpen(node *Node) bool {
	return p.pendingActions[node] > 0 || node.Name == "crash"
}

// maybeCheckLivenessIncrementally runs a partial check when the visited
// count has reached the next threshold. It reports whether exploration
// should stop.
func (p *Processor) maybeCheckLivenessIncrementally() bool {
	if !p.incrementalLiveness || len(p.visited) < p.nextIncrementalCheck {
		return false
	}
	if p.incrementalEvery > 0 {
		p.nextIncrementalCheck = len(p.visited) + p.incrementalEvery - 1
	} else {
		for p.nextIncrementalCheck <= len(p.visited) {
			p.nextIncrementalCheck *= 2
		}
	}
	if p.partialCheckHook == nil && !hasIncrementalAssertions(p.Files) {
		p.incrementalLiveness = false
		return false
	}
	s := p.partialSccGraph()
	if s == nil {
		return false
	}
	if p.partialCheckHook != nil {
		p.partialCheckHook(s)
		return false
	}
	if !p.isTest {
		fmt.Printf("Checking liveness on %d closed nodes of %d\n", len(s.nodes), len(p.visited))
	}
	path, pos, err := s.checkStrictLiveness(p.Files, hasPossibly)
	if err != nil {
		p.incrementalErr = err
		return true
	}
	if pos != nil {
		p.incrementalFailurePath, p.incrementalFailedInvariant = path, pos
		return true
	}
	return false
}

// partialSccGraph is the graph of the closed nodes reachable from Init
// through closed nodes, or nil when there is none yet.
func (p *Processor) partialSccGraph() *sccGraph {
	if p.Init == nil || p.isOpen(p.Init) || len(p.Files) > 1 {
		return nil
	}
	nodes := p.closedNodes()
	if len(nodes) == 0 {
		return nil
	}
	s := newPartialSccGraph(p.Init, nodes, p.isOpen)
	s.quiet = true
	return s
}

// closedNodes lists the nodes GetAllNodes would list, restricted to the
// closed ones reachable from Init without passing an open node. Unlike
// GetAllNodes it leaves the graph as it is.
func (p *Processor) closedNodes() []*Node {
	seen := map[*Node]bool{p.Init: true}
	queue := []*Node{p.Init}
	var result []*Node
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		if node.Process != nil && !node.Process.Enabled {
			continue
		}
		result = append(result, node)
		for _, link := range node.Outbound {
			next := link.Node
			if !next.Enabled || seen[next] || p.isOpen(next) {
				continue
			}
			seen[next] = true
			queue = append(queue, next)
		}
	}
	return result
}

// hasPossibly reports whether invariant is a formula that uses possibly.
func hasPossibly(invariant *ast.Invariant) bool {
	if invariant.Formula == nil {
		return false
	}
	occurrences, _ := possiblyOccurrences(invariant.Formula)
	return len(occurrences) > 0
}

// hasIncrementalAssertions reports whether files have an assertion a
// partial check would check.
func hasIncrementalAssertions(files []*ast.File) bool {
	for _, file := range files {
		for _, invariant := range file.Invariants {
			if invariant.Formula != nil && !hasPossibly(invariant) {
				return true
			}
			if alwaysEventually, eventuallyAlways := livenessOperators(invariant); alwaysEventually || eventuallyAlways {
				return true
			}
		}
	}
	return false
}
