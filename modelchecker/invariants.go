package modelchecker

import (
	ast "fizz/proto"
	"fmt"
	"github.com/fizzbee-io/fizzbee/lib"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"maps"
	"slices"
	"strings"
)

type InvariantPosition struct {
	FileIndex      int
	InvariantIndex int
}

func NewInvariantPosition(fileIndex, invariantIndex int) *InvariantPosition {
	return &InvariantPosition{
		FileIndex:      fileIndex,
		InvariantIndex: invariantIndex,
	}
}
func CheckTransitionInvariants(process *Process) map[int][]int {
	if process.Parent == nil {
		return nil
	}
	if process.Heap.CachedHashCode == process.Parent.Heap.CachedHashCode && process.Parent.Heap.CachedHashCode != "" {
		return nil
	}
	results := make(map[int][]int)
	for i, file := range process.Files {
		results[i] = make([]int, 0)
		for j, invariant := range file.Invariants {
			if invariant.Block != nil && slices.Contains(invariant.TemporalOperators, "transition") {
				passed := CheckTransitionAssertion(process, invariant, j)
				if !passed {
					results[i] = append(results[i], j)
				}
			}
		}
	}
	return results
}
func CheckInvariants(process *Process) map[int][]int {
	if len(process.Files) > 1 {
		panic("Invariant checking not supported for multiple files")
	}
	results := make(map[int][]int)
	for i, file := range process.Files {
		results[i] = make([]int, 0)
		for j, invariant := range file.Invariants {
			passed := false
			if invariant.Block == nil {
				passed = CheckInvariant(process, invariant)
				if invariant.Eventually && passed /*&& (len(process.Threads) == 0 || process.Name == "yield")*/ {
					process.Witness[i][j] = true
				} else if !invariant.Eventually && !passed {
					results[i] = append(results[i], j)
				}
			} else {
				if slices.Contains(invariant.TemporalOperators, "transition") {
					continue
				}
				passed = CheckAssertion(process, invariant, j)
				if (slices.Contains(invariant.TemporalOperators, "eventually") || slices.Contains(invariant.TemporalOperators, "exists")) && passed /*&& (len(process.Threads) == 0 || process.Name == "yield")*/ {
					process.Witness[i][j] = true
				} else if !(slices.Contains(invariant.TemporalOperators, "eventually") || slices.Contains(invariant.TemporalOperators, "exists")) && !passed {
					results[i] = append(results[i], j)
				}
			}
		}
	}
	return results
}

func CheckInvariant(process *Process, invariant *ast.Invariant) bool {
	eventuallyAlways := invariant.Eventually && invariant.GetNested().GetAlways()
	if !invariant.Always && !(eventuallyAlways) {
		panic("Invariant checking not supported for non-always invariants")
	}
	if !eventuallyAlways && invariant.Nested != nil {
		panic("Invariant checking not supported for nested invariants")
	}
	pyExpr := invariant.PyExpr
	if eventuallyAlways && invariant.Nested != nil {
		pyExpr = invariant.Nested.PyExpr
	}
	ref := make(map[starlark.Value]starlark.Value)
	vars := CloneDict(process.Heap.state, ref, nil, 0)
	vars["__returns__"] = NewDictFromStringDict(process.Returns)
	cond, err := process.Evaluator.EvalPyExpr(process.Files[0].GetSourceInfo().GetFileName(), pyExpr, vars)
	PanicOnError(err)
	return bool(cond.Truth())
}

func CheckAssertion(process *Process, invariant *ast.Invariant, index int) bool {
	if !slices.Contains(invariant.TemporalOperators, "always") && !slices.Contains(invariant.TemporalOperators, "exists") {
		panic("Invariant checking supported only for always/always-eventually/eventually-always/exists invariants" + strings.Join(invariant.TemporalOperators, ","))
	}
	cloned := process.CloneForAssert(nil, 0)
	cloned.Heap.state["__returns__"] = NewDictFromStringDict(cloned.Returns)

	return execAssertionFunction(cloned, index, invariant)

}

func execAssertionFunction(cloned *Process, index int, invariant *ast.Invariant) bool {
	numThreads := cloned.GetThreadsCount()
	assertThread := cloned.NewThread()

	assertThread.currentFrame().pc = fmt.Sprintf("Invariants[%d]", index)
	assertThread.currentFrame().Name = invariant.Name
	for {
		forks, _ := assertThread.Execute()
		if cloned.GetThreadsCount() <= numThreads {
			return bool(cloned.Returns[invariant.Name].Truth())
		}
		if len(forks) > 0 {
			panic("Assertions should not include non-deterministic behavior")
		}

	}
}

func CheckTransitionAssertion(process *Process, invariant *ast.Invariant, index int) bool {
	if process.Parent == nil {
		return true
	}
	beforeParam, afterParam := "before", "after"
	if len(invariant.Params) == 2 {
		beforeParam, afterParam = invariant.Params[0].GetName(), invariant.Params[1].GetName()
	} else if len(invariant.Params) != 0 {
		panic("Invariant should not have params or exactly 2 params")
	}
	cloned := process.CloneForAssert(nil, 0)
	cloned.Heap.state[afterParam] = starlarkstruct.FromStringDict(starlark.String(afterParam), cloned.Heap.state)
	cloned.Heap.state[beforeParam] = starlarkstruct.FromStringDict(starlark.String(beforeParam), process.Parent.Heap.state)

	return execAssertionFunction(cloned, index, invariant)
}

func CheckSimpleExistsWitness(nodes []*Node) []*InvariantPosition {
	process := nodes[0].Process
	if len(process.Files) > 1 {
		panic("Invariant checking not supported for multiple files yet")
	}
	existsInvariantPositions := make([]*InvariantPosition, 0)
	for i, file := range process.Files {
		for j, invariant := range file.Invariants {
			if invariant.Block != nil && slices.Contains(invariant.TemporalOperators, "exists") {
				existsInvariantPositions = append(existsInvariantPositions, NewInvariantPosition(i, j))
			}
		}
	}
	satisfiedInvariants := make([]int, 0)
	for _, node := range nodes {
		if len(existsInvariantPositions) == 0 {
			break
		}
		for j, position := range existsInvariantPositions {
			// If the node has witness in this position, then the invariant is satisfied
			if node.Process.Witness[position.FileIndex][position.InvariantIndex] {
				satisfiedInvariants = append(satisfiedInvariants, j)
			}
		}
		// remove the satisfied invariants
		for _, index := range satisfiedInvariants {
			existsInvariantPositions = slices.Delete(existsInvariantPositions, index, index+1)
		}
	}
	return existsInvariantPositions
}

func CheckStrictLiveness(node *Node) ([]*Link, *InvariantPosition) {
	process := node.Process
	if len(process.Files) > 1 {
		panic("Invariant checking not supported for multiple files yet")
	}
	for i, file := range process.Files {
		for j, invariant := range file.Invariants {
			predicate := func(n *Node) (bool, bool) {
				return n.Process.GetThreadsCount() == 0 || n.Name == "yield", n.Process.Witness[i][j]
			}
			eventuallyAlways := false
			alwaysEventually := false
			if invariant.Block == nil {
				if invariant.Always && invariant.Eventually {
					alwaysEventually = true
				} else if invariant.Eventually && invariant.GetNested().GetAlways() {
					eventuallyAlways = true
				}
			} else {
				if slices.Contains(invariant.TemporalOperators, "eventually") &&
					invariant.TemporalOperators[0] == "eventually" && invariant.TemporalOperators[1] == "always" {
					eventuallyAlways = true
				} else if slices.Contains(invariant.TemporalOperators, "eventually") &&
					invariant.TemporalOperators[0] == "always" && invariant.TemporalOperators[1] == "eventually" {
					alwaysEventually = true
				}
			}
			if eventuallyAlways {
				fmt.Println("Checking eventually always", invariant.Name)
				failurePath, isLive := EventuallyAlwaysFinal(node, predicate)
				if !isLive {
					return failurePath, NewInvariantPosition(i, j)
				}
			} else if alwaysEventually {
				fmt.Println("Checking always eventually", invariant.Name)
				// Always Eventually
				failurePath, isLive := AlwaysEventuallyFinal(node, predicate)
				if !isLive {
					return failurePath, NewInvariantPosition(i, j)
				}
			}
		}

	}
	return nil, nil
}

func CheckFastLiveness(allNodes []*Node) ([]*Link, *InvariantPosition) {
	fmt.Println("Checking strict liveness with nondeterministic checker")
	node := allNodes[0]
	process := node.Process
	if len(process.Files) > 1 {
		panic("Invariant checking not supported for multiple files yet")
	}
	for i, file := range process.Files {
		for j, invariant := range file.Invariants {
			predicate := func(n *Node) (bool, bool) {
				return n.Process.GetThreadsCount() == 0, n.Process.Witness[i][j]
			}
			eventuallyAlways := false
			alwaysEventually := false
			if invariant.Block == nil {
				if invariant.Always && invariant.Eventually {
					alwaysEventually = true
				} else if invariant.Eventually && invariant.GetNested().GetAlways() {
					eventuallyAlways = true
				}
			} else {
				if slices.Contains(invariant.TemporalOperators, "eventually") &&
					invariant.TemporalOperators[0] == "eventually" && invariant.TemporalOperators[1] == "always" {
					eventuallyAlways = true
				} else if slices.Contains(invariant.TemporalOperators, "eventually") &&
					invariant.TemporalOperators[0] == "always" && invariant.TemporalOperators[1] == "eventually" {
					alwaysEventually = true
				}
			}
			if eventuallyAlways {
				fmt.Println("Checking eventually always", invariant.Name)
				failurePath, isLive := EventuallyAlwaysFast(allNodes, predicate)
				if !isLive {
					return failurePath, NewInvariantPosition(i, j)
				}
			} else if alwaysEventually {
				fmt.Println("Checking always eventually", invariant.Name)
				// Always Eventually
				failurePath, isLive := AlwaysEventuallyFast(allNodes, predicate)
				if !isLive {
					return failurePath, NewInvariantPosition(i, j)
				}
			}
		}

	}
	return nil, nil
}

func AlwaysEventuallyFast(nodes []*Node, predicate Predicate) ([]*Link, bool) {
	// For strong fairness.
	// For each good node, walk up the Strongly Fair inbound links, and mark them good as well. Eventually, you will
	// end up with nodes that cannot reach a good node either because of a cycle or because of stuttering

	falseNodes := make(map[*Node]bool)
	visited := make(map[*Node]bool)
	queue := lib.NewQueue[*Node]()
	for _, node := range nodes {
		relevant, value := predicate(node)
		if relevant && value {
			queue.Enqueue(node)
		} else {
			falseNodes[node] = true
		}
	}
	for queue.Count() > 0 {
		node, _ := queue.Dequeue()
		if visited[node] {
			continue
		}
		visited[node] = true
		for _, link := range node.Inbound {
			if visited[link.Node] || link.Node == node ||
				(link.Fairness != ast.FairnessLevel_FAIRNESS_LEVEL_STRONG && link.Fairness != ast.FairnessLevel_FAIRNESS_LEVEL_WEAK) {
				continue
			}
			delete(falseNodes, link.Node)
			queue.Enqueue(link.Node)
		}
	}
	if len(falseNodes) > 0 {
		var closestDeadNode *Node

		for node, _ := range falseNodes {
			//fmt.Println("-\n",node.String(), count)
			if closestDeadNode == nil || (closestDeadNode.GetThreadsCount() > 0 && node.GetThreadsCount() == 0) {
				closestDeadNode = node
				continue
			}
			if node.actionDepth > closestDeadNode.actionDepth {
				continue
			} else if node.actionDepth < closestDeadNode.actionDepth {
				closestDeadNode = node
			} else if node.forkDepth < closestDeadNode.forkDepth {
				closestDeadNode = node
			}
		}
		//fmt.Println("Closest dead node:", closestDeadNode.String())
		failurePath := pathToInit(nodes, closestDeadNode)
		path := findCyclePath(closestDeadNode, falseNodes)
		path = append(failurePath, path...)
		return path, false
	} else {
		fmt.Println("Always eventually  invariant passed")
	}
	return nil, true
}

func pathToInit(nodes []*Node, closestDeadNode *Node) []*Link {
	failurePath := make([]*Link, 0)

	node := closestDeadNode
	for node != nil {

		if len(node.Inbound) == 0 || node.Name == "init" || node == nodes[0] {
			failurePath = append(failurePath, InitNodeToLink(node))
			break
		}

		failurePath = append(failurePath, ReverseLink(node, node.Inbound[0]))
		node = node.Inbound[0].Node
	}
	slices.Reverse(failurePath)
	return failurePath
}

func InitNodeToLink(node *Node) *Link {
	return &Link{
		Node:     node,
		Name:     "Init",
		Labels:   node.Labels,
		Fairness: node.Fairness,
	}
}

func findCyclePath(startNode *Node, nodes map[*Node]bool) []*Link {
	type Wrapper struct {
		link    *Link
		path    []*Link
		visited map[*Node]bool
	}
	queue := lib.NewQueue[*Wrapper]()
	queue.Enqueue(&Wrapper{link: InitNodeToLink(startNode), path: make([]*Link, 0), visited: make(map[*Node]bool)})

	for queue.Count() > 0 {
		element, _ := queue.Dequeue()
		node := element.link.Node
		path := element.path
		visited := element.visited
		fairCount := 0
		for _, link := range node.Outbound {
			if link.Fairness != ast.FairnessLevel_FAIRNESS_LEVEL_STRONG {
				continue
			}
			if !nodes[link.Node] {
				continue
			}
			fairCount++
			pathCopy := slices.Clone(path)
			visitedCopy := maps.Clone(visited)

			pathCopy = append(pathCopy, link)
			if visitedCopy[node] {
				return path
			}
			visitedCopy[node] = true
			queue.Enqueue(&Wrapper{link: link, path: pathCopy, visited: visitedCopy})
		}
		if fairCount == 0 {
			pathCopy := slices.Clone(path)
			pathCopy = append(pathCopy, &Link{
				Node: node,
				Name: "stutter",
			})
			return pathCopy
		}
	}
	// TODO: Should this panic?
	panic("Cycle not found")
	//return nil
}

func EventuallyAlwaysFast(nodes []*Node, predicate Predicate) ([]*Link, bool) {
	// For strong fairness to support Eventually Always. The logic is,
	// For each bad node, walk up the Strongly Fair inbound links, and mark them bad as well. Eventually, you will
	// end up with only nodes that can never reach a bad node.
	// This is the list of good nodes.
	// Then use this fact to create a Predicate that can be used to check Always Eventually. That is,
	// if any behavior can reach these known good state via strong fair nodes, we know for sure that it will
	// never reach a bad state.

	trueNodes := make(map[*Node]bool)
	visited := make(map[*Node]bool)
	queue := lib.NewQueue[*Node]()
	for _, node := range nodes {
		if len(node.Outbound) == 0 {
			fmt.Println("Deadlock detected, at node: ", node.String())
			panic("Deadlock detected, at node: " + node.String())
		}
		relevant, value := predicate(node)
		if relevant && !value {
			queue.Enqueue(node)
		} else if relevant {
			trueNodes[node] = true
		}
	}
	//fmt.Println("True nodes len:", len(trueNodes))
	//fmt.Println("Queue len:", queue.Count())
	for queue.Count() > 0 {
		node, _ := queue.Dequeue()
		//fmt.Println("Dequeued Node:", node.String())
		if visited[node] {
			continue
		}
		visited[node] = true
		for _, link := range node.Inbound {
			//fmt.Println("Link:", link.Node.String())
			if visited[link.Node] {
				continue
			}
			delete(trueNodes, link.Node)
			queue.Enqueue(link.Node)
		}
	}
	//fmt.Println("True nodes len:", len(trueNodes))
	//fmt.Println("True nodes:", trueNodes)
	if len(trueNodes) > 0 {
		// Create a predicate that can be used to check Always Eventually
		predicate := func(n *Node) (bool, bool) {
			return true, trueNodes[n]
		}
		// Always Eventually
		failurePath, isLive := AlwaysEventuallyFast(nodes, predicate)

		return failurePath, isLive

	}
	fmt.Println("Every behavior leads to a bad state eventually")

	return CycleFinderFinalBfs(nodes[0], func(path []*Link, cycles int) (bool, *CycleCallbackResult) {
		return false, nil
	})
}

type Predicate func(n *Node) (bool, bool)

type CycleCallbackResult struct {
	missingLinks []*Pair[*Node, []*Link]
}
type CycleCallback func(path []*Link, cycles int) (bool, *CycleCallbackResult)

func AlwaysEventuallyFinal(root *Node, predicate Predicate) ([]*Link, bool) {
	f := func(path []*Link, cycles int) (bool, *CycleCallbackResult) {
		mergeNode := path[len(path)-1].Node
		mergeIndex := -1
		// iterate over the path in order to find the earliest merge node forming the largest cycle
		// Then check if the property holds in that cycle
		for i := 0; i < len(path)-1; i++ {
			if path[i].Node == mergeNode {
				mergeIndex = i
				for j := i + 1; j < len(path); j++ {
					relevant, value := predicate(path[j].Node)
					if relevant && value {
						return true, nil
					}
				}
				break
			}

		}
		if mergeIndex == -1 {
			//fmt.Println("No merge node found")
			return true, nil
		}

		//fmt.Println("Live node NOT FOUND in the path")
		isFair, cycleCallbackResult := isFairCycle(path[mergeIndex:], false)
		if isFair {
			//fmt.Println("Fair cycle found")
			//isFairCycle(path[mergeIndex:], true)
			return false, nil
		} else {
			//fmt.Println("Not a fair cycle, and has fair exit link")
			return true, cycleCallbackResult
		}
	}
	return CycleFinderFinal(root, f)
}

func EventuallyAlwaysFinal(root *Node, predicate Predicate) ([]*Link, bool) {
	f := func(path []*Link, cycles int) (bool, *CycleCallbackResult) {
		mergeNode := path[len(path)-1].Node
		mergeIndex := 0
		deadNodeFound := false

		// iterate over the path in order to find the earliest merge node forming the largest cycle
		// Then check if the property holds in that cycle
		for i := 0; i < len(path)-1; i++ {
			if path[i].Node == mergeNode {
				mergeIndex = i
				for j := i + 1; j < len(path); j++ {
					relevant, value := predicate(path[j].Node)
					if relevant && !value {
						deadNodeFound = true
						break
					}
				}
				break
			}
		}

		if deadNodeFound {
			isFair, cycleCallbackResult := isFairCycle(path[mergeIndex:], false)
			if isFair {
				//fmt.Println("Fair cycle found")
				return false, nil
			} else {
				//fmt.Println("Not a fair cycle, and has fair exit link")
				return true, cycleCallbackResult
			}
			//return false, nil
		}
		return true, nil

		//fmt.Println("Dead node NOT FOUND in the path")
	}
	return CycleFinderFinal(root, f)
}

func isFairCycle(path []*Link, debugLog bool) (bool, *CycleCallbackResult) {
	strongFairLinksInChain := map[string]bool{}
	strongFairLinksOutOfChain := map[string]bool{}

	weakFairLinksInChain := map[string]bool{}
	weakFairLinksOutOfChain := map[string]bool{}

	chainLen := len(path)
	firstYield := -1
	prevNodeHasNonCrashLink := false
	nextNodeIsCrash := false
	if debugLog {
		for i, link := range path {
			//node := link.Node
			fmt.Println("i :", i, "Link.Name", link.Name /*"Node:", node.String(),*/, "choice fairness", link.ChoiceFairness)
		}
		fmt.Println("Checking fairness")
	}
	for i, link := range path {
		node := link.Node
		unvisitedWeakFairLinksOutOfChain := map[string]bool{}

		if debugLog {
			fmt.Println(i, ": link: ", link.Name, "Node:", node.String(), "choice fairness", link.ChoiceFairness)
		}
		if nextNodeIsCrash && prevNodeHasNonCrashLink {
			if debugLog {
				fmt.Println("Loop with a crash node, but previous node has non-crash link")
			}
			return false, nil
		}
		isChoiceLink := false
		if node.Name != "init" && node.Name != "yield" && node.Name != "crash" {
			if debugLog {
				fmt.Println("Node is not init or yield")
			}

			if len(node.Outbound) <= 0 || !strings.HasPrefix(node.Outbound[0].Name, "Any:") ||
				node.Outbound[0].ChoiceFairness == ast.FairnessLevel_FAIRNESS_LEVEL_UNKNOWN ||
				node.Outbound[0].ChoiceFairness == ast.FairnessLevel_FAIRNESS_LEVEL_UNFAIR {
				if debugLog {
					fmt.Println("Not a fair Any choice node")
				}
				continue

			}
			isChoiceLink = true
			if debugLog {
				fmt.Println("Fair Any choice node")
			}
		}
		if debugLog {
			fmt.Println("isChoiceLink", isChoiceLink)
		}
		if firstYield == -1 {
			firstYield = i
		}
		prevNodeHasNonCrashLink = false
		for _, outLink := range node.Outbound {
			linkName, fairness, _ := fairnessLinkName(node, outLink)
			if debugLog {
				fmt.Println("Outlink:", outLink.Name, outLink.Fairness, outLink.Labels, outLink.Node.Name, outLink.ChoiceFairness, linkName, fairness)
			}
			if !isCrashLinkName(linkName) {
				prevNodeHasNonCrashLink = true
			} else if outLink.Node == path[(i+1)%chainLen].Node {
				nextNodeIsCrash = true
			}
			if fairness == ast.FairnessLevel_FAIRNESS_LEVEL_STRONG {
				if outLink.Node == path[(i+1)%chainLen].Node {
					// outlink points to the next node in the chain
					// It satisfies the strong fairness condition for that action
					strongFairLinksInChain[linkName] = true
					delete(strongFairLinksOutOfChain, linkName)
				} else if _, ok := strongFairLinksInChain[linkName]; !ok {
					strongFairLinksOutOfChain[linkName] = true
				}
			} else if fairness == ast.FairnessLevel_FAIRNESS_LEVEL_WEAK {
				if outLink.Node == path[(i+1)%chainLen].Node {
					if debugLog {
						fmt.Println("Weak Fair link in chain", linkName, outLink.Node.Name)
					}
					weakFairLinksInChain[linkName] = true
					delete(unvisitedWeakFairLinksOutOfChain, linkName)
				} else if _, ok := weakFairLinksInChain[linkName]; !ok {
					if debugLog {
						fmt.Println("Weak Fair link out of chain", linkName, outLink.Node.Name)
					}
					unvisitedWeakFairLinksOutOfChain[linkName] = true
				}
			}
		}
		if debugLog {
			fmt.Println("Unvisited weak fair links out of chain", unvisitedWeakFairLinksOutOfChain)
		}
		if i == firstYield {
			weakFairLinksOutOfChain = unvisitedWeakFairLinksOutOfChain
		} else {
			for k, _ := range weakFairLinksOutOfChain {
				if isChoiceLink != strings.HasPrefix(k, "Any:") {
					if debugLog {
						fmt.Println("Not deleting weak Fair link out of chain", k, isChoiceLink)
					}
					continue
				}
				if _, ok := unvisitedWeakFairLinksOutOfChain[k]; !ok {
					if debugLog {
						fmt.Println("Deleting weak Fair link out of chain", k, isChoiceLink)
					}
					delete(weakFairLinksOutOfChain, k)
				}
			}

		}
		if debugLog {
			fmt.Println("weakFairLinksOutOfChain", weakFairLinksOutOfChain)
		}
	}
	if debugLog {
		fmt.Println("strong out: ", len(strongFairLinksOutOfChain),
			", strong in: ", len(strongFairLinksInChain),
			", weak out: ", len(weakFairLinksOutOfChain),
			", weak in: ", len(weakFairLinksInChain))
		fmt.Println("Strong Fair Links in chain:", strongFairLinksInChain)
		fmt.Println("Strong Fair Links out of chain:", strongFairLinksOutOfChain)
		fmt.Println("Weak Fair Links in chain:", weakFairLinksInChain)
		fmt.Println("Weak Fair Links out of chain:", weakFairLinksOutOfChain)
		fmt.Println("Checking fairness done. Is Fair:", !(len(strongFairLinksOutOfChain) > 0 || len(weakFairLinksOutOfChain) > 0))
	}
	if len(strongFairLinksOutOfChain) > 0 || len(weakFairLinksOutOfChain) > 0 {
		cycleCallbackResult := findMissingLinks(path, strongFairLinksOutOfChain, weakFairLinksOutOfChain)
		return false, cycleCallbackResult
	}
	return true, nil
}

func fairnessLinkName(node *Node, outLink *Link) (string, ast.FairnessLevel, bool) {
	linkName := outLink.Name
	isChoiceLink := strings.HasPrefix(linkName, "Any:")
	isThreadLink := strings.HasPrefix(linkName, "thread-")
	fairness := outLink.Fairness
	if isChoiceLink {
		linkName = linkName + "|" + node.currentThread().currentPc()
		if node.currentThread().currentFrame().obj != nil {
			linkName = linkName + "|" + node.currentThread().currentFrame().obj.RefStringShort()
		}
		fairness = ast.FairnessLevel_FAIRNESS_LEVEL_STRONG
	} else if isThreadLink {
		linkName = linkName + "|" + node.currentThread().currentPc()
		if node.currentThread().currentFrame().obj != nil {
			linkName = linkName + "|" + node.currentThread().currentFrame().obj.RefStringShort()
		}
		fairness = ast.FairnessLevel_FAIRNESS_LEVEL_STRONG
	}
	return linkName, fairness, isChoiceLink
}

func findMissingLinks(path []*Link, strongFairLinksOutOfChain map[string]bool, weakFairLinksOutOfChain map[string]bool) *CycleCallbackResult {
	missingLinks := make([]*Pair[*Node, []*Link], 0)
	for _, link := range path {
		node := link.Node
		nodeMissingLinks := make([]*Link, 0)
		for _, outLink := range node.Outbound {
			linkName, fairness, _ := fairnessLinkName(node, outLink)
			if fairness == ast.FairnessLevel_FAIRNESS_LEVEL_STRONG {
				if _, ok := strongFairLinksOutOfChain[linkName]; ok {
					nodeMissingLinks = append(nodeMissingLinks, outLink)
				}
			} else if fairness == ast.FairnessLevel_FAIRNESS_LEVEL_WEAK {
				if _, ok := weakFairLinksOutOfChain[linkName]; ok {
					nodeMissingLinks = append(nodeMissingLinks, outLink)
				}
			}
		}
		if len(nodeMissingLinks) > 0 {
			missingLinks = append(missingLinks, &Pair[*Node, []*Link]{First: node, Second: nodeMissingLinks})
		}
	}
	return &CycleCallbackResult{missingLinks: missingLinks}

}

func CycleFinderFinal(node *Node, callback CycleCallback) ([]*Link, bool) {
	visited := make(map[*Node]bool)
	globalVisited := make(map[*Node]bool)
	rootLink := InitNodeToLink(node)
	path := []*Link{rootLink}
	return cycleFinderHelper(node, callback, visited, 0, path, globalVisited)
}

func cycleFinderHelper(node *Node, callback CycleCallback, visited map[*Node]bool, cycles int, path []*Link, globalVisited map[*Node]bool) ([]*Link, bool) {
	if visited[node] {
		//fmt.Println("\n\nCycle detected in the path:")
		////fmt.Println("Path:", path)
		////fmt.Println("Cycle found")
		//for i, link := range path {
		//	fmt.Println(i, link.Name, link.Node.HashCode(), link.Node.String())
		//
		//}
		//fmt.Println("Visited node", node.HashCode(), node.String())
		isLive, result := callback(path, cycles)
		//fmt.Println("Is live:", isLive, "Result:", result)
		if !isLive || result == nil || len(path) > 200 || cycles > 5 {
			return path, isLive
		}
		for _, links := range result.missingLinks {
			//fmt.Println("Missing links from node", i, links.First.String())
			for _, l := range links.Second {
				//fmt.Println(j, l.Name, l.Node.String())
				// Find the next larger cycle including the missing link one by one.
				// That is, copy path, and visited, globalVisited and recursively call cycleFinderHelper for each of the missing link/node

				pathCopy := slices.Clone(path)
				visitedCopy := maps.Clone(visited)
				// Add the links from last node in path to the missing link.
				for k1, oldLink := range path {
					if oldLink.Node == path[len(path)-1].Node {
						for _, oldLink2 := range path[k1+1:] {
							pathCopy = append(pathCopy, oldLink2)
							if oldLink2.Node == links.First {
								break
							}
						}
						break
					}
				}

				pathCopy = append(pathCopy, l)
				//globalVisitedCopy := maps.Clone(globalVisited)
				failedPath, success := cycleFinderHelper(l.Node, callback, visitedCopy, cycles+1, pathCopy, globalVisited)
				if !success {
					return failedPath, false
				}
			}

		}
		return path, isLive
	}

	visited[node] = true
	if globalVisited[node] {
		//fmt.Println("Skipping node", node.String())
		return nil, true
	}
	globalVisited[node] = true
	hasFair := false
	pendingAction := false
	for _, link := range node.Outbound {
		if link.Fairness == ast.FairnessLevel_FAIRNESS_LEVEL_STRONG ||
			link.Fairness == ast.FairnessLevel_FAIRNESS_LEVEL_WEAK {
			hasFair = true
		}
		if strings.HasPrefix(link.Name, "thread-") {
			pendingAction = true
		}
	}

	if (node.Name == "yield" || node.Name == "crash" || node.Name == "init") && !hasFair && !pendingAction {
		pathCopy := slices.Clone(path)
		pathCopy = append(pathCopy, &Link{
			Node: node,
			Name: "stutter",
		})

		isLive, _ := callback(pathCopy, cycles)
		if !isLive {
			return pathCopy, false
		}

	}
	// Traverse outbound links
	for _, link := range node.Outbound {
		pathCopy := slices.Clone(path)
		visitedCopy := maps.Clone(visited)
		pathCopy = append(pathCopy, link)
		failedPath, success := cycleFinderHelper(link.Node, callback, visitedCopy, cycles, pathCopy, globalVisited)
		if !success {
			return failedPath, false
		}
	}

	return nil, true
}

func CycleFinderFinalBfs(node *Node, callback CycleCallback) ([]*Link, bool) {
	visited := make(map[*Node]bool)
	path := make([]*Link, 0)
	return cycleFinderHelperBfs(node, callback, visited, path)
}

func cycleFinderHelperBfs(root *Node, callback CycleCallback, visited map[*Node]bool, path []*Link) ([]*Link, bool) {
	type Wrapper struct {
		link    *Link
		path    []*Link
		visited map[*Node]bool
	}
	queue := lib.NewQueue[*Wrapper]()
	queue.Enqueue(&Wrapper{link: InitNodeToLink(root), path: path, visited: visited})
	for queue.Count() > 0 {
		element, _ := queue.Dequeue()
		node := element.link.Node
		path = element.path
		visited = element.visited

		if visited[node] {
			path = append(path, element.link)
			//fmt.Println("\n\nCycle detected in the path:")
			//fmt.Println("Path:", path)
			live, _ := callback(path, 1)
			if live {
				continue
			}
			return path, false
		}
		visited[node] = true
		path = append(path, element.link)

		fairCount := 0
		// Traverse outbound links
		for _, outLink := range node.Outbound {
			if outLink.Fairness == ast.FairnessLevel_FAIRNESS_LEVEL_STRONG ||
				outLink.Fairness == ast.FairnessLevel_FAIRNESS_LEVEL_WEAK {
				fairCount++
			}
			pathCopy := slices.Clone(path)
			visitedCopy := maps.Clone(visited)
			queue.Enqueue(&Wrapper{link: outLink, path: pathCopy, visited: visitedCopy})

		}
		if fairCount == 0 {
			pathCopy := slices.Clone(path)
			pathCopy = append(pathCopy, &Link{
				Node: node,
				Name: "stutter",
			})
			live, _ := callback(pathCopy, 1)
			if live {
				continue
			}
			return pathCopy, false

		}
	}
	return nil, true
}

func NewDictFromStringDict(vals starlark.StringDict) *starlark.Dict {
	result := starlark.NewDict(len(vals))
	for k, v := range vals {
		err := result.SetKey(starlark.String(k), v)
		// Should not fail
		PanicOnError(err)
	}
	return result
}

// Tarjan's algorithm helper structure
type tarjanData struct {
	index   int
	lowLink int
	onStack bool
}

func CheckLivenessSccAlwaysEventually(nodes []*Node, invPos InvariantPosition) (bool, error) {
	// Map each node to its Tarjan data
	tarjan := make(map[*Node]*tarjanData)
	var stack []*Node
	var index int
	var sccs [][]*Node

	// Tarjan's algorithm to find SCCs
	var strongConnect func(node *Node)
	strongConnect = func(node *Node) {
		data := &tarjanData{index: index, lowLink: index, onStack: true}
		tarjan[node] = data
		stack = append(stack, node)
		index++

		for _, link := range node.Outbound {
			next := link.Node
			if _, exists := tarjan[next]; !exists {
				// Visit unvisited node
				strongConnect(next)
				data.lowLink = min(data.lowLink, tarjan[next].lowLink)
			} else if tarjan[next].onStack {
				// Update lowLink for back edges
				data.lowLink = min(data.lowLink, tarjan[next].index)
			}
		}

		// If node is a root of an SCC
		if data.lowLink == data.index {
			var scc []*Node
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				tarjan[w].onStack = false
				scc = append(scc, w)
				if w == node {
					break
				}
			}
			sccs = append(sccs, scc)
		}
	}

	// Initialize Tarjan's algorithm
	for _, node := range nodes {
		if _, visited := tarjan[node]; !visited {
			strongConnect(node)
		}
	}

	// Check each SCC for liveness property
	for i, scc := range sccs {
		fmt.Println("SCC:", i, len(scc))
		for j, node := range scc {
			fmt.Println(j, node.String())
		}
		//if isTerminalSCC(scc) {
		//	// Check if every node in this SCC satisfies P
		//	for _, node := range scc {
		//		if !node.Process.Witness[invPos.FileIndex][invPos.InvariantIndex] {
		//			return false, fmt.Errorf("liveness property violated in SCC containing node %+v", node)
		//		}
		//	}
		//}
	}

	return true, nil
}

// Helper to determine if an SCC is terminal
func isTerminalSCC(scc []*Node) bool {
	nodeSet := make(map[*Node]struct{})
	for _, node := range scc {
		nodeSet[node] = struct{}{}
	}

	for _, node := range scc {
		for _, link := range node.Outbound {
			if _, inSCC := nodeSet[link.Node]; !inSCC {
				return false
			}
		}
	}
	return true
}
