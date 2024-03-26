package modelchecker

import (
	ast "fizz/proto"
	"github.com/stretchr/testify/assert"
	"go.starlark.net/starlark"
	"testing"
)

func TestCheckInvariants(t *testing.T) {
	t.Run("alwaysTrue", func(t *testing.T) {
		file0 := &ast.File{
			Invariants: []*ast.Invariant{
				&ast.Invariant{Always: true, PyExpr: "True"},
			},
		}

		process := NewProcess("example", []*ast.File{file0}, nil)

		failed := CheckInvariants(process)
		assert.Len(t, failed[0], 0)
	})
	t.Run("alwaysFalse", func(t *testing.T) {
		file0 := &ast.File{
			Invariants: []*ast.Invariant{
				&ast.Invariant{Always: true, PyExpr: "False"},
			},
		}

		process := NewProcess("example", []*ast.File{file0}, nil)

		failed := CheckInvariants(process)
		assert.Len(t, failed[0], 1)
		assert.Equal(t, 0, failed[0][0])
	})
	t.Run("multipleSimple", func(t *testing.T) {
		file0 := &ast.File{
			Invariants: []*ast.Invariant{
				&ast.Invariant{Always: true, PyExpr: "True"},
				&ast.Invariant{Always: true, PyExpr: "False"},
			},
		}

		process := NewProcess("example", []*ast.File{file0}, nil)

		failed := CheckInvariants(process)
		assert.Len(t, failed[0], 1)
		assert.Equal(t, 1, failed[0][0])
	})
	t.Run("expr", func(t *testing.T) {
		file0 := &ast.File{
			Invariants: []*ast.Invariant{
				&ast.Invariant{Always: true, PyExpr: "x > 2"},
				&ast.Invariant{Always: true, PyExpr: "x < 2"},
			},
		}

		process := NewProcess("example", []*ast.File{file0}, nil)
		process.Heap.globals["x"] = starlark.MakeInt(1)
		failed := CheckInvariants(process)
		assert.Len(t, failed[0], 1)
		assert.Equal(t, 0, failed[0][0])
	})

}
