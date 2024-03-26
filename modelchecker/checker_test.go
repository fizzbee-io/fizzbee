package modelchecker

import (
	ast "fizz/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
	"google.golang.org/protobuf/encoding/protojson"
	"testing"
)

func TestExecInit(t *testing.T) {
	astJson := `
    {
        "states": {
			"code": "MAX_ELEMENTS = 5\nelements = set()\ncount = 0"
		}
    }
    `
	checker := NewModelChecker("test")
	f := &ast.File{}
	err := protojson.Unmarshal([]byte(astJson), f)
	require.Nil(t, err)
	vars, err := checker.ExecInit(f.States)
	require.Nil(t, err)
	require.NotNil(t, vars)
	assert.Len(t, vars, 3)
	assert.Equal(t, "int", vars["MAX_ELEMENTS"].Type())
	assert.Equal(t, "5", vars["MAX_ELEMENTS"].String())
	assert.Equal(t, "int", vars["count"].Type())
	assert.Equal(t, "0", vars["count"].String())
	assert.Equal(t, "set", vars["elements"].Type())
	assert.Equal(t, "set([])", vars["elements"].String())
}

func TestExecIfStmt(t *testing.T) {

}

func TestExecBlock(t *testing.T) {
	astJson := `
    {
        "stmts": [
		  {
			"pyStmt": {
			  "code": "elements = elements | set([e])"
			}
		  },
		  {
			"pyStmt": {
			  "code": "count = count + 1"
			}
		  }
		]
    }
    `
	checker := NewModelChecker("test")
	b := &ast.Block{}

	t.Run("empty_block", func(t *testing.T) {
		vars := starlark.StringDict{}
		valid, err := checker.ExecBlock("name.fizz", b, vars)
		require.Nil(t, err)
		assert.False(t, valid)
	})
	t.Run("with_simple_stmts", func(t *testing.T) {
		vars := starlark.StringDict{}
		err := protojson.Unmarshal([]byte(astJson), b)
		require.Nil(t, err)

		vars["count"] = starlark.MakeInt(5)
		vars["elements"] = starlark.NewSet(3)
		vars["e"] = starlark.String("a")
		valid, err := checker.ExecBlock("name.fizz", b, vars)
		require.Nil(t, err)
		assert.True(t, valid)
		assert.Equal(t, "int", vars["count"].Type())
		assert.Equal(t, "6", vars["count"].String())
		assert.Equal(t, "set", vars["elements"].Type())
		assert.Equal(t, "set([\"a\"])", vars["elements"].String())
	})
	t.Run("with_nested_block", func(t *testing.T) {
		vars := starlark.StringDict{}
		b2 := &ast.Block{}
		err := protojson.Unmarshal([]byte(astJson), b)
		require.Nil(t, err)
		err = protojson.Unmarshal([]byte(astJson), b2)
		require.Nil(t, err)
		b.Stmts = append(b.Stmts, &ast.Statement{Block: b2})
		vars["count"] = starlark.MakeInt(5)
		vars["elements"] = starlark.NewSet(3)
		vars["e"] = starlark.String("a")
		valid, err := checker.ExecBlock("name.fizz", b, vars)
		require.Nil(t, err)
		assert.True(t, valid)
		assert.Equal(t, "int", vars["count"].Type())
		assert.Equal(t, "7", vars["count"].String())
		assert.Equal(t, "set", vars["elements"].Type())
		assert.Equal(t, "set([\"a\"])", vars["elements"].String())
	})

}
