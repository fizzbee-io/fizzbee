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

// lazyVerifyReuse, when set (by tests), is called with every reused clone
// right after its reset, together with a fresh fork of the same parent. The
// two must be indistinguishable.
var lazyVerifyReuse func(reused, fresh *Process)

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
		if lazyVerifyReuse != nil {
			lazyVerifyReuse(proc, la.parent.Fork())
		}
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
	proc.Action = frame.Name
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
	proc.cardStep = parent.cardStep
	proc.Enabled = false
	proc.ThreadProgress = false
	proc.IsYield = false
	proc.Fairness = 0
	proc.Action = ""
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

var pureExprCache sync.Map // expression code -> pureExpr
var pureStmtCache sync.Map // statement code -> pureStmt

// pureExpr is the purity of an expression by its syntax alone. calls lists
// the builtin names it calls: the expression is pure only where each of them
// still resolves to that builtin (see builtinsShadowed).
type pureExpr struct {
	ok    bool
	calls []string
}

type pureStmt struct {
	ok    bool
	name  string // assigned local name, "" for a bare expression
	calls []string
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

// classifyExprCode classifies an expression by its syntax: whether it can
// modify the process state, given that the builtin names it calls resolve to
// the builtins.
func classifyExprCode(code string) pureExpr {
	if code == "" {
		return pureExpr{ok: true}
	}
	if v, ok := pureExprCache.Load(code); ok {
		return v.(pureExpr)
	}
	res := pureExpr{}
	expr, err := syntax.ParseExpr("expr", code, 0)
	if err == nil {
		res.ok = pureSyntaxRoot(expr, &res.calls)
	}
	pureExprCache.Store(code, res)
	return res
}

// isPureExpr reports whether evaluating the expression in t's current frame
// cannot modify the process state.
func (t *Thread) isPureExpr(e *ast.Expr) bool {
	if e == nil {
		return true
	}
	c := classifyExprCode(e.GetPyExpr())
	return c.ok && !t.builtinsShadowed(c.calls)
}

// builtinsShadowed reports whether any of the names, which the purity check
// assumed to be builtins, is bound to something else where t runs: a fizz
// function, a local, a state variable, a global or a module. The lookup
// order follows lookupNameIn.
func (t *Thread) builtinsShadowed(names []string) bool {
	if len(names) == 0 {
		return false
	}
	p := t.Process
	fixed := p.fixedEnvironment()
	frame := t.currentFrame()
	for _, name := range names {
		if _, ok := fixed[name]; ok {
			return true
		}
		if p.fizzFuncBuiltin(name) != nil {
			return true
		}
		for s := frame.scope; s != nil; s = s.parent {
			if _, ok := s.vars[name]; ok {
				return true
			}
		}
		if _, ok := frame.vars[name]; ok {
			return true
		}
		if _, ok := p.Heap.state[name]; ok {
			return true
		}
		if _, ok := p.Heap.globals[name]; ok {
			return true
		}
	}
	return false
}

// pureSyntaxRoot is pureSyntaxExpr for a whole expression. It also rejects
// an expression whose comprehension variables reuse the name of a builtin
// it calls, since the call would then go to the variable.
func pureSyntaxRoot(e syntax.Expr, calls *[]string) bool {
	var bound []string
	if !pureSyntaxExpr(e, calls, &bound) {
		return false
	}
	for _, b := range bound {
		for _, c := range *calls {
			if b == c {
				return false
			}
		}
	}
	return true
}

func pureSyntaxExpr(e syntax.Expr, calls, bound *[]string) bool {
	switch e := e.(type) {
	case nil:
		return true
	case *syntax.Ident, *syntax.Literal:
		return true
	case *syntax.ParenExpr:
		return pureSyntaxExpr(e.X, calls, bound)
	case *syntax.UnaryExpr:
		return pureSyntaxExpr(e.X, calls, bound)
	case *syntax.BinaryExpr:
		return pureSyntaxExpr(e.X, calls, bound) && pureSyntaxExpr(e.Y, calls, bound)
	case *syntax.DotExpr:
		return pureSyntaxExpr(e.X, calls, bound)
	case *syntax.IndexExpr:
		return pureSyntaxExpr(e.X, calls, bound) && pureSyntaxExpr(e.Y, calls, bound)
	case *syntax.SliceExpr:
		return pureSyntaxExpr(e.X, calls, bound) && pureSyntaxExpr(e.Lo, calls, bound) && pureSyntaxExpr(e.Hi, calls, bound) && pureSyntaxExpr(e.Step, calls, bound)
	case *syntax.CondExpr:
		return pureSyntaxExpr(e.Cond, calls, bound) && pureSyntaxExpr(e.True, calls, bound) && pureSyntaxExpr(e.False, calls, bound)
	case *syntax.TupleExpr:
		return pureSyntaxExprs(e.List, calls, bound)
	case *syntax.ListExpr:
		return pureSyntaxExprs(e.List, calls, bound)
	case *syntax.DictExpr:
		return pureSyntaxExprs(e.List, calls, bound)
	case *syntax.DictEntry:
		return pureSyntaxExpr(e.Key, calls, bound) && pureSyntaxExpr(e.Value, calls, bound)
	case *syntax.Comprehension:
		if !pureSyntaxExpr(e.Body, calls, bound) {
			return false
		}
		for _, c := range e.Clauses {
			switch c := c.(type) {
			case *syntax.ForClause:
				if !pureSyntaxExpr(c.Vars, calls, bound) || !pureSyntaxExpr(c.X, calls, bound) {
					return false
				}
				syntax.Walk(c.Vars, func(n syntax.Node) bool {
					if id, ok := n.(*syntax.Ident); ok {
						*bound = append(*bound, id.Name)
					}
					return true
				})
			case *syntax.IfClause:
				if !pureSyntaxExpr(c.Cond, calls, bound) {
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
		for _, arg := range e.Args {
			// sorted, min and max call their key function, which may be
			// any callable; a lambda is already rejected below.
			if kw, ok := arg.(*syntax.BinaryExpr); ok && kw.Op == syntax.EQ {
				if id, ok := kw.X.(*syntax.Ident); ok && id.Name == "key" {
					return false
				}
			}
		}
		*calls = append(*calls, fn.Name)
		return pureSyntaxExprs(e.Args, calls, bound)
	default:
		return false
	}
}

func pureSyntaxExprs(list []syntax.Expr, calls, bound *[]string) bool {
	for _, x := range list {
		if !pureSyntaxExpr(x, calls, bound) {
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
		var calls []string
		switch s := f.Stmts[0].(type) {
		case *syntax.AssignStmt:
			if id, ok := s.LHS.(*syntax.Ident); ok && s.Op == syntax.EQ && pureSyntaxRoot(s.RHS, &calls) {
				res = pureStmt{ok: true, name: id.Name, calls: calls}
			}
		case *syntax.ExprStmt:
			if pureSyntaxRoot(s.X, &calls) {
				res = pureStmt{ok: true, calls: calls}
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
		if !ps.ok || ps.name == "self" || t.builtinsShadowed(ps.calls) {
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
			if !t.isPureExpr(b.GetConditionExpr()) {
				p.dirty = true
				return
			}
		}
	case stmt.AnyStmt != nil:
		if !t.isPureExpr(stmt.AnyStmt.GetIterExpr()) || !t.isPureExpr(stmt.AnyStmt.GetConditionExpr()) {
			p.dirty = true
		}
	case stmt.ForStmt != nil:
		if !t.isPureExpr(stmt.ForStmt.GetIterExpr()) {
			p.dirty = true
		}
	case stmt.WhileStmt != nil:
		if !t.isPureExpr(stmt.WhileStmt.GetIterExpr()) {
			p.dirty = true
		}
	case stmt.RequireStmt != nil:
		if !t.isPureExpr(stmt.RequireStmt.GetConditionExpr()) {
			p.dirty = true
		}
	case stmt.ReturnStmt != nil:
		if !t.isPureExpr(stmt.ReturnStmt.GetExpr()) {
			p.dirty = true
		}
	case stmt.CallStmt != nil:
		// A call to a builtin (no fizz definition) is marked dirty by
		// executeStatement once the definition has been looked up.
		for _, arg := range stmt.CallStmt.Args {
			if !t.isPureExpr(arg.GetExpr()) {
				p.dirty = true
				return
			}
		}
	default:
		p.dirty = true
	}
}
