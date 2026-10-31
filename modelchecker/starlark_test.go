package modelchecker

import (
	ast "fizz/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
	"testing"
)

func TestEvalPyExpr(t *testing.T) {
	checker := NewModelChecker("test")
	predeclared := starlark.StringDict{}
	predeclared["MAX_ELEMENTS"] = starlark.MakeInt(5)

	t.Run("valid_range_expr_fileportion", func(t *testing.T) {
		fileportion := syntax.FilePortion{
			Content:   []byte("range(0, MAX_ELEMENTS)"),
			FirstLine: 10,
			FirstCol:  11,
		}
		val, err := checker.EvalPyExpr("myname.fizz", fileportion, predeclared)
		require.Nil(t, err)
		assert.Equal(t, "range", val.Type())
		assert.Equal(t, "range(5)", val.String())
	})
	t.Run("valid_range_expr", func(t *testing.T) {
		val, err := checker.EvalPyExpr("myname.fizz", "range(0, MAX_ELEMENTS)", predeclared)
		require.Nil(t, err)
		assert.Equal(t, "range", val.Type())
		assert.Equal(t, "range(5)", val.String())
	})
	t.Run("valid_exists_expr", func(t *testing.T) {
		val, err := checker.EvalPyExpr("myname.fizz", "3 in range(0, MAX_ELEMENTS)", predeclared)
		require.Nil(t, err)
		assert.Equal(t, "bool", val.Type())
		assert.Equal(t, "True", val.String())
	})
	t.Run("valid_not_exists_expr", func(t *testing.T) {
		val, err := checker.EvalPyExpr("myname.fizz", "3 not in range(0, MAX_ELEMENTS)", predeclared)
		require.Nil(t, err)
		assert.Equal(t, "bool", val.Type())
		assert.Equal(t, "False", val.String())
	})
	t.Run("invalid_syntax", func(t *testing.T) {
		val, err := checker.EvalPyExpr("myname.fizz", "ra nge(0, MAX_ELEMENTS)", predeclared)
		require.Nil(t, val)
		require.NotNil(t, err)
	})
	t.Run("runtime_error", func(t *testing.T) {
		val, err := checker.EvalPyExpr("myname.fizz", "range(0, MAX_ELEMENTS1)", predeclared)
		require.Nil(t, val)
		require.NotNil(t, err)
	})

}

func TestExecPyStmt(t *testing.T) {
	checker := NewModelChecker("test")
	globals := starlark.StringDict{}
	globals["count"] = starlark.MakeInt(5)
	globals["elements"] = starlark.NewSet(3)

	t.Run("valid_incr_stmt", func(t *testing.T) {
		pystmt := &ast.PyStmt{
			Code: "count = count + 1",
		}
		valid, err := checker.ExecPyStmt("myname.fizz", pystmt, globals)
		require.Nil(t, err)
		assert.True(t, valid)
		assert.Equal(t, "int", globals["count"].Type())
		assert.Equal(t, "6", globals["count"].String())
	})

	t.Run("valid_set_stmt", func(t *testing.T) {
		pystmt := &ast.PyStmt{
			Code: "elements = elements | set(['a'])",
		}
		valid, err := checker.ExecPyStmt("myname.fizz", pystmt, globals)
		require.Nil(t, err)
		assert.True(t, valid)
		assert.Equal(t, "set", globals["elements"].Type())
		assert.Equal(t, "set([\"a\"])", globals["elements"].String())
	})

	t.Run("invalid_syntax_error", func(t *testing.T) {
		pystmt := &ast.PyStmt{
			Code: "count = /count ",
		}
		valid, err := checker.ExecPyStmt("myname.fizz", pystmt, globals)
		require.NotNil(t, err)
		require.False(t, valid)
	})

	t.Run("runtime_error", func(t *testing.T) {
		pystmt := &ast.PyStmt{
			Code: "count = count + undefiled_val",
		}
		valid, err := checker.ExecPyStmt("myname.fizz", pystmt, globals)
		require.NotNil(t, err)
		require.False(t, valid)
	})
}

func cacheTestExpr(src string, line, col int32) *ast.Expr {
	return &ast.Expr{
		PyExpr: src,
		SourceInfo: &ast.SourceInfo{
			Start: &ast.Position{Line: line, Column: col},
		},
	}
}

func TestEvalExprCacheDialectOptions(t *testing.T) {
	// The same expression, file and position under two dialects: the
	// program compiled for one must not be reused for the other, in either
	// order.
	expr := cacheTestExpr("len(set([1, 1, 2]))", 7001, 5)
	withSet := NewEvaluator(&syntax.FileOptions{Set: true}, nil)
	withoutSet := NewEvaluator(&syntax.FileOptions{}, nil)

	v, err := withSet.EvalExpr("dialect.fizz", expr, starlark.StringDict{})
	require.NoError(t, err)
	assert.Equal(t, starlark.MakeInt(2), v)
	_, err = withoutSet.EvalExpr("dialect.fizz", expr, starlark.StringDict{})
	assert.Error(t, err, "set is not in the dialect without Set")

	expr = cacheTestExpr("len(set([1, 1, 2]))", 7002, 5)
	_, err = withoutSet.EvalExpr("dialect.fizz", expr, starlark.StringDict{})
	assert.Error(t, err)
	v, err = withSet.EvalExpr("dialect.fizz", expr, starlark.StringDict{})
	require.NoError(t, err)
	assert.Equal(t, starlark.MakeInt(2), v)
}

func TestEvalExprCacheEnvironments(t *testing.T) {
	checker := NewModelChecker("test")
	eval := func(expr *ast.Expr, env starlark.StringDict) (starlark.Value, error) {
		return checker.EvalExpr("env.fizz", expr, env)
	}

	// Reuse with the same names and different values.
	sum := cacheTestExpr("x + 1", 7101, 1)
	v, err := eval(sum, starlark.StringDict{"x": starlark.MakeInt(1)})
	require.NoError(t, err)
	assert.Equal(t, starlark.MakeInt(2), v)
	v, err = eval(sum, starlark.StringDict{"x": starlark.MakeInt(5)})
	require.NoError(t, err)
	assert.Equal(t, starlark.MakeInt(6), v)

	// A name the environment lacks is still a resolve error, and a later
	// environment that has it still works.
	_, err = eval(sum, starlark.StringDict{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "env.fizz:7101:1: undefined: x")
	v, err = eval(sum, starlark.StringDict{"x": starlark.MakeInt(9)})
	require.NoError(t, err)
	assert.Equal(t, starlark.MakeInt(10), v)

	// A global that shadows a builtin selects a different compiled variant.
	shadow := cacheTestExpr("str", 7201, 1)
	v, err = eval(shadow, starlark.StringDict{"str": starlark.String("mine")})
	require.NoError(t, err)
	assert.Equal(t, starlark.String("mine"), v)
	v, err = eval(shadow, starlark.StringDict{})
	require.NoError(t, err)
	assert.Equal(t, "builtin_function_or_method", v.Type())

	// Names bound inside the expression stay local, whatever the
	// environment defines under the same name.
	comp := cacheTestExpr("[i * x for i in range(3)]", 7301, 1)
	v, err = eval(comp, starlark.StringDict{"x": starlark.MakeInt(2)})
	require.NoError(t, err)
	assert.Equal(t, "[0, 2, 4]", v.String())
	v, err = eval(comp, starlark.StringDict{"x": starlark.MakeInt(3), "i": starlark.MakeInt(100)})
	require.NoError(t, err)
	assert.Equal(t, "[0, 3, 6]", v.String())
}

func TestEvalExprCacheErrorPosition(t *testing.T) {
	checker := NewModelChecker("test")
	// A runtime error on the second line of a multi-line expression keeps
	// the expression's own line and column.
	expr := cacheTestExpr("(1 +\n     1 // zero)", 7401, 9)
	_, err := checker.EvalExpr("pos.fizz", expr, starlark.StringDict{"zero": starlark.MakeInt(0)})
	require.Error(t, err)
	evalErr, ok := err.(*starlark.EvalError)
	require.True(t, ok, "want an EvalError, got %T", err)
	assert.Contains(t, evalErr.Backtrace(), "pos.fizz:7402:")
	assert.Contains(t, evalErr.Error(), "division by zero")
}

// compiledEquivEnv is a fresh environment for each run, so a statement that
// mutates a value in place doesn't leak between the two paths.
func compiledEquivEnv() starlark.StringDict {
	d := starlark.NewDict(1)
	_ = d.SetKey(starlark.String("k"), starlark.MakeInt(0))
	return starlark.StringDict{
		"x":      starlark.MakeInt(10),
		"zero":   starlark.MakeInt(0),
		"lst":    starlark.NewList([]starlark.Value{starlark.MakeInt(1), starlark.MakeInt(2)}),
		"d":      d,
		"pair":   starlark.Tuple{starlark.MakeInt(1), starlark.MakeInt(2)},
		"nested": starlark.Tuple{starlark.Tuple{starlark.MakeInt(1), starlark.MakeInt(2)}, starlark.MakeInt(3)},
	}
}

func compiledEquivStmt(code string, line, col int32) *ast.PyStmt {
	return &ast.PyStmt{
		Code:       code,
		SourceInfo: &ast.SourceInfo{Start: &ast.Position{Line: line, Column: col}},
	}
}

// errPos is the position of the innermost frame of a Starlark error.
func errPos(err error) string {
	if evalErr, ok := err.(*starlark.EvalError); ok && len(evalErr.CallStack) > 0 {
		return evalErr.CallStack[len(evalErr.CallStack)-1].Pos.String()
	}
	return ""
}

func envStrings(env starlark.StringDict) map[string]string {
	m := map[string]string{}
	for k, v := range env {
		m[k] = v.String()
	}
	return m
}

// TestCompiledStmtMatchesREPL runs each sequence of statements through the
// compiled-function path and through the REPL path, each on its own fresh
// environment, and requires the same environment afterwards and the same
// error, at the same position, for a statement that fails.
func TestCompiledStmtMatchesREPL(t *testing.T) {
	checker := NewModelChecker("test")
	cases := []struct {
		name  string
		stmts []string
	}{
		{"rebind", []string{"x = x + 1"}},
		{"augmented", []string{"x += 1", "lst += [3]"}},
		{"new_name_visible_to_next", []string{"y = x + 1", "z = y * 2"}},
		{"tuple_unpack", []string{"a, b = pair"}},
		{"list_unpack", []string{"[a, b] = pair"}},
		{"nested_unpack", []string{"(a, b), c = nested"}},
		{"swap", []string{"a, b = pair", "a, b = b, a"}},
		{"index_targets", []string{"d['k'] = x", "lst[0] = x", "d['n'] = d['k'] + 1"}},
		{"in_place_call", []string{"lst.append(x)", "d.update({'m': 1})"}},
		{"comprehension", []string{"y = [x * i for i in lst if i > 1]", "z = {i: x for i in lst}"}},
		{"shadow_builtin", []string{"len = 3", "y = len + x"}},
		{"lambda_closure", []string{"f = lambda v: v + x", "x = 100", "y = f(1)"}},
		{"lambda_rebind_same_chunk", []string{"x = 10; f = lambda: x; x = 20", "y = f()"}},
		{"lambda_rebind_later", []string{"f = lambda: x", "x = 20", "y = f()", "x = 30", "z = f()"}},
		{"lambda_param_shadows_env", []string{"f = lambda x: x * 2", "y = f(3) + x"}},
		{"comprehension_var_shadows_env", []string{"x = [x for x in lst]"}},
		{"two_statements_one_line", []string{"a = 1; b = a + x"}},
		{"multiline", []string{"y = (x +\n      1)"}},
		{"runtime_error", []string{"y = x // zero"}},
		{"runtime_error_second_line", []string{"y = (x +\n      x // zero)"}},
		{"index_error", []string{"y = lst[5]"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			compiledEnv, replEnv := compiledEquivEnv(), compiledEquivEnv()
			for i, code := range tc.stmts {
				stmt := compiledEquivStmt(code, int32(20+i), 9)
				start := stmt.GetSourceInfo().GetStart()
				c := checker.compiledStmt("equiv.fizz", code, start.GetLine(), start.GetColumn(), compiledEnv)
				require.NotNil(t, c, "statement %q should take the compiled path", code)
				_, compiledErr := c.exec(newEvalThread(nil), compiledEnv)
				_, replErr := checker.execPyStmtREPL("equiv.fizz", stmt, replEnv, nil)
				if replErr != nil || compiledErr != nil {
					require.Error(t, compiledErr, "REPL failed with %v", replErr)
					require.Error(t, replErr, "compiled path failed with %v", compiledErr)
					assert.Equal(t, errPos(replErr), errPos(compiledErr), "error position")
					return
				}
				assert.Equal(t, envStrings(replEnv), envStrings(compiledEnv), "after %q", code)
			}
		})
	}
}

// TestCompiledStmtFallsBack checks that a statement reading a name it
// binds before that name exists keeps the REPL path and its error.
func TestCompiledStmtFallsBack(t *testing.T) {
	checker := NewModelChecker("test")
	for _, code := range []string{"w = w + 1", "w += 1", "a, b = 1, a", "d[w] = 1; w = 2", "w = [w for w in lst]", "def g(): pass", "if x: y = 1"} {
		assert.Nil(t, checker.compiledStmt("equiv.fizz", code, 20, 9, compiledEquivEnv()), code)
	}
	assert.NotNil(t, checker.compiledStmt("equiv.fizz", "w = 1; v = w + 1", 20, 9, compiledEquivEnv()))
}

// TestCompiledExprMatchesProgram does the same for expressions.
func TestCompiledExprMatchesProgram(t *testing.T) {
	checker := NewModelChecker("test")
	cases := []string{
		"x + 1",
		"[i * x for i in lst]",
		"d['k'] == 0 and len(lst) > 1",
		"pair[0] if x > 5 else pair[1]",
		"(x +\n     lst[1])",
		"x // zero",
		"(x +\n     x // zero)",
		"lst[5]",
	}
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			expr := cacheTestExpr(src, 30, 13)
			start := expr.GetSourceInfo().GetStart()
			c := checker.compiledExprFunc("equiv.fizz", src, start.GetLine(), start.GetColumn(), compiledEquivEnv())
			require.NotNil(t, c, "expression %q should take the compiled path", src)
			env := compiledEquivEnv()
			args := make(starlark.Tuple, len(c.params))
			for i, name := range c.params {
				args[i] = env[name]
			}
			compiledVal, compiledErr := starlark.Call(newEvalThread(nil), c.fn, args, nil)
			progVal, progErr := checker.evalExprProgram("equiv.fizz", expr, compiledEquivEnv(), nil)
			if progErr != nil || compiledErr != nil {
				require.Error(t, compiledErr, "program path failed with %v", progErr)
				require.Error(t, progErr, "compiled path failed with %v", compiledErr)
				assert.Equal(t, errPos(progErr), errPos(compiledErr), "error position")
				return
			}
			assert.Equal(t, progVal.String(), compiledVal.String())
		})
	}
}
