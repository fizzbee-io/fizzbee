// Package modelchecker implements the model checker for the FizzBuzz program.
// It is based on the Starlark interpreter for the python part of the code.
// For the interpreter to implement the model checker, we need to simulate
// parallel universe.
// Every time, there is a non-deterministic choice, we need to fork the universe
// and continue the execution in both the universes with the different choices.
// Each universe is represented by a process.
// Each process has a heap and multiple threads.
// Each thread has a stack of call frames.
// Each call frame has a program counter and scope (with nesting).
// The heap is shared across all the threads in the process.
// Duplicate detection: Two threads are same if they have the same stack of call frames
// Two processes are same if they have the same heap and same threads.
package modelchecker

import (
	"crypto/sha256"
	"encoding/json"
	ast "fizz/proto"
	"fmt"
	"github.com/jayaprabhakar/fizzbee/lib"
	"go.starlark.net/starlark"
	"maps"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"
)

// DefType is a custom enum-like type
type DefType string

const (
	Function DefType = "function"
)

const enableCaptureStackTrace = false

type Definition struct {
	DefType   DefType
	name      string
	fileIndex int
	path      string
}

type Stats struct {
	TotalActions int         `json:"totalActions"`
	Counts map[string]int    `json:"counts"`
}

// NewStats returns a new Stats object
func NewStats() *Stats {
	return &Stats{
		Counts: make(map[string]int),
	}
}

func (s *Stats) Clone() *Stats {
	stats := &Stats{
		TotalActions: s.TotalActions,
		Counts: make(map[string]int),
	}
	for k, v := range s.Counts {
		stats.Counts[k] = v
	}
	return stats
}

func (s *Stats) Increment(action string) {
	s.TotalActions++
	s.Counts[action]++
}

type Process struct {
	Heap             *Heap		      `json:"state"`
	Threads          []*Thread        `json:"threads"`
	Current          int              `json:"current"`
	Name             string           `json:"name"`
	Files            []*ast.File      `json:"-"`
	Parent           *Process         `json:"-"`
	Evaluator        *Evaluator       `json:"-"`
	Children         []*Process       `json:"-"`
	FailedInvariants map[int][]int    `json:"failedInvariants"`
	Stats            *Stats           `json:"stats"`
	// Witness indicates the successful liveness checks
	// For liveness checks, not all nodes will pass the condition, witness indicates
	// which invariants this node passed.
	Witness     [][]bool               `json:"witness"`
	Returns     starlark.StringDict    `json:"returns"`
	SymbolTable map[string]*Definition `json:"-"`
	Labels 		[]string               `json:"-"`

	// Fairness is actually a property of the transition/link. But to determine whether
	// the link is fair, we need to know if the process stepped through at least one
	// fair statement. To determine that, each thread maintains the fairness level
	// of the action that started. If that thread executed a statement, that process becomes fair,
	// that in-turn makes the link fair.
	Fairness    ast.FairnessLevel      `json:"-"`

	Enabled		bool                   `json:"-"`
}

func NewProcess(name string, files []*ast.File, parent *Process) *Process {
	var mc *Evaluator
	var symbolTable map[string]*Definition

	if parent == nil {
		mc = NewModelChecker("example")
		symbolTable = make(map[string]*Definition)

		for i, file := range files {
			for j, function := range file.Functions {
				symbolTable[function.Name] = &Definition{
					DefType:   Function,
					name:      function.Name,
					fileIndex: i,
					path:      fmt.Sprintf("Functions[%d]", j),
				}
			}
		}
	} else {
		mc = parent.Evaluator
		symbolTable = parent.SymbolTable
	}
	p := &Process{
		Name:        name,
		Heap:        &Heap{starlark.StringDict{}},
		Threads:     []*Thread{},
		Current:     0,
		Files:       files,
		Parent:      parent,
		Evaluator:   mc,
		Children:    []*Process{},
		Returns:     make(starlark.StringDict),
		SymbolTable: symbolTable,
		Labels:      make([]string, 0),
		Stats:       NewStats(),
	}
	p.Witness = make([][]bool, len(files))
	for i, file := range files {
		p.Witness[i] = make([]bool, len(file.Invariants))
	}
	p.Children = append(p.Children, p)

	return p
}

func (p *Process) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"state":     p.Heap,
		"threads":   p.Threads,
		"current":   p.Current,
		"name":      p.Name,
		"failedInvariants": p.FailedInvariants,
		"stats":     p.Stats,
		"witness":   p.Witness,
		"returns":   StringDictToJsonString(p.Returns),
	})
}

func (p *Process) HasFailedInvariants() bool {
	if p == nil || p.FailedInvariants == nil {
		return false
	}
	for _, invIndex := range p.FailedInvariants {
		if len(invIndex) > 0 {
			return true
		}
	}
	return false
}

func (p *Process) Fork() *Process {
	p2 := &Process{
		Name:        p.Name,
		Heap:        p.Heap.Clone(),
		Current:     p.Current,
		Parent:      p,
		Evaluator:   p.Evaluator,
		Children:    []*Process{},
		Files:       p.Files,
		Returns:     make(starlark.StringDict),
		SymbolTable: p.SymbolTable,
		Labels:      make([]string, 0),
		Stats:       p.Stats.Clone(),
	}
	p2.Witness = make([][]bool, len(p.Files))
	for i, file := range p.Files {
		p2.Witness[i] = make([]bool, len(file.Invariants))
	}

	p.Children = append(p.Children, p2)
	clonedThreads := make([]*Thread, len(p.Threads))
	for i, thread := range p.Threads {
		clonedThreads[i] = thread.Clone()
		clonedThreads[i].Process = p2
	}
	p2.Threads = clonedThreads
	return p2
}

func (p *Process) Enable() {
	if !p.Enabled {
		parent := p.Parent
		for parent != nil && len(parent.Threads) != 0 && !parent.Enabled {
			parent.Enabled = true
			parent = parent.Parent

		}
	}
	p.Enabled = true
}

func (p *Process) NewThread() *Thread {
	thread := NewThread(p, p.Files, 0, "")
	p.Threads = append(p.Threads, thread)
	return thread
}

// String method for Process
func (n *Node) String() string {
	p := n.Process
	if p == nil {
		return "DUPLICATE"
	}
	buf := &strings.Builder{}
	escapedName := strings.ReplaceAll(p.Name, "\"", "\\\"")
	buf.WriteString(fmt.Sprintf("%s\n", escapedName))
	buf.WriteString(fmt.Sprintf("Actions: %d, Forks: %d\n", n.actionDepth, n.forkDepth))
	//buf.WriteString(fmt.Sprintf("Enabled: %t\n", n.Process.Enabled))
	//buf.WriteString(fmt.Sprintf("Fair: %s\n", n.Process.Fairness))

	n.appendState(p, buf)
	buf.WriteString("\n")
	if len(p.Threads) > 0 {
		buf.WriteString(fmt.Sprintf("Threads: %d/%d\n", p.Current, len(p.Threads)))
	} else {
		//buf.WriteString("Threads: 0\n")
	}

	return buf.String()
}

func (n *Node) MarshalJSON() ([]byte, error) {
	return json.Marshal(n.Process)
}

func (n *Node) GetJsonString() string {
	bytes, err := json.Marshal(n.Process)
	if err != nil {
		fmt.Println("Error:", err)
		return ""
	}
	return string(bytes)
}

func (n *Node) GetStateString() string {
	buf := &strings.Builder{}
	n.appendState(n.Process, buf)
	return buf.String()
}
func (n *Node) appendState(p *Process, buf *strings.Builder) {
	if len(p.Heap.globals) > 0 {
		jsonString := p.Heap.String()
		// Escape double quotes
		escapedString := strings.ReplaceAll(jsonString, "\"", "\\\"")
		buf.WriteString("State: ")
		buf.WriteString(escapedString)
	}
	if len(p.Returns) > 0 {
		jsonString := StringDictToJsonString(p.Returns)
		// Escape double quotes
		escapedString := strings.ReplaceAll(jsonString, "\"", "\\\"")
		buf.WriteString("Returns: ")
		buf.WriteString(escapedString)
	}
}

// GetName returns the name
func (n *Node) GetName() string {
	p := n.Process
	if p == nil {
		return ""
	}
	return p.Name
}

func (p *Process) HashCode() string {
	threadHashes := make([]string, len(p.Threads))
	for i, thread := range p.Threads {
		threadHashes[i] = thread.HashCode()
	}

	h := sha256.New()

	// Use the Current thread's hash first, not the index
	currentThreadHash := ""
	if len(threadHashes) > 0 {
		currentThreadHash = threadHashes[p.Current]
	}
	h.Write([]byte(currentThreadHash))

	// Sort the thread hashes to make the hash deterministic
	sort.Strings(threadHashes)
	for _, hash := range threadHashes {
		h.Write([]byte(hash))
	}

	h.Write([]byte(StringDictToJsonString(p.Returns)))

	// hash the heap variables as well
	heapHash := p.Heap.HashCode()
	h.Write([]byte(heapHash))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func (p *Process) currentThread() *Thread {
	return p.Threads[p.Current]
}

func (p *Process) removeCurrentThread() {
	if len(p.Threads) == 0 {
		return
	}
	p.Threads = append(p.Threads[:p.Current],
		p.Threads[p.Current+1:]...)
	p.Current = 0
}

// GetAllVariables returns all variables visible in the Current thread.
// This includes state variables and variables from the Current thread's variables in the top call frame
func (p *Process) GetAllVariables() starlark.StringDict {
	dict := CloneDict(p.Heap.globals)
	frame := p.currentThread().currentFrame()
	frame.scope.getAllVisibleVariablesToDict(dict)
	return dict
}

func (p *Process) updateAllVariablesInScope(dict starlark.StringDict) {
	frame := p.currentThread().currentFrame()
	for k, v := range dict {
		if p.updateScopedVariable(frame.scope, k, v) {
			// Check local variables in the scope, starting from
			// deepest to its parent. If present, update that
			// and continue
			continue
		}
		if p.Heap.update(k, v) {
			// if no scoped variable exists, check if it is state
			// variable, then update the state variable
			continue
		}
		// Declare the variable to the Current scope
		frame.scope.vars[k] = v
	}
}

func (p *Process) updateScopedVariable(scope *Scope, key string, val starlark.Value) bool {
	if scope == nil {
		return false
	}
	if _, ok := scope.vars[key]; ok {
		scope.vars[key] = val
		return true
	}
	return p.updateScopedVariable(scope.parent, key, val)
}

func (p *Process) NewModelError(msg string, nestedError error) *ModelError {
	return NewModelError(msg, p, nestedError)
}

func (p *Process) PanicOnError(msg string, nestedError error)  {
	if nestedError != nil {
		panic(p.NewModelError(msg, nestedError))
	}
}

type Node struct {
	*Process            `json:"process"`

	Inbound  []*Link    `json:"-"`
	Outbound []*Link    `json:"-"`

	// The number of actions started until this node
	// Note: This is the shorted path to this node from the root as we do BFS.
	actionDepth int

	// The number of forks until this node from the root. This will be >= actionDepth
	// If every action is atomic, then this will be equal to actionDepth
	// Every non-determinism includes a fork, so this will be greater than actionDepth
	// Note: This is the shorted path to this node from the root as we do BFS.
	forkDepth  int
	stacktrace string

	// ancestors map is used to detect cycles in the graph.
	// TODO(jp): Should this be an array instead?
	ancestors map[string]bool
}

type Link struct {
	Node *Node
	Name string
	Labels   []string
	Fairness ast.FairnessLevel
}

func NewNode(process *Process) *Node {
	return &Node{
		Process:     process,
		Inbound:     make([]*Link, 0, 10),
		Outbound:    make([]*Link, 0, 10),
		actionDepth: 0,
		forkDepth:   0,
		stacktrace:  captureStackTrace(),
		ancestors:   make(map[string]bool),
	}
}

func (n *Node) Duplicate(other *Node) {
	if !n.Enabled {
		return
	}
	parent := n.Inbound[0].Node
	other.Inbound = append(other.Inbound, n.Inbound[0])
	parent.Outbound = append(parent.Outbound, &Link{
		Node:     other,
		Name:     n.Inbound[0].Name,
		Labels:   n.Inbound[0].Labels,
		Fairness: n.Inbound[0].Fairness,
	})
	maps.Copy(other.ancestors, n.ancestors)
}

func (n *Node) Stutter() {
	//n.Outbound = append(n.Outbound, &Link{Node: n, Name: "stutter"})
	//n.Inbound = append(n.Inbound, &Link{Node: n, Name: "stutter"})
}

func (n *Node) Attach() {
	if len(n.Inbound) == 0 {
		return
	}
	parent := n.Inbound[0].Node
	parent.Outbound = append(parent.Outbound, &Link{
		Node:     n,
		Name:     n.Inbound[0].Name,
		Labels:   n.Inbound[0].Labels,
		Fairness: n.Inbound[0].Fairness,
	})
}

func (n *Node) ForkForAction(process *Process, action *ast.Action) *Node {
	if process == nil {
		process = n.Process
	}
	// Creates a new node, that can potentially be a child node. There is a chance, after executing
	// this node will lead to a duplicate state. To avoid adding and then replacing later, we will
	// create the node and add it to the queue, but do not add the outbound link from the parent node.
	// If the node leads to duplicate state, it will eventually call Duplicate(), else Attach()
	// that will add to the appropriate outbound link to the parent node.
	forkNode := &Node{
		Process:     process.Fork(),
		Inbound:     make([]*Link, 0, 10),
		Outbound:    make([]*Link, 0, 10),
		actionDepth: n.actionDepth + 1,
		forkDepth:   n.forkDepth + 1,
		stacktrace:  captureStackTrace(),
		ancestors:   maps.Clone(n.ancestors),
	}
	forkNode.Process.Name = action.Name
	forkNode.Inbound = append(forkNode.Inbound, &Link{Node: n, Name: action.Name})
	forkNode.Process.Stats.Increment(action.Name)
	forkNode.ancestors[n.HashCode()] = true
	return forkNode
}

func (n *Node) ForkForAlternatePaths(process *Process, name string) *Node {

	forkNode := &Node{
		Process:     process,
		Inbound:     make([]*Link, 0, 10),
		Outbound:    make([]*Link, 0, 10),
		actionDepth: n.actionDepth,
		forkDepth:   n.forkDepth + 1,
		stacktrace:  captureStackTrace(),
		ancestors:   maps.Clone(n.ancestors),
	}
	forkNode.Inbound = append(forkNode.Inbound, &Link{Node: n, Name: name})
	forkNode.ancestors[n.HashCode()] = true
	return forkNode
}

type Processor struct {
	Init    *Node
	Files   []*ast.File
	queue   *lib.Queue[*Node]
	visited map[string]*Node
	config  *ast.StateSpaceOptions
}

func NewProcessor(files []*ast.File, options *ast.StateSpaceOptions) *Processor {
	return &Processor{
		Files:   files,
		queue:   lib.NewQueue[*Node](),
		visited: make(map[string]*Node),
		config:  options,
	}
}

func (p *Processor) GetVisitedNodesCount() int {
	return len(p.visited)
}
// Start the model checker
func (p *Processor) Start() (init *Node, failedNode *Node, err error) {
	// recover from panic
	//defer func() {
	//	if r := recover(); r != nil {
	//		if modelErr, ok := r.(*ModelError); ok {
	//			err = modelErr
	//			return
	//		}
	//		panic(r)
	//	}
	//}()
	if p.Init != nil {
		panic("processor already started")
	}
	startTime := time.Now()
	process := NewProcess("init", p.Files, nil)

	p.Init = NewNode(process)
	init = p.Init

	if p.Files[0].Actions[0].Name != "Init" {
		globals, err := process.Evaluator.ExecInit(p.Files[0].States)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error in executing init: ", p.Files[0].States, err)
			panic(err)
		}
		process.Enable()
		process.Heap.globals = globals
		failed := CheckInvariants(process)
		if len(failed[0]) > 0 {
			p.Init.Process.FailedInvariants = failed
			if !p.config.ContinuePathOnInvariantFailures {
				return p.Init, p.Init, nil
			}
		}
		process.NewThread()
	} else {
		// This is init node
		action := p.Files[0].Actions[0]

		thread := p.Init.Process.NewThread()
		thread.currentFrame().pc = fmt.Sprintf("Actions[%d]", 0)
		thread.currentFrame().Name = action.Name
		p.Init.Name = action.Name
	}

	_ = p.queue.Push(p.Init)
	prevCount := 0
	for p.queue.Count() != 0 {
		found, node := p.queue.Pop()
		if !found {
			panic("queue should not be empty")
		}
		//process := node.Process
		//if other, ok := p.visited[process.HashCode()]; ok {
		//	node.Merge(other)
		//	continue
		//}

		if node.actionDepth > int(p.config.Options.MaxActions) {
			// Add a node to indicate why this node was not processed
			continue
		}
		if len(p.visited)%20000 == 0 && len(p.visited) != prevCount {
			fmt.Printf("Nodes: %d, elapsed: %s\n", len(p.visited), time.Since(startTime))
			prevCount = len(p.visited)
		}

		invariantFailure := p.processNode(node)
		if p.visited[node.HashCode()] == nil {
			p.visited[node.HashCode()] = node
		}

		if invariantFailure && failedNode == nil {
			failedNode = node
		}
		if invariantFailure && !p.config.ContinueOnInvariantFailures {
			break
		}
	}
	fmt.Printf("Nodes: %d, elapsed: %s\n", len(p.visited), time.Since(startTime))
	return p.Init, failedNode, err
}

func (p *Processor) processNode(node *Node) bool {
	if node.Process.currentThread().currentPc() == "" && node.Name == "init" {
		if node.Process.Files[0].Actions[0].Name != "Init" {
			return p.processInit(node)
		}

	}
	forks, yield := node.currentThread().Execute()
	// Add the labels from the process to the inbound links
	// This must be done even for duplicate nodes
	// The labels for the outbound links are added when the node is merged/attached
	if len(node.Inbound) > 0 {
		node.Inbound[0].Labels = append(node.Inbound[0].Labels, node.Process.Labels...)
		node.Inbound[0].Fairness = node.Process.Fairness
	}

	// If the node is already visited, merge the nodes and return
	// In this case, we are skipping checking invariants as well.
	// Reevaluate if this is the right thing to do. Invariants are checked only
	// if they are in yield point, but yield point is not part of the hash code.
	// So, we might miss some invariants. However, since the yield points are
	// determined by the statement, and we include program counter in the hash code,
	// this may not be an issue.
	if other, ok := p.visited[node.HashCode()]; ok {
		// Check if visited before scheduling children
		node.Duplicate(other)
		//if other.ancestors[node.Inbound[0].Node.HashCode()] {
		//	fmt.Println("Cycle detected")
		//	// TODO: Check if we can find the liveness here, incrementally.
		//	// Naively calling the liveness checker here will make it very
		//	// slow and expensive.
		//}
		return false
	} else {
		node.Attach()
	}

	var failedInvariants map[int][]int
	if yield {
		failedInvariants = CheckInvariants(node.Process)
	}
	if len(failedInvariants[0]) > 0 {
		//panic(fmt.Sprintf("Invariant failed: %v", failedInvariants))
		node.Process.FailedInvariants = failedInvariants
		if !p.config.ContinuePathOnInvariantFailures {
			return true
		}
	}
	if !yield {
		for _, fork := range forks {
			newNode := node.ForkForAlternatePaths(fork, "")
			_ = p.queue.Push(newNode)
		}
		return false
	}

	if yield {
		if len(forks) > 0 {
			//fmt.Println("yield and fork at the same time")
			for _, fork := range forks {
				p.YieldFork(node, fork)
			}
		} else {
			p.YieldNode(node)
			node.Name = "yield"
		}
		node.Stutter()
		if len(node.Process.Threads) == 0 {
			return false
		}
		crashFork := node.Process.Fork()
		crashFork.Name = "crash"
		crashFork.removeCurrentThread()
		crashNode := node.ForkForAlternatePaths(crashFork, "crash")
		// TODO: We could just copy the failed invariants from the parent
		// instead of checking again
		CheckInvariants(crashFork)
		crashNode.Attach()
		crashNode.Stutter()

		//if other, ok := p.visited[node.HashCode()]; ok {
		//	// Check if visited before scheduling children
		//	node.Duplicate(other)
		//	return false
		//} else {
		//	node.Attach()
		//}
		p.YieldNode(crashNode)
		return false
	}
	return false
}

func (p *Processor) processInit(node *Node) bool {
	node.Stutter()
	node.Process.removeCurrentThread()
	// This is init node, generate a fork for each action in the file
	for i, action := range p.Files[0].Actions {
		newNode := node.ForkForAction(nil, action)
		//newNode.Process.removeCurrentThread()
		thread := newNode.Process.NewThread()
		//thread := newNode.currentThread()
		thread.currentFrame().pc = fmt.Sprintf("Actions[%d]", i)
		thread.currentFrame().Name = action.Name
		_ = p.queue.Push(newNode)
	}
	return false
}

func (p *Processor) YieldNode(node *Node) {

	for i, thread := range node.Threads {
		if thread.currentPc() == "" {
			continue
		}
		name := fmt.Sprintf("thread-%d", i)
		newNode := node.ForkForAlternatePaths(thread.Process.Fork(), name)
		newNode.Current = i

		_ = p.queue.Push(newNode)
	}

	if node.actionDepth >= int(p.config.Options.MaxActions) ||
		len(node.Threads) >= int(p.config.Options.MaxConcurrentActions) {
		return
	}
	for i, action := range p.Files[0].Actions {
		if action.Name == "Init" {
			continue
		}
		if p.config.ActionOptions[action.Name] != nil &&
			node.Stats.Counts[action.Name] >= int(p.config.ActionOptions[action.Name].MaxActions) {
			continue
		}
		newNode := node.ForkForAction(nil, action)
		newNode.Process.NewThread()
		newNode.Process.Current = len(newNode.Process.Threads) - 1
		newNode.currentThread().currentFrame().pc = fmt.Sprintf("Actions[%d]", i)
		newNode.currentThread().currentFrame().Name = action.Name

		_ = p.queue.Push(newNode)
	}
}

func (p *Processor) YieldFork(node *Node, process *Process) {
	for i, thread := range process.Threads {
		if thread.currentPc() == "" {
			continue
		}
		name := fmt.Sprintf("thread-%d", i)
		newNode := node.ForkForAlternatePaths(thread.Process.Fork(), name)
		newNode.Current = i

		_ = p.queue.Push(newNode)
	}
	if node.actionDepth >= int(p.config.Options.MaxActions) ||
		len(process.Threads) >= int(p.config.Options.MaxConcurrentActions) {

		return
	}
	for i, action := range p.Files[0].Actions {
		if action.Name == "Init" {
			continue
		}
		if p.config.ActionOptions[action.Name] != nil &&
			process.Stats.Counts[action.Name] >= int(p.config.ActionOptions[action.Name].MaxActions) {
			continue
		}
		newNode := node.ForkForAction(process, action)
		newNode.Process.NewThread()
		newNode.Process.Current = len(newNode.Process.Threads) - 1
		newNode.currentThread().currentFrame().pc = fmt.Sprintf("Actions[%d]", i)
		newNode.currentThread().currentFrame().Name = action.Name

		_ = p.queue.Push(newNode)
	}
}

func captureStackTrace() string {
	if !enableCaptureStackTrace {
		return ""
	}
	const depth = 32
	var pcs [depth]uintptr
	n := runtime.Callers(2, pcs[:])
	if n == 0 {
		return "Unable to capture stack trace"
	}

	var sb strings.Builder
	frames := runtime.CallersFrames(pcs[:n])
	for {
		frame, more := frames.Next()
		fmt.Fprintf(&sb, "- %s:%d %s\n", frame.File, frame.Line, frame.Function)
		if !more {
			break
		}
	}

	return sb.String()
}
