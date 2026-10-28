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
