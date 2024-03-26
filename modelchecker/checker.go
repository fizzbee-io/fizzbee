package modelchecker

import (
	ast "fizz/proto"
	"fmt"
	"github.com/golang/glog"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
	"log"
)

type Evaluator struct {
	options *syntax.FileOptions
	thread  *starlark.Thread
}

func NewEvaluator(options *syntax.FileOptions, thread *starlark.Thread) *Evaluator {
	return &Evaluator{
		options: options,
		thread:  thread,
	}
}

func (e *Evaluator) ExecAction(filename string, action *ast.Action, prevState starlark.StringDict) (bool, error) {
	return e.ExecBlock(filename, action.Block, prevState)
}

func (e *Evaluator) ExecBlock(filename string, block *ast.Block, prevState starlark.StringDict) (bool, error) {
	valid := false
	for _, stmt := range block.Stmts {
		if stmt.PyStmt != nil {
			pyStmtRes, err := e.ExecPyStmt(filename, stmt.PyStmt, prevState)
			if err != nil {
				return false, err
			}
			valid = pyStmtRes || valid
		} else if stmt.Block != nil {
			nextedBlockRes, err := e.ExecBlock(filename, stmt.Block, prevState)
			if err != nil {
				return false, err
			}
			valid = nextedBlockRes || valid
		} else if stmt.AnyStmt != nil {
			anyStmtRes, err := e.ExecAnyStmt(filename, stmt.AnyStmt, prevState)
			if err != nil {
				return false, err
			}
			valid = anyStmtRes || valid
		} else if stmt.ForStmt != nil {
			valid = e.ExecForStmt(filename, stmt.ForStmt, prevState) || valid
		} else if stmt.IfStmt != nil {
			valid = e.ExecIfStmt(filename, stmt.IfStmt, prevState) || valid
		}
	}
	return valid, nil
}

func (e *Evaluator) ExecIfStmt(filename string, ifStmt *ast.IfStmt, prevState starlark.StringDict) bool {

	for _, branch := range ifStmt.Branches {
		val, _ := e.EvalPyExpr(filename, branch.Condition, prevState)
		fmt.Printf("Branch: %v, value: %s", branch, val)
		if val.Truth() {
			v, _ := e.ExecBlock(filename, branch.Block, prevState)
			return v
		}
	}
	// no branches matched. That is, there are no else block and none of the if/elif conditions matched.
	return false
}

func (e *Evaluator) ExecForStmt(filename string, forStmt *ast.ForStmt, prevState starlark.StringDict) bool {
	valid := false

	val, _ := e.EvalPyExpr(filename, forStmt.PyExpr, prevState)
	rangeVal, _ := val.(starlark.Iterable)
	iter := rangeVal.Iterate()
	defer iter.Done()
	var x starlark.Value
	if len(forStmt.LoopVars) != 1 {
		log.Fatal("Not supported: multiple variables in forstmt")
	}
	loopVar := forStmt.LoopVars[0]
	if _, ok := prevState[loopVar]; ok {
		log.Fatal("Not supported: overriding variables in nested scope")
	}
	for iter.Next(&x) {
		fmt.Printf("Iter: %s, Type: %s\n", x, x.Type())
		prevState[loopVar] = x
		match, _ := e.ExecBlock(filename, forStmt.Block, prevState)
		valid = match || valid
	}
	delete(prevState, loopVar)
	return valid
}

func (e *Evaluator) ExecAnyStmt(filename string, anyStmt *ast.AnyStmt, prevState starlark.StringDict) (bool, error) {
	valid := false

	val, err := e.EvalPyExpr(filename, anyStmt.PyExpr, prevState)
	if err != nil {
		return false, err
	}
	rangeVal, _ := val.(starlark.Iterable)
	iter := rangeVal.Iterate()
	defer iter.Done()

	fmt.Printf("LoopVars: %s\n", anyStmt.LoopVars)
	if len(anyStmt.LoopVars) != 1 {
		log.Fatal("Not supported: multiple variables in anystmt")
	}
	loopVar := anyStmt.LoopVars[0]
	if _, ok := prevState[loopVar]; ok {
		log.Fatal("Not supported: overriding variables in nested scope")
	}
	var x starlark.Value
	for iter.Next(&x) {
		fmt.Printf("Iter: %s, Type: %s\n", x, x.Type())
		prevState[loopVar] = x
		match, _ := e.ExecBlock(filename, anyStmt.Block, prevState)
		valid = match || valid
		if match {
			break
		}
	}
	delete(prevState, loopVar)
	return valid, nil
}

func (e *Evaluator) ExecInit(variables *ast.StateVars) (starlark.StringDict, error) {

	initStr := variables.GetCode()

	predeclared := starlark.StringDict{}

	f, err := e.options.Parse("apparent/filename.star", initStr, 0)
	if err != nil {
		glog.Errorf("Error parsing expr: %+v", err)
		return nil, err
	}

	err = starlark.ExecREPLChunk(f, e.thread, predeclared)
	return predeclared, err

	//glog.Info("Running Init")
	//globals, err := starlark.ExecFileOptions(e.options, e.thread, "apparent/filename.star", initStr, predeclared)
	//if err != nil {
	//	glog.Error("Error in init: %+v", err)
	//}
	//return globals, err
}

func (e *Evaluator) ExecInitOld(variables *ast.StateVars) (starlark.StringDict, error) {

	initStr := variables.GetCode()

	predeclared := starlark.StringDict{}

	glog.Info("Running Init")
	globals, err := starlark.ExecFileOptions(e.options, e.thread, "apparent/filename.star", initStr, predeclared)
	if err != nil {
		glog.Error("Error in init: %+v", err)
	}
	return globals, err
}

func NewModelChecker(name string) *Evaluator {
	thread := &starlark.Thread{
		Name:  name,
		Print: func(_ *starlark.Thread, msg string) { fmt.Println(msg) },
	}
	options := &syntax.FileOptions{Set: true, GlobalReassign: true, TopLevelControl: true}

	mc := NewEvaluator(options, thread)
	return mc
}
