package modelchecker

import (
	ast "fizz/proto"
	"fmt"

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
