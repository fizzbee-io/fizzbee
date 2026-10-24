package modelchecker

import (
	ast "fizz/proto"
	"fmt"
	"sync"

	"github.com/fizzbee-io/fizzbee/lib"
	"github.com/golang/glog"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

func (e *Evaluator) EvalPyExpr(filename string, src interface{}, prevState starlark.StringDict) (starlark.Value, error) {
	return e.EvalPyExprWithContext(filename, src, prevState, nil)
}

func (e *Evaluator) EvalPyExprWithContext(filename string, src interface{}, prevState starlark.StringDict, symCtx *lib.SymmetryContext) (starlark.Value, error) {
	thread := &starlark.Thread{
		Print: func(_ *starlark.Thread, msg string) { fmt.Println(msg) },
	}
	// Inject symmetry context if provided
	if symCtx != nil {
		thread.SetLocal(lib.SymmetryContextKey, symCtx)
		if symCtx.FizzCall != nil {
			thread.SetLocal(lib.FizzCallKey, symCtx.FizzCall)
		}
		if symCtx.Roles != nil {
			thread.SetLocal(lib.RolesKey, symCtx.Roles)
		}
		if symCtx.Channels != nil {
			thread.SetLocal(lib.ChannelsKey, symCtx.Channels)
		}
	}
	value, err := starlark.EvalOptions(e.options, thread, filename, src, prevState)
	if err != nil {
		if !lib.IsDisableTransitionError(err) {
			glog.Errorf("Error evaluating expr: %+v", err)
		}
		return nil, err
	}

	return value, nil
}

func (e *Evaluator) EvalExpr(filename string, expr *ast.Expr, prevState starlark.StringDict) (starlark.Value, error) {
	return e.EvalExprWithContext(filename, expr, prevState, nil)
}

func (e *Evaluator) EvalExprWithContext(filename string, expr *ast.Expr, prevState starlark.StringDict, symCtx *lib.SymmetryContext) (starlark.Value, error) {
	start := expr.GetSourceInfo().GetStart()
	filePortion := syntax.FilePortion{
		Content:   []byte(expr.GetPyExpr()),
		FirstLine: start.GetLine(),
		FirstCol:  start.GetColumn(),
	}
	return e.EvalPyExprWithContext(filename, filePortion, prevState, symCtx)
}

func (e *Evaluator) ExecPyStmt(filename string, stmt *ast.PyStmt, prevState starlark.StringDict) (bool, error) {
	return e.ExecPyStmtWithContext(filename, stmt, prevState, nil)
}

func (e *Evaluator) ExecPyStmtWithContext(filename string, stmt *ast.PyStmt, prevState starlark.StringDict, symCtx *lib.SymmetryContext) (bool, error) {
	start := stmt.GetSourceInfo().GetStart()
	filePortion := syntax.FilePortion{
		Content:   []byte(stmt.Code),
		FirstLine: start.GetLine(),
		FirstCol:  start.GetColumn(),
	}
	f, err := e.options.Parse(filename, filePortion, 0)
	if err != nil {
		glog.Errorf("Error parsing expr: %+v", err)
		return false, err
	}
	thread := &starlark.Thread{
		Print: func(_ *starlark.Thread, msg string) { fmt.Println(msg) },
	}
	// Inject symmetry context if provided
	if symCtx != nil {
		thread.SetLocal(lib.SymmetryContextKey, symCtx)
		if symCtx.FizzCall != nil {
			thread.SetLocal(lib.FizzCallKey, symCtx.FizzCall)
		}
		if symCtx.Roles != nil {
			thread.SetLocal(lib.RolesKey, symCtx.Roles)
		}
		if symCtx.Channels != nil {
			thread.SetLocal(lib.ChannelsKey, symCtx.Channels)
		}
	}
	err = starlark.ExecREPLChunk(f, thread, prevState)
	globals := prevState
	//state, err := starlark.ExecFileOptions(e.options, e.thread, filename, starCode, prevState)
	if err != nil {
		if !lib.IsDisableTransitionError(err) {
			glog.Errorf("Error executing stmt: %+v", err)
		}
		return false, err
	}

	// Print the global environment.
	for _, name := range globals.Keys() {
		v := globals[name]

		prevState[name] = v
	}
	return true, nil
}

// referencedNames returns every identifier the statements mention, apart
// from attribute names, in first-use order: a superset of the names the
// code can read or bind. Local names (comprehension variables, lambda
// parameters) are included too; a lookup that finds nothing drops them.
func referencedNames(stmts []syntax.Stmt) []string {
	seen := map[string]bool{}
	var names []string
	var walk func(n syntax.Node) bool
	walk = func(n syntax.Node) bool {
		switch n := n.(type) {
		case *syntax.Ident:
			if !seen[n.Name] {
				seen[n.Name] = true
				names = append(names, n.Name)
			}
		case *syntax.DotExpr:
			syntax.Walk(n.X, walk)
			return false
		}
		return true
	}
	for _, st := range stmts {
		syntax.Walk(st, walk)
	}
	return names
}

// namesCache memoizes the referenced names per source text; the same
// statements and expressions run many times across states.
var namesCache = struct {
	mu    sync.RWMutex
	exprs map[string][]string
	stmts map[string][]string
}{exprs: make(map[string][]string), stmts: make(map[string][]string)}

// exprNames returns the names an expression references; a source that
// does not parse gets none, and evaluation reports the error.
func exprNames(text string) []string {
	namesCache.mu.RLock()
	names, ok := namesCache.exprs[text]
	namesCache.mu.RUnlock()
	if ok {
		return names
	}
	names = []string{}
	if expr, err := syntax.ParseExpr("<expr>", text, 0); err == nil {
		names = referencedNames([]syntax.Stmt{&syntax.ExprStmt{X: expr}})
	}
	namesCache.mu.Lock()
	namesCache.exprs[text] = names
	namesCache.mu.Unlock()
	return names
}

// stmtNames returns the names a statement's code references.
func stmtNames(code string) []string {
	namesCache.mu.RLock()
	names, ok := namesCache.stmts[code]
	namesCache.mu.RUnlock()
	if ok {
		return names
	}
	names = []string{}
	if f, err := syntax.Parse("<stmt>", code, 0); err == nil {
		names = referencedNames(f.Stmts)
	}
	namesCache.mu.Lock()
	namesCache.stmts[code] = names
	namesCache.mu.Unlock()
	return names
}
