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
		process.Heap.state["x"] = starlark.MakeInt(1)
		failed := CheckInvariants(process)
		assert.Len(t, failed[0], 1)
		assert.Equal(t, 0, failed[0][0])
	})

}

func TestRejectRoleInvariants(t *testing.T) {
	roleAssertion := func(name string, line int32) *ast.Invariant {
		return &ast.Invariant{
			Name:              name,
			TemporalOperators: []string{"always"},
			SourceInfo:        &ast.SourceInfo{Start: &ast.Position{Line: line}},
		}
	}
	t.Run("topLevelOnly", func(t *testing.T) {
		file := &ast.File{
			Invariants: []*ast.Invariant{roleAssertion("Safe", 3)},
			Roles:      []*ast.Role{{Name: "Server"}},
		}
		assert.Nil(t, RejectRoleInvariants(file))
	})
	t.Run("roleAssertion", func(t *testing.T) {
		file := &ast.File{
			Roles: []*ast.Role{{Name: "Server", Invariants: []*ast.Invariant{roleAssertion("ValuePositive", 5)}}},
		}
		err := RejectRoleInvariants(file)
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "assertion 'ValuePositive' in role 'Server' (line 5)")
		assert.Contains(t, err.Error(), "issues/179")
	})
	t.Run("listsEveryOffender", func(t *testing.T) {
		file := &ast.File{
			Roles: []*ast.Role{
				{Name: "Server", Invariants: []*ast.Invariant{roleAssertion("A", 5), roleAssertion("B", 8)}},
				{Name: "Client", Invariants: []*ast.Invariant{roleAssertion("C", 12)}},
			},
		}
		err := RejectRoleInvariants(file)
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "assertion 'A' in role 'Server' (line 5)")
		assert.Contains(t, err.Error(), "assertion 'B' in role 'Server' (line 8)")
		assert.Contains(t, err.Error(), "assertion 'C' in role 'Client' (line 12)")
	})
	t.Run("deprecatedInvariantsBlock", func(t *testing.T) {
		file := &ast.File{
			Roles: []*ast.Role{{Name: "Server", Invariants: []*ast.Invariant{
				{Always: true, PyExpr: "self.value >= 0", SourceInfo: &ast.SourceInfo{Start: &ast.Position{Line: 7}}},
			}}},
		}
		err := RejectRoleInvariants(file)
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "unnamed assertion in role 'Server' (line 7)")
	})
}
