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
	"github.com/fizzbee-io/fizzbee/lib"
	"github.com/golang/protobuf/proto"
	"github.com/jayaprabhakar/go-clone"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/syntax"
	"log"
	"maps"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// DefType is a custom enum-like type
type DefType string

const (
	Function DefType = "function"
)

var forkLock sync.Mutex

const enableCaptureStackTrace = false

type Definition struct {
	DefType    DefType
	name       string
	fileIndex  int
	path   string
	params []*ast.Parameter
	roleIndex int
	roleName string
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


func HandleModules(path string) map[string]starlark.Value {
	//module_file := path + "/sample.star"
	files, err := filepath.Glob(path + "/*.star")
	if err != nil {
		log.Fatal(err)
	}
	modules := make(map[string]starlark.Value)
	for _, file := range files {
		fmt.Println(file)
		moduleName := getFileNameWithoutExt(file)
		module := LoadModule(file)
		modules[moduleName] = &starlarkstruct.Module{Name: moduleName, Members: module}
	}
	return modules
}

func getFileNameWithoutExt(filePath string) string {
	// Extract filename with extension
	fileNameWithExt := filepath.Base(filePath)

	// Get filename without extension
	return fileNameWithExt[:len(fileNameWithExt)-len(filepath.Ext(fileNameWithExt))]
}


func LoadModule(moduleFile string) starlark.StringDict {
	thread := &starlark.Thread{
		Print: func(_ *starlark.Thread, msg string) { fmt.Println(msg) },
	}
	options := &syntax.FileOptions{Set: true, GlobalReassign: true, TopLevelControl: true}
	predeclared := starlark.StringDict{}
	globals, err := starlark.ExecFileOptions(options, thread, moduleFile, nil, predeclared)
	if err != nil {
		panic(err)
		return nil
	}
	return globals
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
	Messages    []*ast.Message          `json:"-"`

	// Fairness is actually a property of the transition/link. But to determine whether
	// the link is fair, we need to know if the process stepped through at least one
	// fair statement. To determine that, each thread maintains the fairness level
	// of the action that started. If that thread executed a statement, that process becomes fair,
	// that in-turn makes the link fair.
	Fairness    ast.FairnessLevel      `json:"-"`

	Enabled		bool                   `json:"-"`

	Roles 	    []*lib.Role `json:"roles"`

	CachedHashCode string              `json:"-"`

	Modules	 map[string]starlark.Value `json:"-"`
	EnableCheckpoint bool 		  `json:"-"`
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
					params:    function.Params,
					fileIndex: i,
					path:      fmt.Sprintf("Functions[%d]", j),
				}
			}
			for r, role := range file.Roles {
				for j, function := range role.Functions {
					symbolTable[role.Name + "." + function.Name] = &Definition{
						DefType:   Function,
						name:      function.Name,
						params:    function.Params,
						fileIndex: i,
						roleIndex: r,
						roleName:  role.Name,
						path:      fmt.Sprintf("Roles[%d].Functions[%d]", r, j),
					}
				}
			}
		}
	} else {
		mc = parent.Evaluator
		symbolTable = parent.SymbolTable
	}
	p := &Process{
		Name:        name,
		Heap:        &Heap{starlark.StringDict{}, starlark.StringDict{}},
		Threads:     []*Thread{},
		Current:     0,
		Files:       files,
		Parent:      parent,
		Evaluator:   mc,
		Children:    []*Process{},
		Returns:     make(starlark.StringDict),
		SymbolTable: symbolTable,
		Labels:      make([]string, 0),
		Messages:    make([]*ast.Message, 0),
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
		"roles":     p.Roles,
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
	// SetCustomPtrFunc and SetCustomFunc changes the global state,
	// so while the clone is in progress this should not be changed :(
	// There is github issue to fix this in the clone library
	// The only place the clone library is used is to clone the Threads (for the CallStack),
	// this could probably be pushed down to minimize contention
	forkLock.Lock()
	defer forkLock.Unlock()

	refs := make(map[string]*lib.Role)
	clone.SetCustomPtrFunc(reflect.TypeOf(&lib.Role{}), roleResolveCloneFn(refs, nil, 0))
	clone.SetCustomFunc(reflect.TypeOf(starlark.Set{}), starlarkSetResolveFn(refs, nil, 0))
	clone.SetCustomFunc(reflect.TypeOf(starlark.Dict{}), starlarkDictResolveFn(refs, nil, 0))
	p2 := &Process{
		Name:        p.Name,
		Heap:        p.Heap.Clone(refs, nil, 0),
		Current:     p.Current,
		Parent:      p,
		Evaluator:   p.Evaluator,
		Children:    []*Process{},
		Files:       p.Files,
		Returns:     make(starlark.StringDict),
		SymbolTable: p.SymbolTable,
		Modules: 	 p.Modules,
		Labels:      make([]string, 0),
		Messages: 	 make([]*ast.Message, 0),
		Stats:       p.Stats.Clone(),
	}
	p2.Witness = make([][]bool, len(p.Files))
	for i, file := range p.Files {
		p2.Witness[i] = make([]bool, len(file.Invariants))
	}

	p.Children = append(p.Children, p2)
	clonedThreads := make([]*Thread, len(p.Threads))
	for i, thread := range p.Threads {
		clonedThreads[i] = thread.Clone(nil, 0)
		clonedThreads[i].Process = p2
	}
	p2.Threads = clonedThreads
	p2.Roles = MapRoleValuesInOrder(refs, p.Roles)

	return p2
}

func (p *Process) CloneForAssert(permutations map[lib.SymmetricValue][]lib.SymmetricValue, alt int) *Process {
	// SetCustomPtrFunc and SetCustomFunc changes the global state,
	// so while the clone is in progress this should not be changed :(
	// There is github issue to fix this in the clone library
	// The only place the clone library is used is to clone the Threads (for the CallStack),
	// this could probably be pushed down to minimize contention
	forkLock.Lock()
	defer forkLock.Unlock()

	refs := make(map[string]*lib.Role)
	clone.SetCustomPtrFunc(reflect.TypeOf(&lib.Role{}), roleResolveCloneFn(refs, permutations, alt))
	clone.SetCustomFunc(reflect.TypeOf(starlark.Dict{}), starlarkDictResolveFn(refs, permutations, alt))
	clone.SetCustomFunc(reflect.TypeOf(starlark.Set{}), starlarkSetResolveFn(refs, permutations, alt))
	clone.SetCustomFunc(reflect.TypeOf(lib.SymmetricValue{}), symmetricValueResolveFn(refs, permutations, alt))
	p2 := &Process{
		Name:        p.Name,
		Heap:        p.Heap.Clone(refs, permutations, alt),
		Current:     p.Current,
		Parent:      p,
		Evaluator:   p.Evaluator,
		Children:    []*Process{},
		Files:       p.Files,
		Returns:     make(starlark.StringDict),
		SymbolTable: p.SymbolTable,
		Modules: 	 p.Modules,
		Labels:      make([]string, 0),
		Messages: 	 make([]*ast.Message, 0),
		Stats:       p.Stats.Clone(),
	}
	p2.Witness = make([][]bool, len(p.Files))
	for i, file := range p.Files {
		p2.Witness[i] = make([]bool, len(file.Invariants))
	}

	clonedThreads := make([]*Thread, len(p.Threads))
	for i, thread := range p.Threads {
		clonedThreads[i] = thread.Clone(permutations, alt)
		clonedThreads[i].Process = p2
	}
	p2.Threads = clonedThreads
	p2.Roles = MapRoleValuesInOrder(refs, p.Roles)
	return p2
}

// MapRoleValuesInOrder returns the values of the map m.
// The values will be in an indeterminate order.
func MapRoleValuesInOrder(m map[string]*lib.Role, oldList []*lib.Role) []*lib.Role {
	r := make([]*lib.Role, 0, len(m))
	for _, v := range oldList {
		r = append(r, m[v.RefString()])
	}
	return r
}

func (p *Process) Enable() {
	if !p.Enabled {
		p.propagateEnabled()
	}
	p.Enabled = true
}

func (p *Process) propagateEnabled() {
	if !p.Enabled {
		return
	}
	parent := p.Parent
	for parent != nil && len(parent.Threads) != 0 && !parent.Enabled {
		parent.Enabled = true
		parent = parent.Parent

	}
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
	str := buf.String()
	str = strings.ReplaceAll(str, lib.SymmetryPrefix, "")
	return str
}

func (n *Node) MarshalJSON() ([]byte, error) {
	return lib.MarshalJSON(n.Process)
}

func (n *Node) GetJsonString() string {
	bytes, err := lib.MarshalJSON(n.Process)
	if err != nil {
		fmt.Println("Error:", err)
		return ""
	}
	str := string(bytes)
	str = strings.ReplaceAll(str, lib.SymmetryPrefix, "")
	return str
}

func (n *Node) GetStateString() string {
	buf := &strings.Builder{}
	n.appendState(n.Process, buf)
	return buf.String()
}
func (n *Node) appendState(p *Process, buf *strings.Builder) {
	if len(p.Heap.state) > 0 {
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
	if p.CachedHashCode != "" {
		return p.CachedHashCode
	}
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
	p.CachedHashCode = fmt.Sprintf("%x", h.Sum(nil))
	return p.CachedHashCode
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
	// Shallow clone the globals
	dict := maps.Clone(p.Heap.globals)

	roleRefs := make(map[string]*lib.Role)
	for i, role := range p.Roles {
		roleRefs[role.RefString()] = p.Roles[i]
	}

	CopyDict(p.Heap.state, dict, roleRefs, nil, 0)
	frame := p.currentThread().currentFrame()
	if frame.obj != nil {
		self, err := deepCloneStarlarkValue(frame.obj, roleRefs)
		if err != nil {
			panic(err)
		}
		dict["self"] = self
	}
	CopyDict(frame.vars, dict, roleRefs, nil, 0)
	frame.scope.getAllVisibleVariablesResolveRoles(dict, roleRefs)
	maps.Copy(dict, lib.Builtins)
	dict["deepcopy"] = starlark.NewBuiltin("deepcopy", DeepCopyBuiltIn)
	maps.Copy(dict, p.Modules)

	for _, file := range p.Files {
		for _, role := range file.Roles {
			symmetric := slices.Contains(role.Modifiers, "symmetric")
			dict[role.Name] = lib.CreateRoleBuiltin(role.Name, symmetric, &p.Roles)
		}
	}
	return dict
}

// GetAllVariablesNocopy returns all variables visible in the Current thread, without deep copying.
// This includes state variables and variables from the Current thread's variables in the top call frame
func (p *Process) GetAllVariablesNocopy() starlark.StringDict {
	// Shallow clone the globals
	dict := maps.Clone(p.Heap.globals)

	maps.Copy(dict, p.Heap.state)
	frame := p.currentThread().currentFrame()
	if frame.obj != nil {
		dict["self"] = frame.obj
	}
	maps.Copy(dict, frame.vars)
	frame.scope.getAllVisibleVariablesToDictNoCopy(dict)

	maps.Copy(dict, lib.Builtins)
	dict["deepcopy"] = starlark.NewBuiltin("deepcopy", DeepCopyBuiltIn)
	maps.Copy(dict, p.Modules)

	for _, file := range p.Files {
		for _, role := range file.Roles {
			symmetric := slices.Contains(role.Modifiers, "symmetric")
			dict[role.Name] = lib.CreateRoleBuiltin(role.Name, symmetric, &p.Roles)
		}
	}
	return dict
}

func (p *Process) updateAllVariablesInScope(dict starlark.StringDict) {
	frame := p.currentThread().currentFrame()
	for k, v := range dict {
		p.updateVariableInternal(k, v, frame)
	}
}

func (p *Process) updateVariable(key string, val starlark.Value) {
	frame := p.currentThread().currentFrame()
	p.updateVariableInternal(key, val, frame)
}

func (p *Process) updateVariableInternal(key string, val starlark.Value, frame *CallFrame) {
	if p.updateScopedVariable(frame.scope, key, val) {
		// Check local variables in the scope, starting from
		// deepest to its parent. If present, update that
		// and continue
		return
	}
	if p.Heap.update(key, val) {
		// if no scoped variable exists, check if it is state
		// variable, then update the state variable
		return
	}
	if p.Heap.globals.Has(key) {
		return
	}
	if key == "self" {
		frame.obj = val.(*lib.Role)
		return
	}
	if val.Type() == "builtin_function_or_method" || val.Type() == "module" {
		return
	}
	// Declare the variable to the Current scope
	frame.scope.vars[key] = val
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

func (p *Process) NewModelError(sourceInfo *ast.SourceInfo, msg string, nestedError error) *ModelError {
	return NewModelError(sourceInfo, msg, p, nestedError)
}

func (p *Process) PanicOnError(sourceInfo *ast.SourceInfo, msg string, nestedError error)  {
	if nestedError != nil {
		panic(p.NewModelError(sourceInfo, msg, nestedError))
	}
}

func (p *Process) PanicIfFalse(ok bool, sourceInfo *ast.SourceInfo, msg string) {
	if !ok {
		panic(p.NewModelError(sourceInfo, msg, nil))
	}
}

func (p *Process) RecordCall(callerFrame *CallFrame, receiverFrame *CallFrame, flow ast.Flow) {
	if (callerFrame.obj == nil && receiverFrame.obj == nil) || callerFrame.obj == receiverFrame.obj {
		return
	}
	msg := &ast.Message{
		Name: receiverFrame.Name,
	}
	if callerFrame.obj != nil {
		msg.Sender = callerFrame.obj.RefStringShort()
	}
	if receiverFrame.obj != nil {
		msg.Receivers = []string{receiverFrame.obj.RefStringShort()}
	}
	for name, value := range receiverFrame.vars {
		msg.Values = append(msg.Values, &ast.NameValue{Name: name, Value: value.String()})
	}
	if flow != ast.Flow_FLOW_ATOMIC {
		msg.Lossy = true
	}
	p.Messages = append(p.Messages, msg)
}

func (p *Process) RecordReturn(callerFrame *CallFrame, receiverFrame *CallFrame, val starlark.Value, flow ast.Flow) {
	if (callerFrame.obj == nil && receiverFrame.obj == nil) || callerFrame.obj == receiverFrame.obj {
		return
	}
	msg := &ast.Message{
		Name: receiverFrame.Name,
		IsReturn: true,
	}

	if callerFrame.obj != nil {
		msg.Sender = callerFrame.obj.RefStringShort()
	}
	if receiverFrame.obj != nil {
		msg.Receivers = []string{receiverFrame.obj.RefStringShort()}
	}
	if val != nil {
		msg.Values = append(msg.Values, &ast.NameValue{Value: val.String()})
	}
	if flow != ast.Flow_FLOW_ATOMIC {
		msg.Lossy = true
	}
	p.Messages = append(p.Messages, msg)
}

type Node struct {
	*Process `json:"process"`

	Inbound  []*Link `json:"-"`
	Outbound []*Link `json:"-"`

	// The number of actions started until this node
	// Note: This is the shorted path to this node from the root as we do BFS.
	actionDepth int

	// The number of forks until this node from the root. This will be >= actionDepth
	// If every action is atomic, then this will be equal to actionDepth
	// Every non-determinism includes a fork, so this will be greater than actionDepth
	// Note: This is the shorted path to this node from the root as we do BFS.
	forkDepth  int
	stacktrace string

	DuplicateOf *Node
}

type Link struct {
	Node     *Node
	Type     string
	Name     string
	Labels   []string
	Fairness ast.FairnessLevel
	Messages []*ast.Message
}

func NewNode(process *Process) *Node {
	return &Node{
		Process:     process,
		Inbound:     make([]*Link, 0, 10),
		Outbound:    make([]*Link, 0, 10),
		actionDepth: 0,
		forkDepth:   0,
		stacktrace:  captureStackTrace(),
	}
}

func (n *Node) Duplicate(other *Node, yield bool) {
	if yield && !n.Enabled {
		return
	}
	parent := n.Inbound[0].Node
	other.Inbound = append(other.Inbound, n.Inbound[0])
	parent.Outbound = append(parent.Outbound, &Link{
		Node:     other,
		Type: 	  n.Inbound[0].Type,
		Name:     n.Inbound[0].Name,
		Labels:   n.Inbound[0].Labels,
		Fairness: n.Inbound[0].Fairness,
		Messages: n.Inbound[0].Messages,
	})

	n.Process = nil
	n.Inbound = nil
	n.Outbound = nil
	n.DuplicateOf = other
}


func (n *Node) Attach() {
	if len(n.Inbound) == 0 {
		return
	}
	parent := n.Inbound[0].Node
	parent.Outbound = append(parent.Outbound, &Link{
		Node:     n,
		Type: 	  n.Inbound[0].Type,
		Name:     n.Inbound[0].Name,
		Labels:   n.Inbound[0].Labels,
		Fairness: n.Inbound[0].Fairness,
		Messages: n.Inbound[0].Messages,
	})
}

func (n *Node) ForkForAction(process *Process, role *lib.Role, action *ast.Action) *Node {
	if process == nil {
		process = n.Process
	}
	actionName := action.Name
	if role != nil {
		actionName = role.RefStringShort() + "." + actionName
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
	}
	forkNode.Process.Name = actionName
	forkNode.Inbound = append(forkNode.Inbound, &Link{Node: n, Type: "action", Name: actionName})
	forkNode.Process.Stats.Increment(actionName)
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
	}
	forkNode.Inbound = append(forkNode.Inbound, &Link{Node: n, Name: name})
	return forkNode
}

type Processor struct {
	Init                *Node
	Files               []*ast.File
	queue               lib.LinearCollection[*Node]
	visited             map[string]*Node
	config              *ast.StateSpaceOptions
	stopped             bool
	dirPath             string
	intermediate_states lib.LinearCollection[*Node]
	simulation          bool
	random rand.Rand
	Seed   int64
}

func NewProcessor(files []*ast.File, options *ast.StateSpaceOptions, simulation bool, seed int64, dirPath string) *Processor {

	var collection lib.LinearCollection[*Node]
	var intermediate_states lib.LinearCollection[*Node]
	if seed == 0 {
		seed = time.Now().UnixMicro()
	}
	random := *rand.New(rand.NewSource(seed))
	if simulation {
		collection = lib.NewRandomQueue[*Node](random)
		intermediate_states = lib.NewRandomQueue[*Node](random)
	} else {
		collection = lib.NewQueue[*Node]()
		intermediate_states = lib.NewQueue[*Node]()
	}
	lib.ClearRoleRefs()
	return &Processor{
		Files:   files,
		queue:   collection,
		visited: make(map[string]*Node),
		config:  proto.Clone(options).(*ast.StateSpaceOptions),
		dirPath: dirPath,

		intermediate_states: intermediate_states,
		simulation:          simulation,
		random:              random,
		Seed:                seed,
	}
}

func (p *Processor) GetVisitedNodesCount() int {
	return len(p.visited)
}


func (p *Processor) InitializeNode() (*Node, *Node, error) {
	process := NewProcess("init", p.Files, nil)

	modules := make(map[string]starlark.Value)
	if p.dirPath != "" {
		modules = HandleModules(p.dirPath)
	}
	process.Modules = modules
	p.Init = NewNode(process)

	if len(p.Files[0].Stmts) > 0 {
		processPreInit(p.Init, p.Files[0].Stmts)
	}

	if p.Files[0].Actions[0].Name != "Init" {
		globals, err := process.Evaluator.ExecInit(p.Files[0].States)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error in executing init: ", p.Files[0].States, err)
			panic(err)
		}
		process.Enable()
		process.Heap.state = globals
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
	return p.Init, nil, nil
}

// Start the model checker
func (p *Processor) Start() (init *Node, failedNode *Node, err error) {
	if p.simulation {
		return p.StartSimulation()
	}
	if p.Init != nil {
		panic("processor already started")
	}
	startTime := time.Now()
	init, failedNode, err = p.InitializeNode()
	if err != nil {
		return init, failedNode, err
	}

	p.queue.Add(p.Init)
	prevCount := 0
	for p.queue.Len() != 0 && !p.stopped {
		node, found := p.queue.Remove()
		if !found {
			panic("queue should not be empty")
		}

		if node.actionDepth > int(p.config.Options.MaxActions) {
			// Add a node to indicate why this node was not processed
			continue
		}

		invariantFailure := false
		symmetryFound := false
		for true {
			if len(p.visited)%20000 == 0 && len(p.visited) != prevCount {
				fmt.Printf("Nodes: %d, queued: %d, elapsed: %s\n", len(p.visited), p.queue.Len(), time.Since(startTime))
				prevCount = len(p.visited)
			}
			invariantFailure, symmetryFound = p.processNode(node)

			if p.intermediate_states.Len() == 0 {
				break
			}
			node, _ = p.intermediate_states.Remove()
		}

		if symmetryFound {
			continue
		}

		if invariantFailure && failedNode == nil {
			failedNode = node
		}
		if invariantFailure && !p.config.ContinueOnInvariantFailures {
			break
		}
	}
	fmt.Printf("Nodes: %d, queued: %d, elapsed: %s\n", len(p.visited), p.queue.Len(), time.Since(startTime))
	return p.Init, failedNode, err
}

func (p *Processor) StartSimulation() (init *Node, failedNode *Node, err error) {
	if p.Init != nil {
		panic("processor already started")
	}
	init, failedNode, err = p.InitializeNode()
	if err != nil || failedNode != nil {
		return init, failedNode, err
	}
	livenessEnabled := false
	livenessNode := failedNode
	if p.config.GetLiveness() == "" || p.config.GetLiveness() == "strict" {
		for _, file := range p.Files {
			for _, invariant := range file.Invariants {
				if invariant.Eventually || slices.Contains(invariant.TemporalOperators, "eventually")  {
					livenessEnabled = true
					break
				}
			}
		}
	}
	maxActions := p.config.Options.MaxActions
	crashOnYield := p.config.Options.CrashOnYield
	randMaxActions := maxActions
	//fmt.Println("Options", p.config.Options)
	if livenessEnabled {
		randMaxActions = p.random.Int63n(maxActions)
		p.config.Options.MaxActions = randMaxActions
		defer func() {
			p.config.Options.MaxActions = maxActions
			p.config.Options.CrashOnYield = crashOnYield
		}()
	}

	//fmt.Println("MaxActions:", p.config.Options.MaxActions, "Seed:", p.Seed)

	p.queue.Add(p.Init)
	liveness := false
	for p.queue.Len() != 0 && !p.stopped {
		node, found := p.queue.Remove()
		if !found {
			panic("queue should not be empty")
		}

		if livenessEnabled && !liveness && node.actionDepth > int(p.config.Options.MaxActions - 1)  {
			//fmt.Println("Max actions reached, switching to liveness", p.config.Options.MaxActions)
			liveness = true
			p.config.Options.MaxActions = 2*maxActions
			*p.config.Options.CrashOnYield = false
		} else if (liveness || !livenessEnabled) && node.actionDepth > int(p.config.Options.MaxActions) {
			//fmt.Println("Max actions reached", p.config.Options.MaxActions)
			continue
		}
		if liveness && node.actionDepth > 0  && node.actionDepth > int(randMaxActions) {
			if node.currentThread().Fairness == ast.FairnessLevel_FAIRNESS_LEVEL_UNFAIR ||
				node.currentThread().Fairness == ast.FairnessLevel_FAIRNESS_LEVEL_UNKNOWN {
				livenessNode = node
				continue
			}
		}

		invariantFailure := false
		symmetryFound := false
		prevLen := p.queue.Len()
		if !liveness || node.actionDepth <= int(maxActions) {
			p.visited = make(map[string]*Node)
		}
		for true {
			inCrashPath := false
			if len(node.Inbound) > 0 {
				if node.Inbound[0].Node.Name == "crash" {
					if liveness && node.actionDepth > int(randMaxActions) {
						var nonCrashNode *Node
						for p.queue.Len() != 0 {
							nonCrashNode, _ = p.queue.Remove()
							if nonCrashNode.Inbound[0].Node.Name == "crash" {
								continue
							}
							break
						}
						if nonCrashNode != nil {
							node = nonCrashNode
							continue
						} else {
							break
						}

					}
					inCrashPath = true
				}
			}
			invariantFailure, symmetryFound = p.processNode(node)

			if node.Process != nil && (node.Name == "yield" || node.Name == "crash") && p.simulation && (!inCrashPath || node.Enabled){
				p.intermediate_states.ClearAll()
				break
			}
			if !inCrashPath {
				if p.intermediate_states.Len() == 0 {
					break
				}
				node, _ = p.intermediate_states.Remove()
			} else {
				has_another_crash_action := false
				var anotherCrashNode *Node
				for p.queue.Len() != 0 {
					anotherCrashNode, _ = p.queue.Remove()
					if anotherCrashNode.Inbound[0].Node.Name == "crash" {
						has_another_crash_action = true
						break
					}
				}

				if !has_another_crash_action {
					prevLen = 0
					break
				} else {
					node = anotherCrashNode
					prevLen = p.queue.Len()
				}
			}


		}
		p.intermediate_states.ClearAll()

		if symmetryFound {
			continue
		}
		if invariantFailure && failedNode == nil {
			failedNode = node
		}
		if invariantFailure {
			break
		}
		if node.Process == nil && liveness {
			failedInvPos, failed, ok := p.checkLiveness(node.DuplicateOf)
			if !ok {
				failedNode = failed
				failedNode.FailedInvariants = make(map[int][]int)
				failedNode.FailedInvariants[failedInvPos.FileIndex] = []int{failedInvPos.InvariantIndex}
				break
			}

		}
		if p.simulation && node.Process != nil && node.Name == "yield" && node.Enabled {
			p.queue.Clear(prevLen)
		}

		if node.Process != nil && !node.Process.Enabled && prevLen == 0 && len(node.Inbound[0].Node.Outbound) == 0 {
			if !liveness && p.config.GetDeadlockDetection() {
				failedNode = node.Inbound[0].Node
				break
			} else {
				// During liveness check since we are skipping non-fair nodes, we may end up with no next steps
				// if this state matches all the required predicates, then it is a valid state. Otherwise, mark it as
				// stutter state.
				livenessNode = node
			}

		}
	}
	if liveness && livenessNode != nil {
		for i, file := range p.Files {
			for j, invariant := range file.Invariants {
				hasEventually := slices.Contains(invariant.TemporalOperators, "eventually") || invariant.Eventually
				if hasEventually {
						if !livenessNode.Inbound[0].Node.Process.Witness[i][j] {
							failedNode = livenessNode.Inbound[0].Node
							break
						}

				}
			}

		}
	}
	return p.Init, failedNode, err
}

func processPreInit(init *Node, stmts []*ast.Statement) {
	thread := init.NewThread()
	thread.currentFrame().pc = fmt.Sprintf("Stmts[%d]", 0)
	thread.currentFrame().Name = "toplevel"

	thread.InsertNewScope()
	thread.currentFrame().scope.flow = ast.Flow_FLOW_ATOMIC
	for _, stmt := range stmts {
		forks, yield := thread.executeStatement()
		if yield || len(forks) > 0 {
			panic("Not supported: No non-determinism at top level in stmt" + stmt.String())
		}
	}
	globals := thread.currentFrame().scope.GetAllVisibleVariables(nil)
	globals.Freeze()
	init.Process.Heap.globals = globals
	init.removeCurrentThread()
}

func (p *Processor) processNode(node *Node) (bool, bool) {
	if node.Process.currentThread().currentPc() == "" && node.Name == "init" {
		if node.Process.Files[0].Actions[0].Name != "Init" {
			return p.processInit(node), false
		}

	}
	node.CachedHashCode = ""
	forks, yield := node.currentThread().Execute()
	if len(forks) == 0 && !node.Enabled {
		return false, false
	}
	// Add the labels from the process to the inbound links
	// This must be done even for duplicate nodes
	// The labels for the outbound links are added when the node is merged/attached
	if len(node.Inbound) > 0 {
		node.Inbound[0].Labels = append(node.Inbound[0].Labels, node.Process.Labels...)
		node.Inbound[0].Fairness = node.Process.Fairness
		node.Inbound[0].Messages = append(node.Inbound[0].Messages, node.Process.Messages...)
	}

	// If the node is already visited, merge the nodes and return
	// In this case, we are skipping checking invariants as well.
	// Reevaluate if this is the right thing to do. Invariants are checked only
	// if they are in yield point, but yield point is not part of the hash code.
	// So, we might miss some invariants. However, since the yield points are
	// determined by the statement, and we include program counter in the hash code,
	// this may not be an issue.
	hashCode := node.HashCode()
	if other, ok := p.visited[hashCode]; ok {
		// This is a bit inefficient.
		// TODO: Enabled should be a property of the link/transition, not the node.
		// We will keep the enabled state in the node, during execution but have to be
		// copied to the link/transition when attaching/merging similar to Fairness.
		if other.Enabled || !node.Enabled {
			node.Duplicate(other, yield)
			return false, false
		} else {
			node.Attach()
			p.visited[hashCode] = node
		}

	} else {
		hashes := node.getSymmetryTranslations()
		for _, hash := range hashes {
			if other, ok := p.visited[hash]; ok {
				if other.Enabled || !node.Enabled {
					node.Duplicate(other, yield)
					return false, true
				}
			}
		}
		node.Attach()
		p.visited[hashCode] = node
	}

	p.visited[hashCode] = node
	var failedInvariants map[int][]int
	if yield {
		failedInvariants = CheckInvariants(node.Process)
	}
	if len(failedInvariants[0]) > 0 {
		//panic(fmt.Sprintf("Invariant failed: %v", failedInvariants))
		node.Process.FailedInvariants = failedInvariants
		if !p.config.ContinuePathOnInvariantFailures {
			return true, false
		}
	}
	if !yield {
		for _, fork := range forks {
			newNode := node.ForkForAlternatePaths(fork, fork.Name)
			p.intermediate_states.Add(newNode)
		}
		return false, false
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
		if len(node.Process.Threads) == 0 || !*p.config.Options.CrashOnYield {
			return false, false
		}
		frameName := node.Process.currentThread().Stack.RawArray()[0].Name

		if p.config.ActionOptions[frameName] != nil && p.config.ActionOptions[frameName].CrashOnYield != nil && !p.config.ActionOptions[frameName].GetCrashOnYield() {
			return false, false
		}

		crashFork := node.Process.Fork()
		crashFork.Name = "crash"
		crashFork.removeCurrentThread()
		crashNode := node.ForkForAlternatePaths(crashFork, "crash")
		// TODO: We could just copy the failed invariants from the parent
		// instead of checking again
		CheckInvariants(crashFork)
		if node.Process.Enabled {
			crashNode.Enable()
		}
		if other, ok := p.visited[crashNode.HashCode()]; ok {
			crashNode.Duplicate(other, true)
			return false, false
		}
		crashNode.Attach()


		//if other, ok := p.visited[node.HashCode()]; ok {
		//	// Check if visited before scheduling children
		//	node.Duplicate(other)
		//	return false
		//} else {
		//	node.Attach()
		//}
		p.YieldNode(crashNode)
		return false, false
	}
	return false, false
}

func (p *Process) getSymmetryTranslations() []string {
	permMap, count := getSymmetryPermutations(p)
	//src := permutations[0]
	hashes := make([]string, count-1)
	for i := 1; i < count; i++ {
		hashes[i-1] = p.symmetricHash(permMap, i)
	}
	return hashes
}

func (p *Process) symmetricHash(permutations map[lib.SymmetricValue][]lib.SymmetricValue, alt int) string {
	p2 := p.CloneForAssert(permutations, alt)
	return p2.HashCode()
}

func (p *Process) GetSymmetryRoles() []*lib.SymmetricValues {
	m := make(map[string][]lib.SymmetricValue)
	for _, role := range p.Roles {
		if role.IsSymmetric() {
			m[role.Name] = append(m[role.Name], lib.NewSymmetricValue(role.Name, role.Ref))
		}
	}
	roleSymValues := make([]*lib.SymmetricValues, 0, len(m))
	for _, values := range m {
		roleSymValues = append(roleSymValues, lib.NewSymmetricValues(values))
	}
	return roleSymValues
}

func getSymmetryPermutations(process *Process) (map[lib.SymmetricValue][]lib.SymmetricValue, int) {
	defs := process.Heap.GetSymmetryDefs()
	values := make([][]lib.SymmetricValue, len(defs))
	for i, def := range defs {
		values[i] = make([]lib.SymmetricValue, def.Len())
		for j := 0; j < def.Len(); j++ {
			values[i][j] = def.Index(j)
		}
		slices.SortFunc(values[i], lib.CompareStringer[lib.SymmetricValue])
	}

	roles := process.GetSymmetryRoles()
	for _, role := range roles {
		v := make([]lib.SymmetricValue, role.Len())
		for j := 0; j < role.Len(); j++ {
			v[j] = role.Index(j)
		}
		slices.SortFunc(v, lib.CompareStringer[lib.SymmetricValue])
		values = append(values, v)
	}
	permutations := lib.GenerateAllPermutations(values)
	v := make([][]lib.SymmetricValue, len(permutations))
	for i, permutation := range permutations {
		v[i] = slices.Concat(permutation...)
	}
	permMap := make(map[lib.SymmetricValue][]lib.SymmetricValue)
	for _, symVals := range v {
		for j, symVal := range symVals {
			permMap[v[0][j]] = append(permMap[v[0][j]], symVal)
		}
	}

	return permMap, len(v)
}

func (p *Processor) processInit(node *Node) bool {
	node.Process.removeCurrentThread()
	// This is init node, generate a fork for each action in the file
	for i, action := range p.Files[0].Actions {
		newNode := node.ForkForAction(nil, nil, action)
		//newNode.Process.removeCurrentThread()
		thread := newNode.Process.NewThread()
		//thread := newNode.currentThread()
		thread.currentFrame().pc = fmt.Sprintf("Actions[%d]", i)
		thread.currentFrame().Name = action.Name
		p.queue.Add(newNode)
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

		p.queue.Add(newNode)
	}

	if node.actionDepth >= int(p.config.Options.MaxActions) ||
		len(node.Threads) >= int(p.config.Options.MaxConcurrentActions) {
		return
	}

	for i, action := range p.Files[0].Actions {
		p.scheduleAction(node, nil, nil, 0, action, i)
	}
	if len(node.Roles) > 0 {
		p.scheduleRoleActions(node, nil)
	}

}

func (p *Processor) scheduleRoleActions(node *Node, process *Process) {
	roleMap := make(map[string]int)
	for i, role := range p.Files[0].Roles {
		roleMap[role.Name] = i
	}
	for _, role := range node.Roles {
		if _, ok := roleMap[role.Name]; !ok {
			panic("Role not found: " + role.Name)
		}
		index := roleMap[role.Name]
		roleAst := p.Files[0].Roles[index]
		for i, action := range roleAst.Actions {
			p.scheduleAction(node, process, role, index, action, i)

		}

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

		p.queue.Add(newNode)
	}
	if node.actionDepth >= int(p.config.Options.MaxActions) ||
		len(process.Threads) >= int(p.config.Options.MaxConcurrentActions) {

		return
	}
	for i, action := range p.Files[0].Actions {
		p.scheduleAction(node, process, nil, 0, action, i)
	}

	if len(node.Roles) > 0 {
		p.scheduleRoleActions(node, process)
	}
}

func (p *Processor) scheduleAction(node *Node, process *Process, role *lib.Role, roleIndex int,
	action *ast.Action, actionIndex int) {

	statProcess := process
	if process == nil {
		statProcess = node.Process
	}
	if action.Name == "Init" {
		return
	}
	if p.ExceedsActionCountLimits(action, statProcess, role) {
		return
	}

	newNode := node.ForkForAction(process, role, action)
	newNode.Process.NewThread()
	newNode.Process.Current = len(newNode.Process.Threads) - 1
	newNode.Process.Fairness = action.Fairness.GetLevel()
	newNode.currentThread().Fairness = action.Fairness.GetLevel()

	frame := newNode.currentThread().currentFrame()
	if role != nil {
		for _, r := range newNode.Roles {
			if r.RefStringShort() == role.RefStringShort() {
				frame.obj = r
				break
			}
		}
		if frame.obj == nil {
			// This happens when a node is removed from the heap. But we cannot remove the node from
			// the old node's `roles` list. So, we filter it out here.
			return
		}
		frame.pc = fmt.Sprintf("Roles[%d].Actions[%d]", roleIndex, actionIndex)
		frame.Name = role.Name + "." + action.Name
	} else {
		frame.pc = fmt.Sprintf("Actions[%d]", actionIndex)
		frame.Name = action.Name
	}

	p.queue.Add(newNode)
}

func (p *Processor) ExceedsActionCountLimits(action *ast.Action, statProcess *Process, role *lib.Role) bool {
	actionName := action.Name
	if role != nil {
		actionName = role.RefStringShort() + "." + actionName
	}
	concurrentStats := make(map[string]int)

	for _, thread := range statProcess.Threads {
		frames := thread.Stack.RawArray()
		rootFrame := frames[0]
		name := rootFrame.Name
		concurrentStats[name]++
		nameParts := strings.Split(name, ".")
		if len(nameParts) > 1 && rootFrame.obj != nil {
			concurrentStats[nameParts[0] + "#." + nameParts[1]]++
			concurrentStats[rootFrame.obj.RefStringShort() + "." + nameParts[1]]++
		}
	}
	//fmt.Println("Concurrent stats", concurrentStats, actionName, concurrentStats[actionName], p.config.ActionOptions[actionName])
	if p.config.ActionOptions[actionName] != nil &&
		int(p.config.ActionOptions[actionName].MaxActions) > 0 && statProcess.Stats.Counts[actionName] >= int(p.config.ActionOptions[actionName].MaxActions)  {
		return true
	}
	if p.config.ActionOptions[actionName] != nil &&
		int(p.config.ActionOptions[actionName].GetMaxConcurrentActions()) > 0 && concurrentStats[actionName] >= int(p.config.ActionOptions[actionName].GetMaxConcurrentActions())  {
		return true
	}
	if role == nil {
		return false
	}
	if p.config.ActionOptions[role.Name + "#." + action.Name] != nil {
		perRoleActionLimit := p.config.ActionOptions[role.Name + "#." + action.Name].MaxActions
		perRoleActionConcurrency := p.config.ActionOptions[role.Name + "#." + action.Name].GetMaxConcurrentActions()
		//fmt.Println("Per role action limit", role.Name + "#." + action.Name, perRoleActionLimit)
		if int(perRoleActionLimit) > 0 && statProcess.Stats.Counts[actionName] >= int(perRoleActionLimit) {
			return true
		}
		if int(perRoleActionConcurrency) > 0 && concurrentStats[actionName] >= int(perRoleActionConcurrency) {
			return true
		}
	}
	actionName = role.Name + "." + action.Name
	if p.config.ActionOptions[actionName] == nil {
		return false
	}
	actionCount := 0
	for k, count := range statProcess.Stats.Counts {

		if strings.HasPrefix(k, role.Name + "#") && strings.HasSuffix(k, "." + action.Name) {
			actionCount += count
		}
	}
	if p.config.ActionOptions[actionName] != nil &&
		int(p.config.ActionOptions[actionName].MaxActions) > 0 &&
		actionCount >= int(p.config.ActionOptions[actionName].MaxActions) {
		//fmt.Println("Exceeds action count limit", actionName, actionCount, role.RefStringShort(), action.Name)
		return true
	}
	if p.config.ActionOptions[actionName] != nil &&
		int(p.config.ActionOptions[actionName].GetMaxConcurrentActions()) > 0 &&
		concurrentStats[actionName] >= int(p.config.ActionOptions[actionName].GetMaxConcurrentActions()) {
		//fmt.Println("Exceeds concurrent action count limit", actionName, concurrentStats[actionName])
		return true
	}

	return false
}

func (p *Processor) Stop() {
	p.stopped = true
}

func (p *Processor) Stopped() bool {
	return p.stopped
}

func (p *Processor) checkLiveness(node *Node) (*InvariantPosition, *Node, bool) {
	// Iterate over all files and their invariants
	for i, file := range p.Files {
		for j, invariant := range file.Invariants {
			// Check if the invariant contains the "eventually" operator
			hasEventually := slices.Contains(invariant.TemporalOperators, "eventually") || invariant.Eventually
			if !hasEventually {
				// Skip if the invariant does not involve liveness
				continue
			}

			// Determine if the invariant is "eventually always" or "always eventually"
			eventuallyAlways := false
			alwaysEventually := false

			// Handle block-based or operator-based invariants
			if invariant.Block == nil {
				if invariant.Always && invariant.Eventually {
					alwaysEventually = true
				} else if invariant.Eventually && invariant.GetNested().GetAlways() {
					eventuallyAlways = true
				}
			} else {
				// Check the order of temporal operators to identify liveness type
				if slices.Contains(invariant.TemporalOperators, "eventually") &&
					invariant.TemporalOperators[0] == "eventually" && invariant.TemporalOperators[1] == "always" {
					eventuallyAlways = true
				} else if slices.Contains(invariant.TemporalOperators, "eventually") &&
					invariant.TemporalOperators[0] == "always" && invariant.TemporalOperators[1] == "eventually" {
					alwaysEventually = true
				}
			}

			// Check Witness for "eventually always" or "always eventually"
			if eventuallyAlways {
				// Check if invariant eventually holds and remains true forever after some point
				if failedNode, ok := p.checkEventuallyAlways(i, j, node); !ok {
					return NewInvariantPosition(i, j), failedNode, false
				}
			} else if alwaysEventually {
				// Check if the invariant holds infinitely often
				if failedNode, ok := p.checkAlwaysEventually(i, j, node); !ok {
					return NewInvariantPosition(i, j), failedNode, false
				}
			}
		}
	}

	// If no invariant failed, return success
	return nil, nil, true
}

// Helper to check "eventually always" (<>[]) property
func (p *Processor) checkEventuallyAlways(fileIndex, invariantIndex int, node *Node) (*Node, bool) {
	// Iterate through the outbound links of the cycle
	currentNode := node
	for {
		// Check if the invariant becomes permanently true at some point
		if !currentNode.Process.Witness[fileIndex][invariantIndex] {
			// If the invariant is false at this node, the "eventually always" condition fails
			return currentNode, false
		}

		// Move to the next node in the cycle
		if len(currentNode.Outbound) == 0 {
			break
		}
		nextNode := currentNode.Outbound[0].Node
		// Break if we detect a cycle back to the initial node
		if nextNode == node {
			break
		}
		currentNode = nextNode
	}

	// If we completed the cycle and the invariant was true throughout, return success
	return nil, true
}

// Helper to check "always eventually" ([]<>) property
func (p *Processor) checkAlwaysEventually(fileIndex, invariantIndex int, node *Node) (*Node, bool) {
	// Iterate through the outbound links of the cycle
	currentNode := node
	for {
		// Check if the invariant is true at least once in this cycle (eventually reappears)
		if currentNode.Process.Witness[fileIndex][invariantIndex] {
			// The invariant was true at some point, so "always eventually" holds
			return nil, true
		}

		// Move to the next node in the cycle
		if len(currentNode.Outbound) == 0 {
			break
		}
		nextNode := currentNode.Outbound[0].Node
		// Break if we detect a cycle back to the initial node
		if nextNode == node {
			break
		}
		currentNode = nextNode
	}

	// If the invariant was never true in the cycle, it fails
	return currentNode, false
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
