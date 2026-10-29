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
	if c := e.compiledExprFunc(filename, expr.GetPyExpr(), start.GetLine(), start.GetColumn(), prevState); c != nil {
		args := make(starlark.Tuple, len(c.params))
		for i, name := range c.params {
			args[i] = prevState[name]
		}
		v, err := starlark.Call(newEvalThread(symCtx), c.fn, args, nil)
		if err != nil && !lib.IsDisableTransitionError(err) {
			glog.Errorf("Error evaluating expr: %+v", err)
		}
		return v, err
	}
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
		if symCtx.Roles != nil {
			thread.SetLocal(lib.RolesKey, symCtx.Roles)
		}
		if symCtx.Channels != nil {
			thread.SetLocal(lib.ChannelsKey, symCtx.Channels)
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
	// The dialect options change how an expression resolves and compiles,
	// so evaluators with different options never share a program.
	opts     syntax.FileOptions
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
	if e.options != nil {
		key.opts = *e.options
	}
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
	if c := e.compiledStmt(filename, stmt.Code, start.GetLine(), start.GetColumn(), prevState); c != nil {
		return c.exec(newEvalThread(symCtx), prevState)
	}
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
	thread := newEvalThread(symCtx)
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

// stmtFuncName is the function compiledStmt wraps a statement in.
const stmtFuncName = "_fizz_stmt_"

// compiledStmt is a spec statement compiled once as a function, so that a
// statement run once per state is not parsed, resolved and compiled each
// time, as ExecREPLChunk does. The statement becomes the body of
//
//	def _fizz_stmt_(<every env name>):
//	    <statement>
//	    return (<every name the statement binds>,)
//
// Inside the function the env names are parameters, so the statement reads
// and rebinds them as it would module globals, and the names it binds come
// back in order. Only plain assignments and expression statements are
// compiled this way; anything else keeps the REPL path. A statement that
// fails stops the transition or the run, so losing its partial writes
// (which the REPL path keeps) changes nothing.
type compiledStmt struct {
	fn       *starlark.Function
	params   []string
	assigned []string
}

func (c *compiledStmt) exec(thread *starlark.Thread, env starlark.StringDict) (bool, error) {
	args := make(starlark.Tuple, len(c.params))
	for i, name := range c.params {
		args[i] = env[name]
	}
	res, err := starlark.Call(thread, c.fn, args, nil)
	if err != nil {
		if !lib.IsDisableTransitionError(err) {
			glog.Errorf("Error executing stmt: %+v", err)
		}
		return false, err
	}
	for i, v := range res.(starlark.Tuple) {
		env[c.assigned[i]] = v
	}
	return true, nil
}

type stmtKey struct {
	opts     syntax.FileOptions
	filename string
	src      string
	line     int32
	col      int32
	params   string
	expr     bool
}

var stmtCache = struct {
	mu sync.RWMutex
	m  map[stmtKey]*compiledStmt // nil = not compilable, use the REPL path
}{m: make(map[stmtKey]*compiledStmt)}

// compiledStmt returns the statement compiled for an env with exactly
// env's names, or nil when it must run through the REPL path.
func (e *Evaluator) compiledStmt(filename string, src string, line int32, col int32, env starlark.StringDict) *compiledStmt {
	params := env.Keys() // sorted
	key := stmtKey{filename: filename, src: src, line: line, col: col, params: strings.Join(params, ",")}
	if e.options != nil {
		key.opts = *e.options
	}
	stmtCache.mu.RLock()
	c, ok := stmtCache.m[key]
	stmtCache.mu.RUnlock()
	if ok {
		return c
	}
	c = e.compileStmt(filename, src, line, col, params)
	stmtCache.mu.Lock()
	stmtCache.m[key] = c
	stmtCache.mu.Unlock()
	return c
}

func (e *Evaluator) compileStmt(filename string, src string, line int32, col int32, params []string) *compiledStmt {
	f, err := e.options.Parse(filename, src, 0)
	if err != nil {
		return nil
	}
	var assigned []string
	seen := map[string]bool{}
	var bind func(x syntax.Expr) bool
	bind = func(x syntax.Expr) bool {
		switch x := x.(type) {
		case *syntax.Ident:
			if !seen[x.Name] {
				seen[x.Name] = true
				assigned = append(assigned, x.Name)
			}
		case *syntax.TupleExpr:
			for _, y := range x.List {
				if !bind(y) {
					return false
				}
			}
		case *syntax.ListExpr:
			for _, y := range x.List {
				if !bind(y) {
					return false
				}
			}
		case *syntax.ParenExpr:
			return bind(x.X)
		case *syntax.DotExpr, *syntax.IndexExpr:
		default:
			return false
		}
		return true
	}
	for _, st := range f.Stmts {
		switch st := st.(type) {
		case *syntax.ExprStmt:
		case *syntax.AssignStmt:
			if !bind(st.LHS) {
				return nil
			}
		default:
			return nil
		}
	}

	// The statement keeps its source position: the def sits on the line
	// before it, and its first line is indented to its column (at least one
	// space, so it is inside the def).
	indent := int(col) - 1
	if indent < 1 {
		indent = 1
	}
	pad := strings.Repeat(" ", indent)
	var b strings.Builder
	b.WriteString("def " + stmtFuncName + "(" + strings.Join(params, ", ") + "):\n")
	b.WriteString(pad + src + "\n")
	b.WriteString(pad + "return (")
	for _, name := range assigned {
		b.WriteString(name + ", ")
	}
	b.WriteString(")\n")
	portion := syntax.FilePortion{Content: []byte(b.String()), FirstLine: line - 1, FirstCol: 1}
	_, prog, err := starlark.SourceProgramOptions(e.options, filename, portion, func(string) bool { return false })
	if err != nil {
		return nil
	}
	globals, err := prog.Init(&starlark.Thread{}, nil)
	if err != nil {
		return nil
	}
	// Frozen because the function is shared by every state and goroutine.
	globals.Freeze()
	fn, ok := globals[stmtFuncName].(*starlark.Function)
	if !ok {
		return nil
	}
	return &compiledStmt{fn: fn, params: params, assigned: assigned}
}

// compiledExprFunc is compiledStmt for an expression: the expression is
// the body of `def _fizz_stmt_(<env names>): return (<expr>)`, compiled and
// instantiated once, so an evaluation is one call instead of a
// Program.Init that builds a new module each time.
func (e *Evaluator) compiledExprFunc(filename string, src string, line int32, col int32, env starlark.StringDict) *compiledStmt {
	params := env.Keys() // sorted
	key := stmtKey{filename: filename, src: src, line: line, col: col, params: strings.Join(params, ","), expr: true}
	if e.options != nil {
		key.opts = *e.options
	}
	stmtCache.mu.RLock()
	c, ok := stmtCache.m[key]
	stmtCache.mu.RUnlock()
	if ok {
		return c
	}
	pad := int(col) - 1
	if pad < 0 {
		pad = 0
	}
	var b strings.Builder
	b.WriteString("def " + stmtFuncName + "(" + strings.Join(params, ", ") + "):\n return (\n")
	b.WriteString(strings.Repeat(" ", pad) + src + "\n)\n")
	portion := syntax.FilePortion{Content: []byte(b.String()), FirstLine: line - 2, FirstCol: 1}
	c = nil
	if _, prog, err := starlark.SourceProgramOptions(e.options, filename, portion, func(string) bool { return false }); err == nil {
		if globals, err := prog.Init(&starlark.Thread{}, nil); err == nil {
			globals.Freeze()
			if fn, ok := globals[stmtFuncName].(*starlark.Function); ok {
				c = &compiledStmt{fn: fn, params: params}
			}
		}
	}
	stmtCache.mu.Lock()
	stmtCache.m[key] = c
	stmtCache.mu.Unlock()
	return c
}
