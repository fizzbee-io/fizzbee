package modelchecker

import (
	ast "fizz/proto"
	"github.com/golang/glog"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

func (e *Evaluator) EvalPyExpr(filename string, src interface{}, prevState starlark.StringDict) (starlark.Value, error) {
	value, err := starlark.EvalOptions(e.options, e.thread, filename, src, prevState)
	if err != nil {
		glog.Errorf("Error evaluating expr: %+v", err)
		return nil, err
	}

	return value, nil
}

func (e *Evaluator) EvalExpr(filename string, expr *ast.Expr, prevState starlark.StringDict) (starlark.Value, error) {
	start := expr.GetSourceInfo().GetStart()
	filePortion := syntax.FilePortion{
		Content:   []byte(expr.GetPyExpr()),
		FirstLine: start.GetLine(),
		FirstCol:  start.GetColumn(),
	}
	return e.EvalPyExpr(filename, filePortion, prevState)
}

func (e *Evaluator) ExecPyStmt(filename string, stmt *ast.PyStmt, prevState starlark.StringDict) (bool, error) {

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

	err = starlark.ExecREPLChunk(f, e.thread, prevState)
	globals := prevState
	//state, err := starlark.ExecFileOptions(e.options, e.thread, filename, starCode, prevState)
	if err != nil {
		glog.Errorf("Error executing stmt: %+v", err)
		return false, err
	}

	// Print the global environment.
	for _, name := range globals.Keys() {
		v := globals[name]

		prevState[name] = v
	}
	return true, nil
}
