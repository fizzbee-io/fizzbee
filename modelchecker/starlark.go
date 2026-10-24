package modelchecker

import (
	ast "fizz/proto"
	"fmt"
	"strings"
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
	thread := newEvalThread(symCtx)
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
	prog, err := e.compiledExpr(filename, expr.GetPyExpr(), start.GetLine(), start.GetColumn(), prevState)
	if err != nil {
		if !lib.IsDisableTransitionError(err) {
			glog.Errorf("Error evaluating expr: %+v", err)
		}
		return nil, err
	}
	thread := newEvalThread(symCtx)
	globals, err := prog.Init(thread, prevState)
	if err != nil {
		if !lib.IsDisableTransitionError(err) {
			glog.Errorf("Error evaluating expr: %+v", err)
		}
		return nil, err
	}
	return globals[exprResultName], nil
}

func newEvalThread(symCtx *lib.SymmetryContext) *starlark.Thread {
	thread := &starlark.Thread{
		Print: func(_ *starlark.Thread, msg string) { fmt.Println(msg) },
	}
	// Inject symmetry context if provided
	if symCtx != nil {
		thread.SetLocal(lib.SymmetryContextKey, symCtx)
		if symCtx.FizzCall != nil {
			thread.SetLocal(lib.FizzCallKey, symCtx.FizzCall)
		}
	}
	return thread
}

// exprResultName is the module global that holds an expression's value in
// the program compiledExpr builds around it.
const exprResultName = "_fizz_expr_result_"

// compiledExpr is a compiled, resolved form of a spec expression. Parsing,
// resolving and compiling an expression cost more than evaluating it, and
// the same expression runs once per state, so the compiled program is
// cached and only instantiated (Program.Init) per evaluation.
//
// starlark.EvalOptions resolves the expression against a specific
// environment and gives no way to rebind it, so the expression is compiled
// as the program `_fizz_expr_result_ = (<expr>)` and run with the
// environment as the predeclared names, which Program.Init takes per call.
// Resolution depends on the environment only through which names it
// defines, so each cached program records the names the resolver asked
// about and is reused only for an environment that answers the same way.
type compiledExpr struct {
	prog    *starlark.Program
	queries map[string]bool // name -> whether the environment defined it at compile time
}

func (c *compiledExpr) matches(env starlark.StringDict) bool {
	for name, had := range c.queries {
		if env.Has(name) != had {
			return false
		}
	}
	return true
}

type exprKey struct {
	filename string
	src      string
	line     int32
	col      int32
}

var exprCache = struct {
	mu sync.RWMutex
	m  map[exprKey][]*compiledExpr
}{m: make(map[exprKey][]*compiledExpr)}

func (e *Evaluator) compiledExpr(filename string, src string, line int32, col int32, env starlark.StringDict) (*starlark.Program, error) {
	key := exprKey{filename: filename, src: src, line: line, col: col}
	exprCache.mu.RLock()
	for _, c := range exprCache.m[key] {
		if c.matches(env) {
			exprCache.mu.RUnlock()
			return c.prog, nil
		}
	}
	exprCache.mu.RUnlock()

	// The expression keeps its source position: the assignment sits on the
	// line before it and the expression's first line is padded to its
	// column, so error messages and stack traces point where they did.
	pad := int(col) - 1
	if pad < 0 {
		pad = 0
	}
	var b strings.Builder
	b.Grow(len(exprResultName) + len(src) + pad + 8)
	b.WriteString(exprResultName)
	b.WriteString(" = (\n")
	for i := 0; i < pad; i++ {
		b.WriteByte(' ')
	}
	b.WriteString(src)
	b.WriteString("\n)")
	portion := syntax.FilePortion{Content: []byte(b.String()), FirstLine: line - 1, FirstCol: 1}

	queries := make(map[string]bool)
	isPredeclared := func(name string) bool {
		has := env.Has(name)
		queries[name] = has
		return has
	}
	_, prog, err := starlark.SourceProgramOptions(e.options, filename, portion, isPredeclared)
	if err != nil {
		return nil, err
	}
	c := &compiledExpr{prog: prog, queries: queries}
	exprCache.mu.Lock()
	exprCache.m[key] = append(exprCache.m[key], c)
	exprCache.mu.Unlock()
	return prog, nil
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
