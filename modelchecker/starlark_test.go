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
