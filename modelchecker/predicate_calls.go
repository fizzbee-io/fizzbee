package modelchecker

import (
	ast "fizz/proto"
	"fmt"
	"maps"

	"github.com/fizzbee-io/fizzbee/lib"
	"go.starlark.net/starlark"
)

// maxFizzCallDepth bounds recursion of fizz funcs called inside predicates.
const maxFizzCallDepth = 64

// maxPredicateSteps bounds the statements one predicate evaluation may run,
// including the funcs it calls, so an endless loop fails instead of hanging.
// A var so tests can lower it.
var maxPredicateSteps = 1_000_000

// enablePredicateCalls lets fizz funcs be called inside expressions evaluated
// on p, for example `not c.is_zero()` or `total() == MAX` in an assertion.
// p must be a clone made for predicate evaluation: each call runs on a fresh
// clone of p, so a func can't change p or leak writes into another call.
func (p *Process) enablePredicateCalls() {
	p.fizzCall = p.callFizzFunc
	if p.predicateSteps == nil {
		steps := maxPredicateSteps
		p.predicateSteps = &steps
	}
}

// countPredicateStep charges one statement to the predicate budget, if any.
func (p *Process) countPredicateStep(stmt *ast.Statement) {
	if p.predicateSteps == nil {
		return
	}
	*p.predicateSteps--
	if *p.predicateSteps < 0 {
		panic(p.NewModelError(stmt.GetSourceInfo(), fmt.Sprintf("assertion ran more than %d statements; is there an endless loop?", maxPredicateSteps), nil))
	}
}

// addPredicateFuncs binds the top-level fizz funcs as builtins in dict, when
// predicate calls are enabled. A user-defined name with the same spelling
// shadows the func, as it would in Python.
func (p *Process) addPredicateFuncs(dict starlark.StringDict) {
	if p.fizzCall == nil {
		return
	}
	for name, def := range p.SymbolTable {
		if def.DefType != Function || def.roleName != "" {
			continue
		}
		if _, ok := dict[name]; ok && lib.Builtins[name] == nil {
			continue
		}
		fnName := name
		dict[fnName] = starlark.NewBuiltin(fnName, func(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			return p.callFizzFunc(t, nil, fnName, args, kwargs)
		})
	}
}

// callFizzFunc runs the fizz func name (a role method when recv is non-nil)
// to completion on a fresh clone of p and returns its result. A func that
// forks (oneof, any, non-atomic interleaving) is an error.
func (p *Process) callFizzFunc(_ *starlark.Thread, recv *lib.Role, name string, args starlark.Tuple, kwargs []starlark.Tuple) (result starlark.Value, err error) {
	key := name
	if recv != nil {
		key = recv.Name + "." + name
	}
	def := p.SymbolTable[key]
	if def == nil || def.DefType != Function {
		return nil, fmt.Errorf("%s is not a fizz func", key)
	}
	if p.fizzCallDepth >= maxFizzCallDepth {
		return nil, fmt.Errorf("%s: fizz func calls nested deeper than %d", key, maxFizzCallDepth)
	}

	refs := make(map[starlark.Value]starlark.Value)
	clone := p.cloneSharingBuiltins(refs)
	clone.predicateSteps = p.predicateSteps
	clone.enablePredicateCalls()
	clone.fizzCallDepth = p.fizzCallDepth + 1
	delete(clone.Returns, name)

	// Map arguments into the clone, so a func that mutates an argument
	// can't change the caller's values.
	vars := starlark.StringDict{}
	if len(args) > len(def.params) {
		return nil, fmt.Errorf("%s: got %d arguments, want at most %d", key, len(args), len(def.params))
	}
	for i, arg := range args {
		v, err := deepCloneStarlarkValue(arg, refs)
		if err != nil {
			return nil, fmt.Errorf("%s: argument %d: %w", key, i, err)
		}
		vars[def.params[i].Name] = v
	}
	for _, kv := range kwargs {
		k := string(kv[0].(starlark.String))
		if !hasParam(def, k) {
			return nil, fmt.Errorf("%s: unexpected keyword argument %s", key, k)
		}
		if _, ok := vars[k]; ok {
			return nil, fmt.Errorf("%s: got multiple values for argument %s", key, k)
		}
		v, err := deepCloneStarlarkValue(kv[1], refs)
		if err != nil {
			return nil, fmt.Errorf("%s: argument %s: %w", key, k, err)
		}
		vars[k] = v
	}

	thread := clone.NewThread()
	frame := thread.currentFrame()
	frame.FileIndex = def.fileIndex
	frame.pc = def.path + ".Block"
	frame.Name = name
	if recv != nil {
		obj, ok := refs[recv].(*lib.Role)
		if !ok {
			return nil, fmt.Errorf("%s: receiver %s is not part of the state", key, recv.RefStringShort())
		}
		frame.obj = obj
	}
	frame.vars = vars

	defer func() {
		if r := recover(); r != nil {
			result, err = nil, fmt.Errorf("%s: %v", key, r)
		}
	}()
	for _, param := range def.params {
		if _, ok := vars[param.Name]; ok {
			continue
		}
		if param.DefaultPyExpr == "" {
			return nil, fmt.Errorf("%s: missing argument %s", key, param.Name)
		}
		// The new frame has no scope yet, so build the variables by hand:
		// state, self and the arguments bound so far.
		env := maps.Clone(lib.Builtins)
		maps.Copy(env, clone.Heap.globals)
		maps.Copy(env, clone.Heap.state)
		if frame.obj != nil {
			env["self"] = frame.obj
		}
		maps.Copy(env, vars)
		clone.addPredicateFuncs(env)
		v, err := clone.Evaluator.EvalExprWithContext(thread.getFileName(), param.DefaultExpr, env, clone.createSymmetryContext())
		if err != nil {
			return nil, fmt.Errorf("%s: default for %s: %w", key, param.Name, err)
		}
		vars[param.Name] = v
	}

	numThreads := clone.GetThreadsCount() - 1
	for {
		forks, _ := thread.Execute()
		if thread.Aborted {
			return nil, fmt.Errorf("%s: a require statement failed; a fizz func called in an expression can't disable a transition", key)
		}
		if len(forks) > 0 {
			return nil, fmt.Errorf("%s: a fizz func called in an expression must be deterministic (no oneof, any or non-atomic interleaving)", key)
		}
		if clone.GetThreadsCount() <= numThreads {
			break
		}
	}
	if v, ok := clone.Returns[name]; ok && v != nil {
		return v, nil
	}
	return starlark.None, nil
}

// cloneSharingBuiltins clones p for a call. Builtins installed in the heap
// (next_states) can't be deep-cloned, so they are set aside during the clone
// and shared with it. They are put back even if cloning panics, so p is
// unchanged on every path.
func (p *Process) cloneSharingBuiltins(refs map[starlark.Value]starlark.Value) *Process {
	builtins := starlark.StringDict{}
	for k, v := range p.Heap.state {
		if _, ok := v.(*starlark.Builtin); ok {
			builtins[k] = v
			delete(p.Heap.state, k)
		}
	}
	defer maps.Copy(p.Heap.state, builtins)
	clone := p.CloneWithRefs(nil, 0, refs)
	maps.Copy(clone.Heap.state, builtins)
	return clone
}

func hasParam(def *Definition, name string) bool {
	for _, p := range def.params {
		if p.Name == name {
			return true
		}
	}
	return false
}
