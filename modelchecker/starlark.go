package modelchecker

import (
	ast "fizz/proto"
	"github.com/golang/glog"
	"go.starlark.net/starlark"
)

func (e *Evaluator) EvalPyExpr(filename string, src interface{}, prevState starlark.StringDict) (starlark.Value, error) {

	value, err := starlark.EvalOptions(e.options, e.thread, filename, src, prevState)
	if err != nil {
		glog.Errorf("Error evaluating expr: %+v", err)
		return nil, err
	}

	return value, nil
}

func (e *Evaluator) ExecPyStmt(filename string, stmt *ast.PyStmt, prevState starlark.StringDict) (bool, error) {

	starCode := stmt.Code

	f, err := e.options.Parse(filename, starCode, 0)
	if err != nil {
		glog.Errorf("Error parsing expr: %+v", err)
		return false, err
	}

	err = starlark.ExecREPLChunk(f, e.thread, prevState)
	globals := prevState
	//globals, err := starlark.ExecFileOptions(e.options, e.thread, filename, starCode, prevState)
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
