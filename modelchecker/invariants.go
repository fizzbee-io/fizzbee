package modelchecker

import (
	ast "fizz/proto"
	"fmt"
	"github.com/jayaprabhakar/fizzbee/lib"
	"go.starlark.net/starlark"
	"maps"
	"slices"
)

type InvariantPosition struct {
	FileIndex int
	InvariantIndex int
}

func NewInvariantPosition(fileIndex, invariantIndex int) *InvariantPosition {
	return &InvariantPosition{
		FileIndex: fileIndex,
		InvariantIndex: invariantIndex,
	}
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
				if invariant.Eventually && passed && len(process.Threads) == 0 {
					process.Witness[i][j] = true
				} else if !invariant.Eventually && !passed {
					results[i] = append(results[i], j)
				}
			} else {
				passed = CheckAssertion(process, invariant)
				if slices.Contains(invariant.TemporalOperators, "eventually") && passed && len(process.Threads) == 0  {
					process.Witness[i][j] = true
				} else if !slices.Contains(invariant.TemporalOperators, "eventually") && !passed {
					results[i] = append(results[i], j)
				}
			}
		}
	}
	return results
}

func CheckInvariant(process *Process, invariant *ast.Invariant) bool {
	eventuallyAlways := invariant.Eventually && invariant.GetNested().GetAlways()
	if !invariant.Always && !(eventuallyAlways){
		panic("Invariant checking not supported for non-always invariants")
	}
	if !eventuallyAlways && invariant.Nested != nil {
		panic("Invariant checking not supported for nested invariants")
	}
	pyExpr := invariant.PyExpr
	if eventuallyAlways && invariant.Nested != nil {
		pyExpr = invariant.Nested.PyExpr
	}
	vars := CloneDict(process.Heap.globals)
	vars["__returns__"] = NewDictFromStringDict(process.Returns)
	cond, err := process.Evaluator.EvalPyExpr("filename.fizz", pyExpr, vars)
	PanicOnError(err)
	return bool(cond.Truth())
}

func CheckAssertion(process *Process, invariant *ast.Invariant) bool {
	if !slices.Contains(invariant.TemporalOperators, "always") {
		panic("Invariant checking supported only for always/always-eventually/eventually-always invariants")
	}

	vars := CloneDict(process.Heap.globals)
	vars["__returns__"] = NewDictFromStringDict(process.Returns)
	pyStmt := &ast.PyStmt{
		Code: invariant.PyCode + "\n" + "__retval__ = " + invariant.Name + "()\n",
	}
	_, err := process.Evaluator.ExecPyStmt("filename.fizz", pyStmt, vars)
	PanicOnError(err)
	return bool(vars["__retval__"].Truth())
}

func CheckStrictLiveness(node *Node) ([]*Link, *InvariantPosition) {
	fmt.Println("Checking strict liveness")
	process := node.Process
	if len(process.Files) > 1 {
		panic("Invariant checking not supported for multiple files yet")
	}
	for i, file := range process.Files {
		for j, invariant := range file.Invariants {
			predicate := func(n *Node) (bool, bool) {
				return len(n.Process.Threads) == 0, n.Process.Witness[i][j]
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
					return failurePath, NewInvariantPosition(i,j)
				}
			} else if alwaysEventually {
				fmt.Println("Checking always eventually", invariant.Name)
				// Always Eventually
				failurePath, isLive := AlwaysEventuallyFinal(node, predicate)
				if !isLive {
					return failurePath, NewInvariantPosition(i,j)
				}
			}
		}

	}
	return nil, nil
}

func CheckFastLiveness(allNodes []*Node) ([]*Link, *InvariantPosition) {
	fmt.Println("Checking strict liveness fast approach")
	node := allNodes[0]
	process := node.Process
	if len(process.Files) > 1 {
		panic("Invariant checking not supported for multiple files yet")
	}
	for i, file := range process.Files {
		for j, invariant := range file.Invariants {
			predicate := func(n *Node) (bool, bool) {
				return len(n.Process.Threads) == 0, n.Process.Witness[i][j]
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
					return failurePath, NewInvariantPosition(i,j)
				}
			} else if alwaysEventually {
				fmt.Println("Checking always eventually", invariant.Name)
				// Always Eventually
				failurePath, isLive := AlwaysEventuallyFast(allNodes, predicate)
				if !isLive {
					return failurePath, NewInvariantPosition(i,j)
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
		if len(node.Outbound) == 0 {
			fmt.Println("Deadlock detected, at node: ", node.String())
			panic("Deadlock detected, at node: " + node.String())
		}
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
				link.Fairness != ast.FairnessLevel_FAIRNESS_LEVEL_STRONG {
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
			if closestDeadNode == nil || (len(closestDeadNode.Threads) > 0 && len(node.Threads) == 0) {
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
		link *Link
		path []*Link
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
				Node:     node,
				Name:     "stutter",
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

	return CycleFinderFinalBfs(nodes[0], func(path []*Link) bool {
		return false
	})
}


type Predicate func(n *Node) (bool, bool)

type CycleCallback func(path []*Link) bool

func AlwaysEventuallyFinal(root *Node, predicate Predicate) ([]*Link, bool) {
	f := func(path []*Link) bool {
		mergeNode := path[len(path)-1].Node
		mergeIndex := 0
		// iterate over the path in reverse order and check if the property holds
		for i := len(path) - 1; i >= 0; i-- {
			relevant, value := predicate(path[i].Node)
			//fmt.Printf("Node: %s, Relevant: %t, Value: %t\n", path[i].String(), relevant, value)
			if relevant && value {
				//fmt.Println("Live node FOUND in the path")
				return true
			}
			if i < len(path) - 1 && path[i].Node == mergeNode {
				mergeIndex = i
				break
			}
		}
		//fmt.Println("Live node NOT FOUND in the path")
		if isFairCycle(path[mergeIndex:]) {
			//fmt.Println("Fair cycle found")
			return false
		} else {
			//fmt.Println("Not a fair cycle, and has fair exit link")
			return true
		}
	}
	return CycleFinderFinal(root, f)
}

func EventuallyAlwaysFinal(root *Node, predicate Predicate) ([]*Link, bool) {
	f := func(path []*Link) bool {
		mergeNode := path[len(path)-1].Node
		mergeIndex := 0
		deadNodeFound := false
		// iterate over the path in reverse order and check if the property holds
		for i := len(path) - 1; i >= 0; i-- {
			relevant, value := predicate(path[i].Node)
			//fmt.Printf("Node: %+v, Relevant: %t, Value: %t\n", path[i], relevant, value)
			if relevant && !value {
				//fmt.Println("Dead node FOUND in the path")
				deadNodeFound = true
			}
			if i < len(path) - 1 && path[i].Node == mergeNode {
				mergeIndex = i
				break
			}
		}
		if deadNodeFound && isFairCycle(path[mergeIndex:]) {
			////fmt.Println("Fair cycle found")
			return false
		} else {
			//fmt.Println("Not a fair cycle, and has fair exit link")
			return true
		}
		//fmt.Println("Dead node NOT FOUND in the path")
	}
	return CycleFinderFinal(root, f)
}

func isFairCycle(path []*Link) bool {
	strongFairLinksInChain := map[string]bool{}
	strongFairLinksOutOfChain := map[string]bool{}

	weakFairLinksInChain := map[string]bool{}
	weakFairLinksOutOfChain := map[string]bool{}

	chainLen := len(path)
	for i, link := range path {
		node := link.Node
		unvisitedWeakFairLinksOutOfChain := map[string]bool{}
		for _, outLink := range node.Outbound {
			if outLink.Fairness == ast.FairnessLevel_FAIRNESS_LEVEL_STRONG {
				if outLink.Node == path[(i+1)%chainLen].Node {
					// outlink points to the next node in the chain
					// It satisfies the strong fairness condition for that action
					strongFairLinksInChain[outLink.Name] = true
					delete(strongFairLinksOutOfChain, outLink.Name)
				} else if _, ok := strongFairLinksInChain[outLink.Name]; !ok {
					strongFairLinksOutOfChain[outLink.Name] = true
				}
			} else if outLink.Fairness == ast.FairnessLevel_FAIRNESS_LEVEL_WEAK {
				if outLink.Node == path[(i+1)%chainLen].Node {
					weakFairLinksInChain[outLink.Name] = true
					delete(unvisitedWeakFairLinksOutOfChain, outLink.Name)
				} else if _, ok := weakFairLinksInChain[outLink.Name]; !ok {
					unvisitedWeakFairLinksOutOfChain[outLink.Name] = true
				}
			}
		}
		if i == 0 {
			weakFairLinksOutOfChain = unvisitedWeakFairLinksOutOfChain
		} else {
			for k, _ := range weakFairLinksOutOfChain {
				if _, ok := unvisitedWeakFairLinksOutOfChain[k]; !ok {
					delete(weakFairLinksOutOfChain, k)
				}
			}

		}
	}
	if len(strongFairLinksOutOfChain) > 0 || len(weakFairLinksOutOfChain) > 0 {
		return false
	}
	return true
}

func CycleFinderFinal(node *Node, callback CycleCallback) ([]*Link, bool) {
	visited := make(map[*Node]bool)
	globalVisited := make(map[*Node]bool)
	rootLink := InitNodeToLink(node)
	path := []*Link{rootLink}
	return cycleFinderHelper(node, callback, visited, path, globalVisited)
}

func cycleFinderHelper(node *Node, callback CycleCallback, visited map[*Node]bool, path []*Link, globalVisited map[*Node]bool) ([]*Link, bool) {
	if visited[node] {
		//fmt.Println("\n\nCycle detected in the path:")
		//fmt.Println("Path:", path)
		return path, callback(path)
	}

	visited[node] = true
	if globalVisited[node] {
		//fmt.Println("Skipping node", node.String())
		return nil, true
	}
	globalVisited[node] = true
	fairCount := 0
	for _, link := range node.Outbound {
		if link.Fairness == ast.FairnessLevel_FAIRNESS_LEVEL_STRONG ||
			link.Fairness == ast.FairnessLevel_FAIRNESS_LEVEL_WEAK {
			fairCount++
		}
	}
	if fairCount == 0 {
		pathCopy := slices.Clone(path)
		pathCopy = append(pathCopy, &Link{
			Node:     node,
			Name:     "stutter",
		})
		return pathCopy, callback(pathCopy)
	}
	// Traverse outbound links
	for _, link := range node.Outbound {
		if link.Fairness == ast.FairnessLevel_FAIRNESS_LEVEL_STRONG ||
			link.Fairness == ast.FairnessLevel_FAIRNESS_LEVEL_WEAK {
			fairCount++
		}
		pathCopy := slices.Clone(path)
		visitedCopy := maps.Clone(visited)
		pathCopy = append(pathCopy, link)
		failedPath, success := cycleFinderHelper(link.Node, callback, visitedCopy, pathCopy, globalVisited)
		if !success {
			return failedPath,false
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
		link *Link
		path []*Link
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
			live := callback(path)
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
				Node:     node,
				Name:     "stutter",
			})
			live := callback(pathCopy)
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
