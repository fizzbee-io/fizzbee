package modelchecker

import (
	"fmt"
	"sync"

	ast "fizz/proto"
	"github.com/fizzbee-io/fizzbee/lib"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

// Lazy action forks.
//
// Scheduling an action used to deep-clone the yield state immediately, once
// per enabled action, and queue the clone. Most of those clones never become
// states: on the Kafka benchmark 80% of action starts end at the action's
// guard (a leading require, or an any with no matching element) before the
// first write. With --experimental_lazy_fork the queue holds a lazyAction
// record instead, the clone is made when the node is dequeued, and a clone
// whose action turned out to be disabled without touching the state is
// reused for the next action of the same parent.
//
// Reuse is only safe when the disabled action left the clone's state
// untouched. Each statement the thread runs is classified before it runs:
// require, if conditions, any and for iterables, return expressions and
// assignments to a local name are read-only when their expressions contain
// no calls other than a small set of pure builtins; everything else marks the
// process dirty and the clone is not reused.

// lazyForksEnabled gates the per-statement purity bookkeeping.
var lazyForksEnabled bool

type lazyAction struct {
	parent      *Process
	role        *lib.Role
	roleIndex   int
	action      *ast.Action
	actionIndex int
}

// newLazyActionNode creates the action-start node without forking the
// parent's state. The clone is made by materializeLazy when the node is
// dequeued.
func (n *Node) newLazyActionNode(parent *Process, role *lib.Role, roleIndex int, action *ast.Action, actionIndex int) *Node {
	actionName := action.Name
	if role != nil {
		actionName = role.RefStringShort() + "." + actionName
	}
	forkNode := &Node{
		Inbound:     make([]*Link, 0, 10),
		Outbound:    make([]*Link, 0, 10),
		actionDepth: n.actionDepth + 1,
		forkDepth:   n.forkDepth + 1,
		stacktrace:  captureStackTrace(),
		lazy:        &lazyAction{parent: parent, role: role, roleIndex: roleIndex, action: action, actionIndex: actionIndex},
	}
	forkNode.Inbound = append(forkNode.Inbound, &Link{Node: n, Type: "action", Name: actionName, ReqId: nextThreadIndex(parent)})
	return forkNode
}

// nextThreadIndex is the slot NewThread will use on a fork of p.
func nextThreadIndex(p *Process) int {
	for i, t := range p.Threads {
		if t == nil {
			return i
		}
	}
	return len(p.Threads)
}

// materializeLazy gives a lazily scheduled action-start node its Process:
// the reusable scratch clone when the previous action of the same parent
// was disabled without writes, a fresh fork otherwise. It returns false when
// the action cannot start because its role is no longer reachable from the
// state (the eager path skipped such actions before queueing them); the
// clone is then kept for the next action.
func (p *Processor) materializeLazy(node *Node) bool {
	la := node.lazy
	node.lazy = nil
	var proc *Process
	if p.scratch != nil && p.scratchParent == la.parent {
		proc = p.scratch
		p.resetScratch(proc, la.parent)
		p.lazyReused++
	} else {
		proc = la.parent.Fork()
		p.lazyForked++
	}
	p.scratch = nil
	p.scratchParent = la.parent
	p.scratchCandidate = proc
	p.scratchPrevCurrent = proc.Current
	p.scratchThread = -1
	proc.dirty = false

	var obj *lib.Role
	if la.role != nil {
		for _, r := range proc.Roles {
			if r != nil && r.RefStringShort() == la.role.RefStringShort() {
				obj = r
				break
			}
		}
		if obj == nil {
			// Role not reachable from the state: the action does not run.
			p.scratch = proc
			return false
		}
	}

	actionName := la.action.Name
	if la.role != nil {
		actionName = la.role.RefStringShort() + "." + actionName
	}
	proc.Name = actionName
	proc.Stats.Increment(actionName)
	node.Process = proc

	thread := proc.NewThread()
	p.scratchThread = proc.Current
	proc.Fairness = la.action.Fairness.GetLevel()
	thread.Fairness = la.action.Fairness.GetLevel()
	frame := thread.currentFrame()
	if la.role != nil {
		frame.obj = obj
		frame.pc = fmt.Sprintf("Roles[%d].Actions[%d]", la.roleIndex, la.actionIndex)
		frame.Name = la.role.Name + "." + la.action.Name
	} else {
		frame.pc = fmt.Sprintf("Actions[%d]", la.actionIndex)
		frame.Name = la.action.Name
	}
	return true
}

// offerScratch is called when an action-start ended disabled. The clone is
// kept for the next action of the same parent when nothing wrote to it.
func (p *Processor) offerScratch(proc *Process) {
	if !p.lazyForks || proc != p.scratchCandidate || proc.dirty {
		return
	}
	p.scratch = proc
}

// resetScratch returns a disabled action's clone to the state right after
// Fork: the action's thread and every per-fork field are cleared. The heap,
// roles, channels and messages are untouched because the action was clean.
func (p *Processor) resetScratch(proc *Process, parent *Process) {
	if i := p.scratchThread; i >= 0 {
		if i == len(proc.Threads)-1 && i >= len(parent.Threads) {
			proc.Threads = proc.Threads[:i]
		} else {
			proc.Threads[i] = nil
		}
	}
	p.scratchThread = -1
	proc.Current = p.scratchPrevCurrent
	proc.Returns = make(starlark.StringDict)
	proc.Labels = make([]string, 0)
	proc.Messages = make([]*ast.Message, 0)
	proc.Stats = parent.Stats.Clone()
	proc.Enabled = false
	proc.ThreadProgress = false
	proc.IsYield = false
	proc.Fairness = 0
	proc.ChoiceFairness = 0
	proc.EnableCheckpoint = false
	proc.FailedInvariants = nil
	proc.Atoms = nil
	proc.CachedHashCode = ""
	proc.CachedThreadHashes = nil
	proc.Heap.CachedHashCode = ""
	proc.Witness = make([][]bool, len(parent.Files))
	for fi, file := range parent.Files {
		proc.Witness[fi] = make([]bool, len(file.Invariants))
	}
	proc.RotationalLastAllocated = nil
	if parent.RotationalLastAllocated != nil {
		proc.RotationalLastAllocated = make(map[string]int64, len(parent.RotationalLastAllocated))
		for k, v := range parent.RotationalLastAllocated {
			proc.RotationalLastAllocated[k] = v
		}
	}
}

// --- statement purity -----------------------------------------------------

var pureExprCache sync.Map // expression code -> bool
var pureStmtCache sync.Map // statement code -> pureStmt

type pureStmt struct {
	ok   bool
	name string // assigned local name, "" for a bare expression
}

// pureBuiltins are calls that read their arguments and allocate at most a
// fresh value. Anything else, including every method call, counts as a write.
var pureBuiltins = map[string]bool{
	"len": true, "range": true, "min": true, "max": true, "sorted": true,
	"list": true, "set": true, "dict": true, "tuple": true, "str": true,
	"int": true, "bool": true, "float": true, "abs": true, "all": true,
	"any": true, "enumerate": true, "zip": true, "reversed": true,
	"getattr": true, "hasattr": true, "type": true, "hash": true,
	"sum": true, "repr": true, "record": true,
}

// isPureExprCode reports whether evaluating the expression cannot modify the
// process state.
func isPureExprCode(code string) bool {
	if code == "" {
		return true
	}
	if v, ok := pureExprCache.Load(code); ok {
		return v.(bool)
	}
	expr, err := syntax.ParseExpr("expr", code, 0)
	pure := err == nil && pureSyntaxExpr(expr)
	pureExprCache.Store(code, pure)
	return pure
}

func isPureExpr(e *ast.Expr) bool {
	if e == nil {
		return true
	}
	return isPureExprCode(e.GetPyExpr())
}

func pureSyntaxExpr(e syntax.Expr) bool {
	switch e := e.(type) {
	case nil:
		return true
	case *syntax.Ident, *syntax.Literal:
		return true
	case *syntax.ParenExpr:
		return pureSyntaxExpr(e.X)
	case *syntax.UnaryExpr:
		return pureSyntaxExpr(e.X)
	case *syntax.BinaryExpr:
		return pureSyntaxExpr(e.X) && pureSyntaxExpr(e.Y)
	case *syntax.DotExpr:
		return pureSyntaxExpr(e.X)
	case *syntax.IndexExpr:
		return pureSyntaxExpr(e.X) && pureSyntaxExpr(e.Y)
	case *syntax.SliceExpr:
		return pureSyntaxExpr(e.X) && pureSyntaxExpr(e.Lo) && pureSyntaxExpr(e.Hi) && pureSyntaxExpr(e.Step)
	case *syntax.CondExpr:
		return pureSyntaxExpr(e.Cond) && pureSyntaxExpr(e.True) && pureSyntaxExpr(e.False)
	case *syntax.TupleExpr:
		return pureSyntaxExprs(e.List)
	case *syntax.ListExpr:
		return pureSyntaxExprs(e.List)
	case *syntax.DictExpr:
		return pureSyntaxExprs(e.List)
	case *syntax.DictEntry:
		return pureSyntaxExpr(e.Key) && pureSyntaxExpr(e.Value)
	case *syntax.Comprehension:
		if !pureSyntaxExpr(e.Body) {
			return false
		}
		for _, c := range e.Clauses {
			switch c := c.(type) {
			case *syntax.ForClause:
				if !pureSyntaxExpr(c.Vars) || !pureSyntaxExpr(c.X) {
					return false
				}
			case *syntax.IfClause:
				if !pureSyntaxExpr(c.Cond) {
					return false
				}
			default:
				return false
			}
		}
		return true
	case *syntax.CallExpr:
		fn, ok := e.Fn.(*syntax.Ident)
		if !ok || !pureBuiltins[fn.Name] {
			return false
		}
		return pureSyntaxExprs(e.Args)
	default:
		return false
	}
}

func pureSyntaxExprs(list []syntax.Expr) bool {
	for _, x := range list {
		if !pureSyntaxExpr(x) {
			return false
		}
	}
	return true
}

// pureLocalStatement classifies a Python statement: an assignment of a pure
// expression to a single plain name, or a bare pure expression. The caller
// still has to check that the name is not a state variable.
func pureLocalStatement(code string) pureStmt {
	if v, ok := pureStmtCache.Load(code); ok {
		return v.(pureStmt)
	}
	res := pureStmt{}
	f, err := syntax.Parse("stmt", code, 0)
	if err == nil && len(f.Stmts) == 1 {
		switch s := f.Stmts[0].(type) {
		case *syntax.AssignStmt:
			if id, ok := s.LHS.(*syntax.Ident); ok && s.Op == syntax.EQ && pureSyntaxExpr(s.RHS) {
				res = pureStmt{ok: true, name: id.Name}
			}
		case *syntax.ExprStmt:
			if pureSyntaxExpr(s.X) {
				res = pureStmt{ok: true}
			}
		}
	}
	pureStmtCache.Store(code, res)
	return res
}

// noteStatement records whether running stmt may write to the process
// state. Only the kinds of statements listed are ever clean.
func (t *Thread) noteStatement(stmt *ast.Statement) {
	p := t.Process
	if p.dirty {
		return
	}
	switch {
	case stmt.PyStmt != nil:
		ps := pureLocalStatement(stmt.PyStmt.GetCode())
		if !ps.ok || ps.name == "self" {
			p.dirty = true
			return
		}
		if ps.name != "" {
			if _, isState := p.Heap.state[ps.name]; isState {
				p.dirty = true
			}
		}
	case stmt.Block != nil, stmt.BreakStmt != nil, stmt.ContinueStmt != nil:
	case stmt.IfStmt != nil:
		for _, b := range stmt.IfStmt.Branches {
			if !isPureExpr(b.GetConditionExpr()) {
				p.dirty = true
				return
			}
		}
	case stmt.AnyStmt != nil:
		if !isPureExpr(stmt.AnyStmt.GetIterExpr()) || !isPureExpr(stmt.AnyStmt.GetConditionExpr()) {
			p.dirty = true
		}
	case stmt.ForStmt != nil:
		if !isPureExpr(stmt.ForStmt.GetIterExpr()) {
			p.dirty = true
		}
	case stmt.WhileStmt != nil:
		if !isPureExpr(stmt.WhileStmt.GetIterExpr()) {
			p.dirty = true
		}
	case stmt.RequireStmt != nil:
		if !isPureExpr(stmt.RequireStmt.GetConditionExpr()) {
			p.dirty = true
		}
	case stmt.ReturnStmt != nil:
		if !isPureExpr(stmt.ReturnStmt.GetExpr()) {
			p.dirty = true
		}
	case stmt.CallStmt != nil:
		// A call to a builtin (no fizz definition) is marked dirty by
		// executeStatement once the definition has been looked up.
		for _, arg := range stmt.CallStmt.Args {
			if !isPureExpr(arg.GetExpr()) {
				p.dirty = true
				return
			}
		}
	default:
		p.dirty = true
	}
}
