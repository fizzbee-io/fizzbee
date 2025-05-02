package modelchecker

import (
	"crypto/sha256"
	"encoding/json"
	ast "fizz/proto"
	"fmt"
	"github.com/fizzbee-io/fizzbee/lib"
	"github.com/huandu/go-clone"
	"go.starlark.net/starlark"
	"google.golang.org/protobuf/proto"
	"hash"
	"maps"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
)

var nextActionId = atomic.Int32{}

type Heap struct {
	state          starlark.StringDict
	globals        starlark.StringDict
	CachedHashCode string
}

func (h *Heap) GetSymmetryDefs() []*lib.SymmetricValues {
	symmetryDefs := make([]*lib.SymmetricValues, 0)
	// If the value is of type *lib.SymmetricValues, then add it to the symmetryDefs, list
	for _, value := range h.globals {
		if sym, ok := value.(lib.SymmetricValues); ok {
			symmetryDefs = append(symmetryDefs, &sym)
		}
	}
	return symmetryDefs
}

func (h *Heap) MarshalJSON() ([]byte, error) {
	return StringDictToJsonRetainType(h.state)
}

func StringDictToJsonRetainType(strDict starlark.StringDict) ([]byte, error) {
	m := normalizeTypes(strDict)
	bytes, err := lib.MarshalJSON(m)
	//fmt.Println("----\n", m, "\n---\n", string(bytes))
	return bytes, err
}

func normalizeTypes(stringDict starlark.StringDict) starlark.StringDict {
	m := make(starlark.StringDict, len(stringDict))
	for k, v := range stringDict {
		if v.Type() == "set" {
			// Convert set to a list.
			iter := v.(starlark.Iterable).Iterate()

			var x starlark.Value
			var list []string
			for iter.Next(&x) {
				list = append(list, x.String())
			}
			sort.Strings(list)
			values := make([]starlark.Value, len(list))
			for i, s := range list {
				values[i] = starlark.String(s)
			}
			m[k] = starlark.NewList(values)
			iter.Done()
			continue
		} else if v.Type() == "dict" {
			// Convert map keys to a sorted list and add re-add them.
			dict := v.(*starlark.Dict)
			keys := dict.Keys()

			var list []string
			var keyMap = make(map[string]starlark.Value)
			for _, x := range keys {
				list = append(list, x.String())
				keyMap[x.String()] = x
			}
			sort.Strings(list)

			newDict := starlark.NewDict(len(list))
			for _, x := range list {
				key := keyMap[x]
				val, _, _ := dict.Get(key)
				err := newDict.SetKey(key, val)
				PanicOnError(err)
			}
			m[k] = newDict
			continue
		} else if v.Type() == "role" {
			// Convert v to Role
			role := v.(*lib.Role)
			m[k] = starlark.String(role.RefString())
		}
		// list is okay. no changes needed
		m[k] = v
	}
	return m
}

func StringDictToMap(stringDict starlark.StringDict) map[string]string {
	m := make(map[string]string, len(stringDict))
	for k, v := range stringDict {
		if v.Type() == "set" {
			// Convert set to a list.
			iter := v.(starlark.Iterable).Iterate()

			var x starlark.Value
			var list []string
			for iter.Next(&x) {
				list = append(list, x.String())
			}
			sort.Strings(list)
			m[k] = fmt.Sprintf("%v", list)
			iter.Done()
			continue
		} else if v.Type() == "dict" {
			// Convert map keys to a sorted list and add re-add them.
			dict := v.(*starlark.Dict)
			keys := dict.Keys()

			var list []string
			var keyMap = make(map[string]starlark.Value)
			for _, x := range keys {
				list = append(list, x.String())
				keyMap[x.String()] = x
			}
			sort.Strings(list)

			newDict := starlark.NewDict(len(list))
			for _, x := range list {
				key := keyMap[x]
				val, _, _ := dict.Get(key)
				err := newDict.SetKey(key, val)
				PanicOnError(err)
			}
			m[k] = fmt.Sprintf("%v", newDict)
			continue
		}
		// list is okay. no changes needed
		str := v.String()
		m[k] = str
	}
	return m
}

func (h *Heap) ToJson() string {
	return StringDictToJsonString(h.state)
}

func StringDictToJsonString(stringDict starlark.StringDict) string {
	bytes, err := StringDictToJson(stringDict)
	if err != nil {
		panic(err)
	}
	return string(bytes)
}

func StringDictToJson(stringDict starlark.StringDict) ([]byte, error) {
	m := StringDictToMap(stringDict)
	bytes, err := json.Marshal(m)
	return bytes, err
}

func (h *Heap) String() string {
	return h.ToJson()
}

// HashCode returns a string hash of the global state.
func (h *Heap) HashCode() string {
	if h.CachedHashCode != "" {
		return h.CachedHashCode
	}
	hashBuf := sha256.New()
	hashBuf.Write([]byte(h.ToJson()))
	h.CachedHashCode = fmt.Sprintf("%x", hashBuf.Sum(nil))
	return h.CachedHashCode
}

func (h *Heap) update(k string, v starlark.Value) bool {
	if _, ok := h.state[k]; ok {
		h.state[k] = v
		return true
	}
	return false
}

func (h *Heap) insert(k string, v starlark.Value) bool {
	h.state[k] = v
	return true
}

func (h *Heap) Clone(refs map[starlark.Value]starlark.Value, permutations map[lib.SymmetricValue][]lib.SymmetricValue, alt int) *Heap {
	return &Heap{state: CloneDict(h.state, refs, permutations, alt), globals: h.globals}
}

type Scope struct {
	// parent is the parent scope, or nil if this is the global scope.
	parent *Scope
	flow   ast.Flow
	// vars is the set of variables defined in this scope.
	vars starlark.StringDict
	// On parallel execution, skipstmts contains the list of statements to skip
	// as it is already executed.
	skipstmts []int

	loopVars []string
	// loopRange contains the range of values for the loop variables (probably a tuple when multiple loopVars).
	loopRange []starlark.Value
}

func (s *Scope) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"parent":    s.parent,
		"vars":      StringDictToJsonString(s.vars),
		"skipstmts": s.skipstmts,
		"loopRange": s.loopRange,
	})
}

func (s *Scope) SetFlow(flow ast.Flow) {
	if flow != ast.Flow_FLOW_UNKNOWN {
		s.flow = flow
	}
}

func (s *Scope) Hash() hash.Hash {
	var h hash.Hash
	if s == nil {
		return sha256.New()
	}
	if s.parent != nil {
		h = s.parent.Hash()
	} else {
		h = sha256.New()
	}
	vars, err := StringDictToJson(s.vars)
	if err != nil {
		panic(err)
	}
	h.Write(vars)
	h.Write([]byte(fmt.Sprintln(sortedCopy(s.skipstmts))))
	h.Write([]byte(fmt.Sprintln(s.loopRange)))
	return h
}

func sortedCopy(slice []int) []int {
	sorted := make([]int, len(slice))
	copy(sorted, slice)
	sort.Ints(sorted)
	return sorted
}

func (s *Scope) Lookup(name string) (starlark.Value, bool) {
	v, ok := s.vars[name]
	if !ok && s.parent != nil {
		return s.parent.Lookup(name)
	}
	return v, ok
}

// GetAllVisibleVariables returns all variables visible in this scope.
func (s *Scope) GetAllVisibleVariables(roleRefs map[starlark.Value]starlark.Value) starlark.StringDict {
	dict := starlark.StringDict{}
	s.getAllVisibleVariablesResolveRoles(dict, roleRefs)
	return dict
}

func (s *Scope) getAllVisibleVariablesResolveRoles(dict starlark.StringDict, roleRefs map[starlark.Value]starlark.Value) {
	if s.parent != nil {
		s.parent.getAllVisibleVariablesResolveRoles(dict, roleRefs)
	}
	// TODO: Resolve roles
	CopyDict(s.vars, dict, roleRefs, nil, 0)
}

func (s *Scope) getAllVisibleVariablesToDictNoCopy(dict starlark.StringDict) {
	if s.parent != nil {
		s.parent.getAllVisibleVariablesToDictNoCopy(dict)
	}
	maps.Copy(dict, s.vars)
}

func (s *Scope) getAllVisibleVariablesToDict(dict starlark.StringDict) {
	s.getAllVisibleVariablesResolveRoles(dict, make(map[starlark.Value]starlark.Value))
}

func CloneDict(oldDict starlark.StringDict, refs map[starlark.Value]starlark.Value, permutations map[lib.SymmetricValue][]lib.SymmetricValue, alt int) starlark.StringDict {
	return CopyDict(oldDict, nil, refs, permutations, alt)
}

// CopyDict copies values `from` to `to` overriding existing values. If the `to` is nil, creates a new dict.
func CopyDict(from starlark.StringDict, to starlark.StringDict, refs map[starlark.Value]starlark.Value, permutations map[lib.SymmetricValue][]lib.SymmetricValue, alt int) starlark.StringDict {
	if to == nil {
		to = make(starlark.StringDict)
	}
	for k, v := range from {
		if v.Type() == "builtin_function_or_method" || v.Type() == "module" {
			continue
		}
		newValue, err := deepCloneStarlarkValueWithPermutations(v, refs, permutations, alt)
		PanicOnError(err)
		to[k] = newValue
	}
	return to
}

type CallFrame struct {
	// FileIndex is the ast.FileIndex that this frame is executing.
	FileIndex int
	// pc is the program counter, pointing at the next instruction to execute.
	pc string

	// Name is the full path of the function/action being executed.
	Name string

	// scope is the lexical scope of the Current frame
	scope *Scope
	// vars is the dictionary of arguments passed to the function.
	// This should eventually replace local variables from the scope as python doesn't have block scope.
	vars starlark.StringDict

	callerAssignVarNames []string
	obj                  *lib.Role
}

func (c *CallFrame) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"fileIndex": c.FileIndex,
		"pc":        c.pc,
		"name":      c.Name,
		"scope":     c.scope,
		"vars":      StringDictToJsonString(c.vars),
	})

}

func (c *CallFrame) HashCode() string {
	// Hash the scope and append the pc to it.
	// This is to ensure that the same scoped variables are not treated the same
	// if program counter is at different stmts.
	h := c.scope.Hash()
	h.Write([]byte(c.pc))
	if c.obj != nil {
		h.Write([]byte(c.obj.RefString()))
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
func (c *CallFrame) Clone(refs map[starlark.Value]starlark.Value, permutations map[lib.SymmetricValue][]lib.SymmetricValue, alt int) (*CallFrame, error) {
	obj := c.obj
	if c.obj != nil {
		cloned, err := deepCloneStarlarkValueWithPermutations(c.obj, refs, permutations, alt)
		if err != nil {
			return nil, err
		}
		obj = cloned.(*lib.Role)
	}
	newVars := CloneDict(c.vars, refs, permutations, alt)
	newScope := clone.Slowly(c.scope).(*Scope)
	newFrame := &CallFrame{
		FileIndex:            c.FileIndex,
		pc:                   c.pc,
		Name:                 c.Name,
		scope:                newScope,
		vars:                 newVars,
		callerAssignVarNames: c.callerAssignVarNames,
		obj:                  obj,
	}
	return newFrame, nil
}

type CallStack struct {
	*lib.Stack[*CallFrame]
}

func NewCallStack() *CallStack {
	return &CallStack{lib.NewStack[*CallFrame]()}
}

func (s *CallStack) Clone(map[lib.SymmetricValue][]lib.SymmetricValue, int) *CallStack {
	// TODO: handle symmetry in stack.Clone()
	return &CallStack{s.Stack.Clone()}
}

func (s *CallStack) HashCode() string {
	if s == nil {
		return ""
	}
	arr := s.RawArray()
	h := sha256.New()

	for _, frame := range arr {
		h.Write([]byte(frame.HashCode()))
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// Thread represents a thread of execution.
type Thread struct {
	Id      int         `json:"id"`
	Process *Process    `json:"-"`
	Files   []*ast.File `json:"-"`
	Stack   *CallStack  `json:"stack"`

	Fairness ast.FairnessLevel `json:"fairness"`
	Aborted  bool              `json:"-"`
}

func NewThread(Process *Process, files []*ast.File, fileIndex int, action string) *Thread {
	stack := NewCallStack()
	frame := &CallFrame{FileIndex: fileIndex, pc: action}
	t := &Thread{Id: int(nextActionId.Add(1)), Process: Process, Files: files, Stack: stack}
	t.pushFrame(frame)
	return t
}

func (t *Thread) HashCode() string {
	if t == nil {
		return ""
	}
	h := sha256.New()
	h.Write([]byte(t.Stack.HashCode()))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// InsertNewScope adds a new scope to the Current stack frame and returns the newly created scope.
func (t *Thread) InsertNewScope() *Scope {
	scope := &Scope{parent: t.currentFrame().scope, vars: starlark.StringDict{}}
	t.currentFrame().scope = scope
	if scope.parent != nil {
		scope.flow = scope.parent.flow
		scope.vars = scope.parent.vars
	}
	return scope
}

// ExitScope removes the Current scope and returns the removed scope or nil if no scope was present.
func (t *Thread) ExitScope() *Scope {
	scope := t.currentFrame().scope
	if scope == nil {
		return nil
	}
	t.currentFrame().scope = scope.parent
	return scope
}

func (t *Thread) currentFrame() *CallFrame {
	frame, ok := t.Stack.Peek()
	PanicIfFalse(ok, "No frame on the stack")
	return frame
}

func (t *Thread) currentFileAst() *ast.File {
	frame := t.currentFrame()
	return t.Files[frame.FileIndex]
}

func PanicIfFalse(ok bool, msg string) {
	if !ok {
		panic(msg)
	}
}

func PanicOnError(err error) {
	if err != nil {
		panic(err)
	}
}

func (t *Thread) pushFrame(frame *CallFrame) {
	t.Stack.Push(frame)
}

func (t *Thread) popFrame() *CallFrame {
	frame, found := t.Stack.Pop()
	PanicIfFalse(found, "No frame on the stack")
	return frame
}

func (t *Thread) Clone(permutations map[lib.SymmetricValue][]lib.SymmetricValue, alt int) *Thread {
	// TODO: handle symmetry in stack.Clone()
	return &Thread{Id: t.Id, Process: t.Process, Files: t.Files, Stack: t.Stack.Clone(permutations, alt), Fairness: t.Fairness}
}

func (t *Thread) Execute() ([]*Process, bool) {
	var forks []*Process
	yield := false
	hasNonEndOfBlockStmts := false
	initialThreads := t.Process.GetThreadsCount()
	defer t.Process.propagateEnabled()
	for t.Stack.Len() > 0 {
		for t.currentFrame().pc == "" || strings.HasSuffix(t.currentFrame().pc, ".Block.$") {
			yield = t.executeEndOfBlock()
			if yield {
				if !hasNonEndOfBlockStmts && t.Process.GetThreadsCount() < initialThreads && t.Process.Parent != nil && t.Process.Parent.Enabled {
					t.Process.Fairness = t.Fairness
					t.Process.Enable()
				}
				return forks, yield
			}
		}
		hasNonEndOfBlockStmts = true
		frame := t.currentFrame()
		protobuf := GetProtoFieldByPath(t.currentFileAst(), frame.pc)

		switch msg := protobuf.(type) {
		case *ast.Action:
			t.Fairness = msg.GetFairness().GetLevel()
			t.Process.Fairness = t.Fairness
			t.executeAction()
		case *ast.Invariant:
			t.executeInvariant()
		case *ast.Block:
			forks = t.executeBlock()
		case *ast.Statement:
			if msg.Label == "checkpoint" && t.Process.EnableCheckpoint {
				// Checkpoint is enabled if there was already no checkpoint
				// before or fork or yield before this.
				fork := t.Process.Fork()
				fork.Name = fmt.Sprintf("checkpoint")
				return []*Process{fork}, false
			}
			t.Process.EnableCheckpoint = true
			forks, yield = t.executeStatement()
		case *ast.ForStmt:
			forks, yield = t.executeForStatement()
		case *ast.WhileStmt:
			forks, yield = t.executeWhileStatement()
		default:
			panic(fmt.Sprintf("Unknown protobuf type: %T, value %v at path %s", protobuf, protobuf, frame.pc))
		}
		if t.Aborted {
			return nil, false
		}
		if len(forks) > 0 {
			break
		}
		if yield {
			if t.Stack.Len() == 0 {
				t.Process.removeCurrentThread()
				return forks, true
			}
			for t.Stack.Len() > 0 && (t.currentFrame().pc == "" || strings.HasSuffix(t.currentFrame().pc, ".Block.$")) {
				t.executeEndOfBlock()
			}

			return forks, true
		}
	}
	return forks, yield
}

func (t *Thread) executeAction() {
	t.currentFrame().pc = t.currentFrame().pc + ".Block"
}

func (t *Thread) executeInvariant() {
	t.currentFrame().pc = t.currentFrame().pc + ".Block"
}

func (t *Thread) executeBlock() []*Process {
	newScope := t.InsertNewScope()
	protobuf := GetProtoFieldByPath(t.currentFileAst(), t.currentPc())
	b := convertToBlock(protobuf)
	newScope.SetFlow(b.Flow)
	switch newScope.flow {
	case ast.Flow_FLOW_ATOMIC:
		t.currentFrame().pc = t.currentPc() + ".Stmts[0]"
		return nil
	case ast.Flow_FLOW_SERIAL:
		t.currentFrame().pc = t.currentPc() + ".Stmts[0]"
		return nil
	case ast.Flow_FLOW_ONEOF:
		forks := make([]*Process, len(b.Stmts))
		for i := range b.Stmts {
			forks[i] = t.Process.Fork()
			forks[i].Name = fmt.Sprintf("Stmt:%d", i)
			forks[i].currentThread().currentFrame().pc = fmt.Sprintf("%s.Stmts[%d]", t.currentPc(), i)
		}
		return forks
	case ast.Flow_FLOW_PARALLEL:
		forks := make([]*Process, len(b.Stmts))
		for i := range b.Stmts {
			forks[i] = t.Process.Fork()
			forks[i].Name = fmt.Sprintf("Stmt:%d", i)
			forks[i].currentThread().currentFrame().pc = fmt.Sprintf("%s.Stmts[%d]", t.currentPc(), i)
			forks[i].currentThread().currentFrame().scope.skipstmts = append(forks[i].currentThread().currentFrame().scope.skipstmts, i)
		}
		return forks
	default:
		panic("Unknown flow type")
	}

	return nil
}

func (t *Thread) executeStatement() ([]*Process, bool) {
	currentFrame := t.currentFrame()
	protobuf := GetProtoFieldByPath(t.currentFileAst(), currentFrame.pc)
	stmt := convertToStatement(protobuf)
	if stmt.Label != "" {
		t.Process.Labels = append(t.Process.Labels, currentFrame.Name+"."+stmt.Label)
	}
	t.Process.Fairness = t.Fairness
	oldRolesCount := len(t.Process.Roles)
	if stmt.PyStmt != nil {
		if stmt.PyStmt.GetCode() == "pass" {
			t.Process.Enable()
			t.currentFrame().pc = t.FindNextProgramCounter()
			return nil, false
		}
		vars := t.Process.GetAllVariablesNocopy()
		_, err := t.Process.Evaluator.ExecPyStmt(t.getFileName(), stmt.PyStmt, vars)
		t.Process.PanicOnError(stmt.PyStmt.GetSourceInfo(), fmt.Sprintf("Error executing statement: %s", stmt.PyStmt.GetCode()), err)
		t.Process.updateAllVariablesInScope(vars)
		t.Process.Enable()
	} else if stmt.Block != nil {
		currentFrame.pc = currentFrame.pc + ".Block"
		forks := t.executeBlock()
		return forks, false
	} else if stmt.IfStmt != nil {
		// For IfStmt, the condition expression is evaluated atomically.
		// So there is no yield in between an if condition evaluation and elif
		// or if/elif/else and the first statement of the block.
		for i, branch := range stmt.IfStmt.Branches {
			vars := t.Process.GetAllVariablesNocopy()
			conditionExpr := branch.GetConditionExpr()
			cond, err := t.Process.Evaluator.EvalExpr(t.getFileName(), conditionExpr, vars)

			t.Process.PanicOnError(conditionExpr.GetSourceInfo(), fmt.Sprintf("Error checking condition: %s", branch.Condition), err)
			t.Process.updateAllVariablesInScope(vars)
			if cond.Truth() {
				currentFrame.pc = fmt.Sprintf("%s.IfStmt.Branches[%d].Block", currentFrame.pc, i)
				return nil, false
			}
		}

		//t.currentFrame().pc = t.currentFrame().pc + ".Block"
		//forks := t.executeBlock()
		//return forks, false
	} else if stmt.AnyStmt != nil {
		t.Process.ThreadProgress = false
		//if stmt.AnyStmt.Flow != ast.Flow_FLOW_ATOMIC && t.currentFrame().scope.flow != ast.Flow_FLOW_ATOMIC {
		//	// TODO: Is this actually needed?
		//	panic("Only atomic flow is supported for any statements")
		//}
		if len(stmt.AnyStmt.LoopVars) != 1 {
			t.Process.PanicIfFalse(false, stmt.AnyStmt.GetSourceInfo(), fmt.Sprintf("Exactly one loop variable expected. Got %d in %s", len(stmt.AnyStmt.LoopVars), stmt.AnyStmt.LoopVars))
		}
		vars := t.Process.GetAllVariablesNocopy()
		val, err := t.Process.Evaluator.EvalExpr(t.getFileName(), stmt.AnyStmt.IterExpr, vars)
		// TODO: This source info should be for the pyExpr not the anyStmt
		t.Process.PanicOnError(stmt.AnyStmt.GetSourceInfo(), fmt.Sprintf("Error evaluating expr: %s", stmt.AnyStmt.PyExpr), err)
		t.Process.updateAllVariablesInScope(vars)
		//PanicOnError(err)
		rangeVal, ok := val.(starlark.Iterable)
		t.Process.PanicIfFalse(ok, stmt.AnyStmt.IterExpr.GetSourceInfo(), fmt.Sprintf("Loop expression %s must be iterable, got %s", stmt.AnyStmt.PyExpr, val.Type()))

		iter := rangeVal.Iterate()
		defer iter.Done()

		if stmt.AnyStmt.Block != nil {
			scope := t.InsertNewScope()
			if stmt.AnyStmt.Flow != ast.Flow_FLOW_UNKNOWN {
				scope.flow = stmt.AnyStmt.Flow
			} else {
				scope.flow = currentFrame.scope.flow
			}
		}

		forks := make([]*Process, 0)
		var x starlark.Value
		for iter.Next(&x) {
			//fmt.Printf("anyVariable: x: %s\n", x.String())
			fork := t.Process.Fork()
			fork.Name = fmt.Sprintf("Any:%s=%s", stmt.AnyStmt.LoopVars[0], x.String())
			fork.ChoiceFairness = stmt.AnyStmt.Fairness.GetLevel()
			if r, ok := x.(*lib.Role); ok {
				for _, role := range fork.Roles {
					if r.RefString() == role.RefString() {
						x = role
						break
					}
				}
			}
			if stmt.AnyStmt.Block == nil {
				fork.updateVariable(stmt.AnyStmt.LoopVars[0], x)
			} else {
				fork.currentThread().currentFrame().scope.vars[stmt.AnyStmt.LoopVars[0]] = x
			}

			if stmt.AnyStmt.Condition != "" {
				vars := fork.GetAllVariablesNocopy()
				vars[stmt.AnyStmt.LoopVars[0]] = x
				cond, err := fork.Evaluator.EvalExpr(t.getFileName(), stmt.AnyStmt.ConditionExpr, vars)
				//PanicOnError(err)
				// TODO: This source info should be for the condition not the anyStmt
				fork.PanicOnError(stmt.AnyStmt.GetSourceInfo(), fmt.Sprintf("Error checking condition: %s", stmt.AnyStmt.Condition), err)
				fork.updateAllVariablesInScope(vars)
				if !cond.Truth() {
					continue
				}
			}

			if stmt.AnyStmt.Block != nil {
				fork.currentThread().currentFrame().pc = fmt.Sprintf("%s.AnyStmt.Block", currentFrame.pc)
			} else {
				fork.currentThread().currentFrame().pc = t.FindNextProgramCounter()
			}
			forks = append(forks, fork)
		}
		if len(forks) > 0 {
			if stmt.AnyStmt.Block == nil {
				t.Process.Enable()
			}
			return forks, false
		} else if stmt.AnyStmt.Block != nil {
			t.ExitScope()
		} else {
			t.Process.Enabled = false
			t.Process.ThreadProgress = false
			t.Aborted = true
			return nil, false
		}

		//scope.vars[stmt.AnyStmt.LoopVars[0]] = val
		//t.currentFrame().pc = fmt.Sprintf("%s.AnyStmt.Block", t.currentPc())
	} else if stmt.ForStmt != nil {
		if stmt.ForStmt.Flow == ast.Flow_FLOW_ONEOF {
			panic("Oneof flow is not supported for any statements")
		}
		if len(stmt.ForStmt.LoopVars) != 1 {
			t.Process.PanicIfFalse(false, stmt.AnyStmt.GetSourceInfo(), fmt.Sprintf("Loop variables must be exactly one. TODO: Support multiple loop variables. Got %d in %s", len(stmt.ForStmt.LoopVars), stmt.ForStmt.LoopVars))
		}
		vars := t.Process.GetAllVariablesNocopy()
		val, err := t.Process.Evaluator.EvalExpr(t.getFileName(), stmt.ForStmt.IterExpr, vars)
		// TODO: This source info should be for the pyExpr not the forStmt
		t.Process.PanicOnError(stmt.ForStmt.GetSourceInfo(), fmt.Sprintf("Error evaluating expr: %s", stmt.ForStmt.PyExpr), err)

		rangeVal, ok := val.(starlark.Iterable)
		t.Process.PanicIfFalse(ok, stmt.ForStmt.IterExpr.GetSourceInfo(), fmt.Sprintf("Loop expression %s must be iterable, got %s", stmt.ForStmt.PyExpr, val.Type()))
		iter := rangeVal.Iterate()
		defer iter.Done()

		scope := t.InsertNewScope()
		scope.SetFlow(stmt.ForStmt.Flow)
		scope.loopVars = stmt.ForStmt.LoopVars
		var x starlark.Value
		for iter.Next(&x) {
			scope.loopRange = append(scope.loopRange, x)
		}
		currentFrame.pc = currentFrame.pc + ".ForStmt"
		return nil, false
	} else if stmt.WhileStmt != nil {
		scope := t.InsertNewScope()
		scope.SetFlow(stmt.WhileStmt.Flow)
		currentFrame.pc = fmt.Sprintf("%s.WhileStmt", currentFrame.pc)
		return nil, false
	} else if stmt.BreakStmt != nil {
		for !(strings.HasSuffix(currentFrame.pc, ".ForStmt") || strings.HasSuffix(currentFrame.pc, ".WhileStmt")) {
			currentFrame.pc = RemoveLastBlock(currentFrame.pc)
			currentFrame.scope = currentFrame.scope.parent
		}
		currentFrame.scope = currentFrame.scope.parent
		currentFrame.pc = RemoveLastLoop(currentFrame.pc)
		return t.executeEndOfStatement()

	} else if stmt.ContinueStmt != nil {
		for {
			currentFrame.pc = RemoveLastBlock(currentFrame.pc)
			if strings.HasSuffix(currentFrame.pc, ".ForStmt") || strings.HasSuffix(currentFrame.pc, ".WhileStmt") {
				break
			}
			currentFrame.scope = currentFrame.scope.parent
		}
		currentFrame.pc = currentFrame.pc + ".Block.$"
		return nil, false
	} else if stmt.RequireStmt != nil {
		t.Process.ThreadProgress = false
		vars := t.Process.GetAllVariablesNocopy()
		cond, err := t.Process.Evaluator.EvalExpr(t.getFileName(), stmt.RequireStmt.GetConditionExpr(), vars)
		//PanicOnError(err)
		t.Process.PanicOnError(stmt.RequireStmt.GetSourceInfo(), fmt.Sprintf("Error checking condition: %s", stmt.RequireStmt.Condition), err)
		t.Process.updateAllVariablesInScope(vars)
		if !cond.Truth() {
			t.Process.Enabled = false
			t.Process.ThreadProgress = false
			t.Aborted = true
			return nil, false
		}
	} else if stmt.ReturnStmt != nil {
		vars := t.Process.GetAllVariablesNocopy()
		var val starlark.Value = starlark.None
		if stmt.ReturnStmt.PyExpr != "" {
			v, err := t.Process.Evaluator.EvalExpr(t.getFileName(), stmt.ReturnStmt.GetExpr(), vars)
			t.Process.PanicOnError(stmt.ReturnStmt.GetSourceInfo(), fmt.Sprintf("Error evaluating expr: %s", stmt.ReturnStmt.PyExpr), err)
			//PanicOnError(err)
			val = v
		}
		pathComp := strings.Split(currentFrame.pc, ".")
		actionPath := pathComp[0]
		fileAst := t.currentFileAst()
		action := GetProtoFieldByPath(fileAst, actionPath)
		oldFrame := t.popFrame()
		if t.Stack.Len() == 0 {
			//t.Process.removeCurrentThread()
			if val != starlark.None {
				if action1, ok := action.(*ast.Action); ok {
					t.Process.Returns[convertToAction(action1).Name] = val
					t.Process.Enable()
				} else if invariant, ok := action.(*ast.Invariant); ok {
					//fmt.Println("Handling invariant returns")
					t.Process.Returns[convertToInvariant(invariant).Name] = val
					//t.Process.Enable()
				} else if _, ok := action.(*ast.Role); ok {
					actionPath = pathComp[0] + "." + pathComp[1]
					action1 = GetProtoFieldByPath(fileAst, actionPath).(*ast.Action)
					t.Process.Returns[oldFrame.obj.RefStringShort()+"."+convertToAction(action1).Name] = val
					t.Process.Enable()
				} else {
					panic(fmt.Sprintf("Unknown protobuf type: %T, value %v at path %s", action, action, currentFrame.pc))
				}

			}
			return nil, true
		} else {
			if len(oldFrame.callerAssignVarNames) > 1 {
				panic("Multiple return values not supported yet")
			}
			returnedVars := starlark.StringDict{}
			for _, name := range oldFrame.callerAssignVarNames {
				returnedVars[name] = val
				t.Process.Enable()
			}
			t.Process.updateAllVariablesInScope(returnedVars)

			parentScope := oldFrame.scope
			if oldFrame.scope.flow != ast.Flow_FLOW_ATOMIC {

				for parentScope != nil && parentScope.flow == ast.Flow_FLOW_ONEOF {
					parentScope = parentScope.parent
				}
			}
			flow := ast.Flow_FLOW_ATOMIC
			if parentScope != nil {
				flow = parentScope.flow
			}
			t.Process.RecordReturn(t.currentFrame(), oldFrame, val, flow)
			return t.executeEndOfStatement()
		}
		return nil, false
	} else if stmt.CallStmt != nil {

		frame := currentFrame
		parentScope := frame.scope
		if frame.scope.flow != ast.Flow_FLOW_ATOMIC {

			for parentScope != nil && parentScope.flow == ast.Flow_FLOW_ONEOF {
				parentScope = parentScope.parent
			}
		}
		receiver, stub, def := t.getDefinition(stmt)
		if def == nil {
			// Handle builtin functions. A slightly better way is to use the exact code from the input file
			// and execute. For now, we will generate the code. This will mess up with error messages later
			// TODO: Use the exact code from the input file, otherwise the error line numbers and message might be slightly wrong
			code := strings.Builder{}
			if len(stmt.CallStmt.Vars) > 0 {
				code.WriteString(strings.Join(stmt.CallStmt.Vars, ", "))
				code.WriteString(" = ")
			}
			if stmt.CallStmt.Receiver != "" {
				code.WriteString(stmt.CallStmt.Receiver)
				code.WriteString(".")
			}
			code.WriteString(stmt.CallStmt.Name)
			code.WriteString("(")

			for _, arg := range stmt.CallStmt.Args {
				if arg.Name != "" {
					code.WriteString(arg.Name)
					code.WriteString("=")
				}
				code.WriteString(arg.PyExpr)
				code.WriteString(", ")
			}
			code.WriteString(")")
			pyEquivStmt := &ast.PyStmt{Code: code.String(), SourceInfo: stmt.CallStmt.GetSourceInfo()}
			vars := t.Process.GetAllVariablesNocopy()
			_, err := t.Process.Evaluator.ExecPyStmt(t.getFileName(), pyEquivStmt, vars)
			t.Process.PanicOnError(stmt.CallStmt.GetSourceInfo(), fmt.Sprintf("Error executing statement: %s", pyEquivStmt.GetCode()), err)
			t.Process.updateAllVariablesInScope(vars)
			t.Process.Enable()
		} else {
			if frame.obj == nil && parentScope != nil && parentScope.flow != ast.Flow_FLOW_ATOMIC {
				msg := fmt.Sprintf("Call stmts can be made only in atomic context or from within roles. %s",
					stmt.CallStmt.Name)
				panic(msg)
			}
			// Handle function calls
			newFrame := &CallFrame{FileIndex: def.fileIndex, pc: def.path + ".Block", Name: stmt.CallStmt.Name}
			newFrame.vars = starlark.StringDict{}
			hasNamedArgs := false
			vars := t.Process.GetAllVariablesNocopy()
			for i, arg := range stmt.CallStmt.Args {
				// TODO: Is it really required to GetAllVariables() for each arg?

				val, err := t.Process.Evaluator.EvalExpr(t.getFileName(), arg.Expr, vars)
				// TODO: This source info should be for the pyExpr not the callStmt
				t.Process.PanicOnError(arg.Expr.GetSourceInfo(), fmt.Sprintf("Error evaluating expr: %s", arg.PyExpr), err)
				//PanicOnError(err)
				t.Process.updateAllVariablesInScope(vars)
				if !hasNamedArgs && arg.Name == "" {
					if i >= len(def.params) {
						panic(t.Process.NewModelError(stmt.CallStmt.GetSourceInfo(), fmt.Sprintf("Too many arguments for %s", stmt.CallStmt.Name), nil))
					}
					newFrame.vars[def.params[i].Name] = val
				} else if arg.Name != "" {
					newFrame.vars[arg.Name] = val
					hasNamedArgs = true
				} else {
					panic("Named arguments must come after positional arguments")
				}
			}
			vars = t.Process.GetAllVariablesNocopy()
			for _, param := range def.params {
				// handle default values
				if _, ok := newFrame.vars[param.Name]; !ok {
					if param.DefaultPyExpr != "" {
						val, err := t.Process.Evaluator.EvalExpr(t.getFileName(), param.DefaultExpr, vars)
						t.Process.PanicOnError(param.GetDefaultExpr().GetSourceInfo(), fmt.Sprintf("Error evaluating expr: %s", param.DefaultPyExpr), err)
						//PanicOnError(err)
						t.Process.updateAllVariablesInScope(vars)
						newFrame.vars[param.Name] = val
					} else {
						panic(fmt.Sprintf("Missing argument %s", param.Name))
					}
				}
			}
			//fmt.Println("CallStmt: ", stmt.CallStmt.Name, newFrame.vars)
			if stmt.CallStmt.Receiver != "" {
				newFrame.obj = receiver
			}
			newFrame.callerAssignVarNames = stmt.CallStmt.Vars
			t.Process.Labels = append(t.Process.Labels, newFrame.Name+".call")
			t.Process.RecordCall(frame, newFrame, parentScope.flow)

			if stub == nil /*|| (stub.Channel.IsSynchronous() && stub.Channel.IsOrdered())*/ {
				// TODO: Handle args
				t.pushFrame(newFrame)
				return nil, false
			} else {
				t.Process.addChannelMessage(stub.Channel, receiver.RefStringShort(), newFrame, newFrame.Name, newFrame.vars)
				t.Process.Enable()
				return t.executeEndOfStatement()
			}
			return nil, false
		}

	} else {
		panic(fmt.Sprintf("Unknown statement type: %v at path %s", stmt, t.currentPc()))
	}

	if len(t.Process.Roles) > oldRolesCount {
		if len(t.Process.Roles)-oldRolesCount > 1 {
			panic("Creating multiple roles in single step is not supported yet")
		}
		newRole := t.Process.Roles[len(t.Process.Roles)-1]
		fileIndex, nextPc := findRoleInitAction(t.Process, newRole)
		if nextPc != "" {
			newFrame := &CallFrame{FileIndex: fileIndex, pc: nextPc, Name: "Init"}
			newFrame.vars = starlark.StringDict{}
			newFrame.obj = newRole
			t.pushFrame(newFrame)
			return nil, false
		}
	}
	return t.executeEndOfStatement()
}

func (t *Thread) getFileName() string {
	if len(t.Process.Files) != 1 {
		panic("Only single file execution is supported")
	}
	return t.Process.Files[0].GetSourceInfo().GetFileName()
}

func (t *Thread) getDefinition(stmt *ast.Statement) (*lib.Role, *lib.RoleStub, *Definition) {
	vars := t.Process.GetAllVariablesNocopy()
	if stmt.CallStmt.Receiver != "" {
		if receiver, ok := vars[stmt.CallStmt.Receiver]; ok {
			if receiver.Type() == "RoleStub" {
				stub := receiver.(*lib.RoleStub)
				role := stub.Role
				return role, stub, t.Process.SymbolTable[role.Name+"."+stmt.CallStmt.Name]
			} else if receiver.Type() == "role" {
				role := receiver.(*lib.Role)
				return role, nil, t.Process.SymbolTable[role.Name+"."+stmt.CallStmt.Name]
			} else {
				return nil, nil, nil
			}
		}
		panic(fmt.Sprintf("Receiver %s not found in vars", stmt.CallStmt.Receiver))
	}
	return nil, nil, t.Process.SymbolTable[stmt.CallStmt.Name]
}

func findRoleInitAction(process *Process, role *lib.Role) (int, string) {
	for i, file := range process.Files {
		for j, r := range file.Roles {
			if r.Name == role.Name {
				global := maps.Clone(process.Heap.globals)
				for _, stmt := range r.Stmts {
					_, err := process.Evaluator.ExecPyStmt(file.GetSourceInfo().GetFileName(), stmt.PyStmt, global)
					process.PanicOnError(stmt.GetSourceInfo(), fmt.Sprintf("Error executing statement: %s", stmt.PyStmt.GetCode()), err)
				}
				for k, v := range global {
					if v.Type() != "function" || process.Heap.globals[k] == v {
						// original global did not change
						continue
					}

					err := role.AddMethod(k, v)
					process.PanicOnError(nil, fmt.Sprintf("Error adding method %s to role %s", k, role.Name), err)
				}
				for k, action := range r.Actions {
					if action.Name == "Init" {
						nextPath := fmt.Sprintf("Roles[%d].Actions[%d].Block", j, k)
						return i, nextPath
					}

				}
			}
		}
	}
	return -1, ""
}

func (t *Thread) executeForStatement() ([]*Process, bool) {
	currentFrame := t.currentFrame()
	if len(currentFrame.scope.loopRange) == 0 {
		currentFrame.scope = currentFrame.scope.parent
		currentFrame.pc = RemoveLastForStmt(t.currentPc())
		return t.executeEndOfStatement()
		//return nil, false
	}
	scope := currentFrame.scope
	currentFrame.pc = fmt.Sprintf("%s.Block", t.currentPc())

	// only atomic flow is supported for now.
	if scope.flow == ast.Flow_FLOW_ATOMIC || scope.flow == ast.Flow_FLOW_SERIAL {
		scope.vars[scope.loopVars[0]] = scope.loopRange[0]
		scope.loopRange = scope.loopRange[1:]
		return nil, false
	}
	forks := make([]*Process, 0)
	for i, x := range scope.loopRange {
		// TODO(jp): This is a hack. We should not be forking for each iteration,
		// instead, create a new thread for each iteration.
		// That is because, even though, for the correctness analysis, the Current formulation is fine,
		// if we eventually want to reason about performance, this formulation is not sufficient.
		// That is, in the Current formulation, a loop on n elements means, there are n! permutations in which,
		// they can be executed sequentially. That is, if each iteration takes 1 second, then it would imply, the total
		// time will take n seconds. But we need a way to capture they are actually happening in parallel, so they
		// should take only 1 second. Technically max time taken for each iteration.
		// This is a subtle difference, but it will be important in the future for performance analysis. After all,
		// if anyone uses parallel flow, it is to speed up.
		fork := t.Process.Fork()
		if r, ok := x.(*lib.Role); ok {
			for _, role := range fork.Roles {
				if r.RefString() == role.RefString() {
					x = role
					break
				}
			}
		}
		fork.currentThread().currentFrame().scope.vars[scope.loopVars[0]] = x
		fork.Name = fmt.Sprintf("For:%s", x.String())
		newSlice := removeElement(scope.loopRange, i)
		fork.currentThread().currentFrame().scope.loopRange = newSlice

		forks = append(forks, fork)
	}
	return forks, false
}

func (t *Thread) executeWhileStatement() ([]*Process, bool) {
	protobuf := GetProtoFieldByPath(t.currentFileAst(), t.currentPc())
	stmt := convertToWhileStmt(protobuf)

	if stmt.Flow == ast.Flow_FLOW_PARALLEL || stmt.Flow == ast.Flow_FLOW_ONEOF {
		panic("Only atomic/serial flow is supported for while statements")
	}
	vars := t.Process.GetAllVariablesNocopy()
	cond, err := t.Process.Evaluator.EvalExpr(t.getFileName(), stmt.GetIterExpr(), vars)
	t.Process.PanicOnError(stmt.GetIterExpr().GetSourceInfo(), fmt.Sprintf("Error evaluating expr: %s", stmt.PyExpr), err)
	//PanicOnError(err)
	t.Process.updateAllVariablesInScope(vars)
	if cond.Truth() {
		t.currentFrame().pc = fmt.Sprintf("%s.Block", t.currentPc())
		return nil, false
	}
	t.currentFrame().scope = t.currentFrame().scope.parent
	t.currentFrame().pc = RemoveLastWhileStmt(t.currentPc())
	return t.executeEndOfStatement()
}

func removeElement[T any](slice []T, index int) []T {
	if index < 0 || index >= len(slice) {
		// Index out of bounds
		return slice
	}
	newSlice := make([]T, 0, len(slice)-1)
	newSlice = append(newSlice, slice[:index]...)
	// Create a new slice with the element at the specified index removed
	return append(newSlice, slice[index+1:]...)
}

func (t *Thread) executeEndOfStatement() ([]*Process, bool) {
	enabled := t.Process.Enabled
	currentFrame := t.currentFrame()
	switch currentFrame.scope.flow {
	case ast.Flow_FLOW_ATOMIC:
		currentFrame.pc = t.FindNextProgramCounter()
		return nil, false
	case ast.Flow_FLOW_SERIAL:
		currentFrame.pc = t.FindNextProgramCounter()
		return nil, enabled
	case ast.Flow_FLOW_ONEOF:
		currentFrame.pc = EndOfBlock(t.currentPc())
		return nil, false
	case ast.Flow_FLOW_PARALLEL:
		// if currentPc ends with .ForStmt do not execute end of statement.
		if strings.HasSuffix(t.currentPc(), ".ForStmt") {
			return nil, enabled
		}
		blockPath := ParentBlockPath(t.currentPc())
		if blockPath == "" {
			//return nil, t.executeEndOfBlock()
		}
		protobuf := GetProtoFieldByPath(t.currentFileAst(), blockPath)
		b := convertToBlock(protobuf)
		skipstmts := currentFrame.scope.skipstmts
		if len(skipstmts) == len(b.Stmts) {
			currentFrame.pc = EndOfBlock(t.currentPc())
			return nil, false
		}
		forks := make([]*Process, 0, len(b.Stmts)-len(skipstmts))
		for i := range b.Stmts {
			if ContainsInt(skipstmts, i) {
				continue
			}
			fork := t.Process.Fork()
			fork.Name = fmt.Sprintf("Stmt:%d", i)
			fork.currentThread().currentFrame().pc = fmt.Sprintf("%s.Stmts[%d]", blockPath, i)
			fork.currentThread().currentFrame().scope.skipstmts = append(fork.currentThread().currentFrame().scope.skipstmts, i)
			forks = append(forks, fork)
		}
		currentFrame.pc = ""
		return forks, enabled
	default:
		panic(fmt.Sprintf("Unknown flow type at %s", t.currentPc()))
	}
}

func (t *Thread) executeEndOfBlock() bool {
	frame := t.currentFrame()
	if frame == nil {
		return false
	}
	for {

		oldScope := frame.scope
		frame.scope = frame.scope.parent
		if frame.scope == nil {
			//t.popFrame()
			pathComp := strings.Split(frame.pc, ".")
			actionPath := pathComp[0]
			protobuf := GetProtoFieldByPath(t.currentFileAst(), actionPath)

			if action, ok := protobuf.(*ast.Action); ok {
				if action.Name == "Init" {
					roleRefs := make(map[starlark.Value]starlark.Value)
					for _, role := range t.Process.Roles {
						roleRefs[role] = role
					}
					variables := oldScope.GetAllVisibleVariables(roleRefs)
					for s, value := range variables {
						if !t.Process.Heap.globals.Has(s) && slices.Contains(t.Process.topLevelVars, s) {
							t.Process.Heap.insert(s, value)
						}
					}
				}
			}
			isRole := false
			if _, ok := protobuf.(*ast.Role); ok {
				actionPath = pathComp[0] + "." + pathComp[1]
				protobuf = GetProtoFieldByPath(t.currentFileAst(), actionPath)
				isRole = true
			}
			oldFrame := t.popFrame()

			if t.Stack.Len() == 0 {
				t.Process.removeCurrentThread()
				return true
			} else {
				frame = t.currentFrame()
				// if protobuf is of type Function then it is a function call.
				isFunction := false
				isInitAction := false
				if _, ok := protobuf.(*ast.Function); ok {
					isFunction = true
				}
				if action, ok := protobuf.(*ast.Action); ok {
					if action.Name == "Init" {
						isInitAction = true
					}
				}
				if isFunction || (isRole && isInitAction) {
					if len(oldFrame.callerAssignVarNames) > 1 {
						panic("Multiple return values not supported yet")
					}
					if isRole && isInitAction {
						t.CopyInitValuesForEphemeralFields(oldFrame)
					}

					returnedVars := starlark.StringDict{}
					for _, name := range oldFrame.callerAssignVarNames {
						returnedVars[name] = starlark.None
						t.Process.Enable()
					}
					t.Process.updateAllVariablesInScope(returnedVars)
					t.Process.RecordReturn(t.currentFrame(), oldFrame, starlark.None, oldScope.flow)
					_, yield := t.executeEndOfStatement()
					return yield
				}

			}
		}
		frame.pc = RemoveLastBlock(t.currentPc())
		forks, yield := t.executeEndOfStatement()
		if len(forks) > 0 || yield {
			return yield
		}

		if t.currentPc() != "" {
			break
		}
	}
	if frame.scope.flow == ast.Flow_FLOW_SERIAL ||
		frame.scope.flow == ast.Flow_FLOW_PARALLEL {
		// Only yield if there was at least one executable statement
		return t.Process.Enabled
	}
	return false
}

func (t *Thread) CopyInitValuesForEphemeralFields(oldFrame *CallFrame) {
	fields := oldFrame.obj.Fields
	fieldsCloned, err := deepCloneStarlarkValue(fields, nil)
	if err != nil {
		panic(err)
	}
	fieldsClonedStruct := fieldsCloned.(*lib.Struct)
	for _, fieldName := range fieldsClonedStruct.AttrNames() {
		isDurable := t.Process.durabilitySpec.IsFieldDurable(oldFrame.obj.Name, fieldName)
		if isDurable {
			continue
		} else {
			attr, err := fieldsClonedStruct.Attr(fieldName)
			if err == nil {
				oldFrame.obj.InitValues.SetField(fieldName, attr)
			}
		}
	}
}

func ContainsInt(skipstmts []int, i int) bool {
	for _, s := range skipstmts {
		if s == i {
			return true
		}
	}
	return false
}

func (t *Thread) currentPc() string {
	return t.currentFrame().pc
}

type CodeSnippet interface {
	GetSourceInfo() *ast.SourceInfo
}

func (t *Thread) CurrentPcSourceInfo() *ast.SourceInfo {
	protoMsg := GetProtoFieldByPath(t.currentFileAst(), t.currentPc())
	if protoMsg == nil {
		return &ast.SourceInfo{}
	}
	snippet := protoMsg.(CodeSnippet)
	info := snippet.GetSourceInfo()
	return info
}

func (t *Thread) FindNextProgramCounter() string {
	frame := t.currentFrame()
	protobuf := GetProtoFieldByPath(t.currentFileAst(), frame.pc)
	switch protobuf.(type) {
	case *ast.Action:
		return frame.pc + ".Block"
	case *ast.Block:
		convertToBlock(protobuf)
		return frame.pc + ".Stmts[0]"
	case *ast.Statement:
		path, _ := GetNextFieldPath(t.currentFileAst(), frame.pc)
		return path
	case *ast.AnyStmt:
		path, _ := GetNextFieldPath(t.currentFileAst(), frame.pc)
		frame.scope = frame.scope.parent
		return path
	case *ast.ForStmt:
		// ForStmt is in the same instruction counter, only the iteration variable changes.
		return frame.pc
	case *ast.WhileStmt:
		return frame.pc
	case *ast.Branch:
		path, _ := GetNextFieldPath(t.currentFileAst(), frame.pc)
		return path
	}
	return ""
}

func convertToAction(message proto.Message) *ast.Action {
	return message.(*ast.Action)
}

func convertToInvariant(message proto.Message) *ast.Invariant {
	return message.(*ast.Invariant)
}

func convertToBlock(message proto.Message) *ast.Block {
	return message.(*ast.Block)
}

func convertToStatement(message proto.Message) *ast.Statement {
	return message.(*ast.Statement)
}

func convertToWhileStmt(message proto.Message) *ast.WhileStmt {
	return message.(*ast.WhileStmt)
}
