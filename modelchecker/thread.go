package modelchecker

import (
	"crypto/sha256"
	"encoding/json"
	ast "fizz/proto"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/jayaprabhakar/fizzbee/lib"
	"go.starlark.net/starlark"
	"hash"
	"sort"
	"strings"
)

type Heap struct {
	globals starlark.StringDict
}

func (h *Heap) MarshalJSON() ([]byte, error) {
	return StringDictToJson(h.globals)
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
	return StringDictToJsonString(h.globals)
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
	hashBuf := sha256.New()
	hashBuf.Write([]byte(h.ToJson()))
	return fmt.Sprintf("%x", hashBuf.Sum(nil))
}

func (h *Heap) update(k string, v starlark.Value) bool {
	if _, ok := h.globals[k]; ok {
		h.globals[k] = v
		return true
	}
	return false
}

func (h *Heap) insert(k string, v starlark.Value) bool {
	h.globals[k] = v
	return true
}

func (h *Heap) Clone() *Heap {
	return &Heap{CloneDict(h.globals)}
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

func (s *Scope) HashCode() string {
	return fmt.Sprintf("%x", s.Hash().Sum(nil))
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
func (s *Scope) GetAllVisibleVariables() starlark.StringDict {
	dict := starlark.StringDict{}
	s.getAllVisibleVariablesToDict(dict)
	return dict
}

func (s *Scope) getAllVisibleVariablesToDict(dict starlark.StringDict) {
	if s.parent != nil {
		s.parent.getAllVisibleVariablesToDict(dict)
	}
	CopyDict(s.vars, dict)
}

func CloneDict(oldDict starlark.StringDict) starlark.StringDict {
	return CopyDict(oldDict, nil)
}

// CopyDict copies values `from` to `to` overriding existing values. If the `to` is nil, creates a new dict.
func CopyDict(from starlark.StringDict, to starlark.StringDict) starlark.StringDict {
	if to == nil {
		to = make(starlark.StringDict)
	}
	for k, v := range from {
		newValue, err := deepCloneStarlarkValue(v)
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
	return fmt.Sprintf("%x", h.Sum(nil))
}

type CallStack struct {
	*lib.Stack[*CallFrame]
}

func NewCallStack() *CallStack {
	return &CallStack{lib.NewStack[*CallFrame]()}
}

func (s *CallStack) Clone() *CallStack {
	return &CallStack{s.Stack.Clone()}
}

func (s *CallStack) HashCode() string {
	if s == nil {
		return ""
	}
	arr := s.RawArrayCopy()
	h := sha256.New()

	for _, frame := range arr {
		h.Write([]byte(frame.HashCode()))
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// Thread represents a thread of execution.
type Thread struct {
	Process *Process      `json:"-"`
	Files   []*ast.File   `json:"-"`
	Stack   *CallStack	  `json:"stack"`

	Fairness ast.FairnessLevel `json:"fairness"`
}

func NewThread(Process *Process, files []*ast.File, fileIndex int, action string) *Thread {
	stack := NewCallStack()
	frame := &CallFrame{FileIndex: fileIndex, pc: action}
	t := &Thread{Process: Process, Files: files, Stack: stack}
	t.pushFrame(frame)
	return t
}

func (t *Thread) HashCode() string {
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

func (t *Thread) Clone() *Thread {
	return &Thread{Process: t.Process, Files: t.Files, Stack: t.Stack.Clone(), Fairness: t.Fairness}
}

func (t *Thread) Execute() ([]*Process, bool) {
	var forks []*Process
	yield := false
	for t.Stack.Len() > 0 {
		for t.currentFrame().pc == "" || strings.HasSuffix(t.currentFrame().pc, ".Block.$") {
			yield = t.executeEndOfBlock()
			if yield {
				return forks, yield
			}
		}
		frame := t.currentFrame()
		protobuf := GetProtoFieldByPath(t.currentFileAst(), frame.pc)

		switch msg := protobuf.(type) {
		case *ast.Action:
			t.Fairness = msg.GetFairness().GetLevel()
			t.executeAction()
		case *ast.Block:
			forks = t.executeBlock()
		case *ast.Statement:
			forks, yield = t.executeStatement()
		case *ast.ForStmt:
			forks, yield = t.executeForStatement()
		case *ast.WhileStmt:
			forks, yield = t.executeWhileStatement()
		default:
			panic(fmt.Sprintf("Unknown protobuf type: %v", protobuf))
		}
		if len(forks) > 0 || yield {
			break
		}
	}
	return forks, yield
}

func (t *Thread) executeAction() {
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
		t.Process.Labels = append(t.Process.Labels, currentFrame.Name + "." + stmt.Label)
	}
	t.Process.Fairness = t.Fairness
	if stmt.PyStmt != nil {
		vars := t.Process.GetAllVariables()
		_, err := t.Process.Evaluator.ExecPyStmt("filename.fizz", stmt.PyStmt, vars)
		t.Process.PanicOnError(fmt.Sprintf("Error executing statement: %s", stmt.PyStmt.GetCode()), err)
		t.Process.updateAllVariablesInScope(vars)
		t.Process.Enable()
	} else if stmt.Block != nil {
		currentFrame.pc = currentFrame.pc + ".Block"
		forks := t.executeBlock()
		return forks, false
	} else if stmt.IfStmt != nil {
		if stmt.IfStmt.Flow != ast.Flow_FLOW_ATOMIC && currentFrame.scope.flow != ast.Flow_FLOW_ATOMIC {
			panic("Only atomic flow is supported for if statements")
		}
		for i, branch := range stmt.IfStmt.Branches {
			vars := t.Process.GetAllVariables()
			cond, err := t.Process.Evaluator.EvalPyExpr("filename.fizz", branch.Condition, vars)
			//PanicOnError(err)
			t.Process.PanicOnError(fmt.Sprintf("Error checking condition: %s", branch.Condition), err)
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
		//if stmt.AnyStmt.Flow != ast.Flow_FLOW_ATOMIC && t.currentFrame().scope.flow != ast.Flow_FLOW_ATOMIC {
		//	// TODO: Is this actually needed?
		//	panic("Only atomic flow is supported for any statements")
		//}
		if len(stmt.AnyStmt.LoopVars) != 1 {
			panic("Loop variables must be exactly one")
		}
		vars := t.Process.GetAllVariables()
		val, err := t.Process.Evaluator.EvalPyExpr("filename.fizz", stmt.AnyStmt.PyExpr, vars)
		t.Process.PanicOnError(fmt.Sprintf("Error evaluating expr: %s", stmt.AnyStmt.PyExpr), err)
		//PanicOnError(err)
		rangeVal, _ := val.(starlark.Iterable)
		iter := rangeVal.Iterate()
		defer iter.Done()

		scope := t.InsertNewScope()
		if stmt.AnyStmt.Flow != ast.Flow_FLOW_UNKNOWN {
			scope.flow = stmt.AnyStmt.Flow
		} else {
			scope.flow = currentFrame.scope.flow
		}
		forks := make([]*Process, 0)
		var x starlark.Value
		for iter.Next(&x) {
			//fmt.Printf("anyVariable: x: %s\n", x.String())
			fork := t.Process.Fork()
			fork.Name = fmt.Sprintf("Any:%s", x.String())
			fork.currentThread().currentFrame().pc = fmt.Sprintf("%s.AnyStmt.Block", currentFrame.pc)
			fork.currentThread().currentFrame().scope.vars[stmt.AnyStmt.LoopVars[0]] = x
			forks = append(forks, fork)

		}
		if len(forks) > 0 {
			return forks, false
		} else {
			t.ExitScope()
		}

		//scope.vars[stmt.AnyStmt.LoopVars[0]] = val
		//t.currentFrame().pc = fmt.Sprintf("%s.AnyStmt.Block", t.currentPc())
	} else if stmt.ForStmt != nil {
		if stmt.ForStmt.Flow == ast.Flow_FLOW_ONEOF {
			panic("Oneof flow is supported for any statements")
		}
		if len(stmt.ForStmt.LoopVars) != 1 {
			panic("Loop variables must be exactly one. TODO: Support multiple loop variables")
		}
		vars := t.Process.GetAllVariables()
		val, err := t.Process.Evaluator.EvalPyExpr("filename.fizz", stmt.ForStmt.PyExpr, vars)
		t.Process.PanicOnError(fmt.Sprintf("Error evaluating expr: %s", stmt.ForStmt.PyExpr), err)
		//PanicOnError(err)
		rangeVal, ok := val.(starlark.Iterable)
		PanicIfFalse(ok, fmt.Sprintf("Loop variable must be iterable, got %s", val.Type()))
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
	} else if stmt.ReturnStmt != nil {
		vars := t.Process.GetAllVariables()
		var val starlark.Value = starlark.None
		if stmt.ReturnStmt.PyExpr != ""	{
			v, err := t.Process.Evaluator.EvalPyExpr("filename.fizz", stmt.ReturnStmt.PyExpr, vars)
			t.Process.PanicOnError(fmt.Sprintf("Error evaluating expr: %s", stmt.ReturnStmt.PyExpr), err)
			//PanicOnError(err)
			val = v
		}
		actionPath := strings.Split(currentFrame.pc, ".")[0]
		action := GetProtoFieldByPath(t.currentFileAst(), actionPath)
		oldFrame := t.popFrame()
		if t.Stack.Len() == 0 {
			t.Process.removeCurrentThread()
			if val != starlark.None {
				t.Process.Returns[convertToAction(action).Name] = val
				t.Process.Enable()
			}
			return nil, true
		} else {
			if len(oldFrame.callerAssignVarNames) > 1 {
				panic("Multiple return values not supported yet")
			}
			for _, name := range oldFrame.callerAssignVarNames {
				t.currentFrame().scope.vars[name] = val
				t.Process.Enable()
			}
			return t.executeEndOfStatement()
		}
		return nil, false
	} else if stmt.CallStmt != nil {

		frame := currentFrame
		if frame.scope.flow != ast.Flow_FLOW_ATOMIC {
			panic("Only atomic flow is supported for call statements for now")
		}
		def := t.Process.SymbolTable[stmt.CallStmt.Name]
		if def != nil && len(stmt.CallStmt.Args) != 0 {
			panic("CallStmt with args not supported")
		}
		if def == nil {
			// Handle builtin functions. A slightly better way is to use the exact code from the input file
			// and execute. For now, we will generate the code. This will mess up with error messages later
			code := strings.Builder{}
			if len(stmt.CallStmt.Vars) > 0 {
				code.WriteString(strings.Join(stmt.CallStmt.Vars, ", "))
				code.WriteString(" = ")
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
			pyEquivStmt := &ast.PyStmt{Code: code.String()}
			vars := t.Process.GetAllVariables()
			_, err := t.Process.Evaluator.ExecPyStmt("filename.fizz", pyEquivStmt, vars)
			t.Process.PanicOnError(fmt.Sprintf("Error executing statement: %s", pyEquivStmt.GetCode()), err)
			t.Process.updateAllVariablesInScope(vars)
			t.Process.Enable()
		} else {

			newFrame := &CallFrame{FileIndex: def.fileIndex, pc: def.path + ".Block", Name: stmt.CallStmt.Name}
			newFrame.callerAssignVarNames = stmt.CallStmt.Vars
			t.Process.Labels = append(t.Process.Labels, newFrame.Name+".call")
			// TODO: Handle args
			t.pushFrame(newFrame)
			return nil, false
		}

	} else {
		panic(fmt.Sprintf("Unknown statement type: %v at path %s", stmt, t.currentPc()))
	}
	return t.executeEndOfStatement()
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
	vars := t.Process.GetAllVariables()
	cond, err := t.Process.Evaluator.EvalPyExpr("filename.fizz", stmt.PyExpr, vars)
	t.Process.PanicOnError(fmt.Sprintf("Error evaluating expr: %s", stmt.PyExpr), err)
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

	currentFrame := t.currentFrame()
	switch currentFrame.scope.flow {
	case ast.Flow_FLOW_ATOMIC:
		currentFrame.pc = t.FindNextProgramCounter()
		return nil, false
	case ast.Flow_FLOW_SERIAL:
		currentFrame.pc = t.FindNextProgramCounter()
		return nil, true
	case ast.Flow_FLOW_ONEOF:
		currentFrame.pc = EndOfBlock(t.currentPc())
		return nil, false
	case ast.Flow_FLOW_PARALLEL:
		// if currentPc ends with .ForStmt do not execute end of statement.
		if strings.HasSuffix(t.currentPc(), ".ForStmt") {
			return nil, true
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
		return forks, true
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
			actionPath := strings.Split(frame.pc, ".")[0]
			protobuf := GetProtoFieldByPath(t.currentFileAst(), actionPath)
			if action, ok := protobuf.(*ast.Action); ok {
				if action.Name == "Init" {
					variables := oldScope.GetAllVisibleVariables()
					for s, value := range variables {
						t.Process.Heap.insert(s, value)
					}
				}
			}
			oldFrame := t.popFrame()

			if t.Stack.Len() == 0 {
				t.Process.removeCurrentThread()
				return true
			} else {
				frame = t.currentFrame()
				// if protobuf is of type Function then it is a function call.
				if _, ok := protobuf.(*ast.Function); ok {
					if len(oldFrame.callerAssignVarNames) > 1 {
						panic("Multiple return values not supported yet")
					}
					for _, name := range oldFrame.callerAssignVarNames {
						frame.scope.vars[name] = starlark.None
					}
					_,yield := t.executeEndOfStatement()
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
		return true
	}
	return false
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

func convertToBlock(message proto.Message) *ast.Block {
	return message.(*ast.Block)
}

func convertToStatement(message proto.Message) *ast.Statement {
	return message.(*ast.Statement)
}

func convertToWhileStmt(message proto.Message) *ast.WhileStmt {
	return message.(*ast.WhileStmt)
}
