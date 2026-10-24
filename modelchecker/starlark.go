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
	}
	if symCtx != nil && symCtx.OwnNames != nil {
		symCtx.OwnNames(exprRefs(filename, src), prevState)
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
	_, err := e.ExecPyStmtAssigned(filename, stmt, prevState, symCtx)
	return err == nil, err
}

// ExecPyStmtAssigned executes stmt in prevState and returns the names the
// statement binds at its top level: assignment and augmented-assignment
// targets, loop variables and def names. The REPL chunk writes those into
// prevState; the caller uses the list to write them back to the process
// without walking the whole environment.
func (e *Evaluator) ExecPyStmtAssigned(filename string, stmt *ast.PyStmt, prevState starlark.StringDict, symCtx *lib.SymmetryContext) ([]string, error) {
	start := stmt.GetSourceInfo().GetStart()
	filePortion := syntax.FilePortion{
		Content:   []byte(stmt.Code),
		FirstLine: start.GetLine(),
		FirstCol:  start.GetColumn(),
	}
	f, err := e.options.Parse(filename, filePortion, 0)
	if err != nil {
		glog.Errorf("Error parsing expr: %+v", err)
		return nil, err
	}
	assigned := assignedNames(f.Stmts, nil)
	if symCtx != nil && symCtx.OwnNames != nil {
		refs := analyzeRefs(f.Stmts)
		refs.MayMutate = true
		symCtx.OwnNames(refs, prevState)
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
	//state, err := starlark.ExecFileOptions(e.options, e.thread, filename, starCode, prevState)
	if err != nil {
		if !lib.IsDisableTransitionError(err) {
			glog.Errorf("Error executing stmt: %+v", err)
		}
		return nil, err
	}
	return assigned, nil
}

// assignedNames appends to names every name the statements bind at the
// level of a REPL chunk: assignment targets (through tuple and list
// destructuring), augmented-assignment targets, for-loop variables and
// def names, including inside if/for/while bodies, which do not open a
// scope. Index and attribute targets mutate a value and bind nothing.
func assignedNames(stmts []syntax.Stmt, names []string) []string {
	var target func(e syntax.Expr)
	target = func(e syntax.Expr) {
		switch e := e.(type) {
		case *syntax.Ident:
			names = append(names, e.Name)
		case *syntax.TupleExpr:
			for _, x := range e.List {
				target(x)
			}
		case *syntax.ListExpr:
			for _, x := range e.List {
				target(x)
			}
		case *syntax.ParenExpr:
			target(e.X)
		}
	}
	for _, st := range stmts {
		switch st := st.(type) {
		case *syntax.AssignStmt:
			target(st.LHS)
		case *syntax.DefStmt:
			names = append(names, st.Name.Name)
		case *syntax.ForStmt:
			target(st.Vars)
			names = assignedNames(st.Body, names)
		case *syntax.WhileStmt:
			names = assignedNames(st.Body, names)
		case *syntax.IfStmt:
			names = assignedNames(st.True, names)
			names = assignedNames(st.False, names)
		}
	}
	return names
}

// pureBuiltins are builtins that never change their arguments.
var pureBuiltins = map[string]bool{
	"len": true, "range": true, "str": true, "int": true, "float": true, "bool": true,
	"min": true, "max": true, "sorted": true, "reversed": true, "any": true, "all": true,
	"list": true, "tuple": true, "set": true, "dict": true, "abs": true, "enumerate": true,
	"zip": true, "hash": true, "type": true, "repr": true, "print": true, "getattr": true,
	"hasattr": true, "dir": true, "chr": true, "ord": true, "record": true, "bag": true,
	"genericset": true, "genericmap": true, "struct": true, "sum": true, "isinstance": true,
	"fail": true, "frozenset": true,
}

// pureMethods are methods of the built-in types that never change their
// receiver or arguments.
var pureMethods = map[string]bool{
	"keys": true, "values": true, "items": true, "get": true, "index": true, "count": true,
	"startswith": true, "endswith": true, "split": true, "join": true, "strip": true,
	"lower": true, "upper": true, "format": true, "replace": true, "find": true,
	"issubset": true, "issuperset": true, "union": true, "intersection": true,
	"difference": true, "symmetric_difference": true, "isdisjoint": true, "copy": true,
	"rfind": true, "lstrip": true, "rstrip": true, "title": true, "capitalize": true,
	"elems": true, "codepoints": true, "isalpha": true, "isdigit": true, "isspace": true,
	"partition": true, "rpartition": true, "splitlines": true, "rsplit": true,
}

// analyzeRefs walks statements and returns what they reference by name:
// identifiers, the attributes used on each identifier, attributes used on
// other expressions, and the identifiers called. MayMutate is set when a
// call could change a value: a method that is not known to be pure, or
// a function other than a pure builtin.
func analyzeRefs(stmts []syntax.Stmt) *lib.NameRefs {
	refs := &lib.NameRefs{}
	seen := map[string]bool{}
	attrSeen := map[string]bool{}
	looseSeen := map[string]bool{}
	var walk func(n syntax.Node) bool
	walk = func(n syntax.Node) bool {
		switch n := n.(type) {
		case *syntax.Ident:
			if !seen[n.Name] {
				seen[n.Name] = true
				refs.Names = append(refs.Names, n.Name)
			}
		case *syntax.DotExpr:
			if id, ok := n.X.(*syntax.Ident); ok {
				k := id.Name + "." + n.Name.Name
				if !attrSeen[k] {
					attrSeen[k] = true
					if refs.Attrs == nil {
						refs.Attrs = map[string][]string{}
					}
					refs.Attrs[id.Name] = append(refs.Attrs[id.Name], n.Name.Name)
				}
			} else if !looseSeen[n.Name.Name] {
				looseSeen[n.Name.Name] = true
				refs.LooseAttrs = append(refs.LooseAttrs, n.Name.Name)
			}
			syntax.Walk(n.X, walk)
			return false
		case *syntax.CallExpr:
			switch fn := n.Fn.(type) {
			case *syntax.Ident:
				if !pureBuiltins[fn.Name] {
					refs.MayMutate = true
					refs.CallIdents = append(refs.CallIdents, fn.Name)
				}
			case *syntax.DotExpr:
				if !pureMethods[fn.Name.Name] {
					refs.MayMutate = true
				}
			default:
				refs.MayMutate = true
				refs.CallIdents = append(refs.CallIdents, "")
			}
		}
		return true
	}
	for _, st := range stmts {
		syntax.Walk(st, walk)
	}
	return refs
}

// exprRefsCache memoizes exprRefs per expression text: the same
// expression is evaluated many times across states.
var exprRefsCache = struct {
	mu sync.RWMutex
	m  map[string]*lib.NameRefs
}{m: make(map[string]*lib.NameRefs)}

// exprRefs returns analyzeRefs for an expression source as passed to
// starlark.EvalOptions (a string, []byte or syntax.FilePortion). An
// expression that does not parse gets empty refs; evaluation reports the
// error.
func exprRefs(filename string, src interface{}) *lib.NameRefs {
	var text string
	switch x := src.(type) {
	case string:
		text = x
	case []byte:
		text = string(x)
	case syntax.FilePortion:
		text = string(x.Content)
	default:
		return &lib.NameRefs{}
	}
	exprRefsCache.mu.RLock()
	refs, ok := exprRefsCache.m[text]
	exprRefsCache.mu.RUnlock()
	if ok {
		return refs
	}
	expr, err := syntax.ParseExpr(filename, text, 0)
	if err == nil {
		refs = analyzeRefs([]syntax.Stmt{&syntax.ExprStmt{X: expr}})
	} else {
		refs = &lib.NameRefs{}
	}
	exprRefsCache.mu.Lock()
	exprRefsCache.m[text] = refs
	exprRefsCache.mu.Unlock()
	return refs
}
