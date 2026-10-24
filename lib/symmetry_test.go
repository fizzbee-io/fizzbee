package lib

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
)

// newOrdinalThread returns a thread with a symmetry context whose state holds
// the given ordinal IDs for domain "ts".
func newOrdinalThread(ids ...int64) *starlark.Thread {
	thread := &starlark.Thread{Name: "test"}
	thread.SetLocal(SymmetryContextKey, NewSymmetryContext(func() map[string][]int64 {
		return map[string][]int64{"ts": ids}
	}))
	return thread
}

// evalSegments runs expr with TIMES, a, b, c bound and returns the segments as strings.
func evalSegments(t *testing.T, thread *starlark.Thread, d *SymmetryDomain, expr string, vals ...int64) []string {
	t.Helper()
	env := starlark.StringDict{"TIMES": d}
	for i, id := range vals {
		env[string(rune('a'+i))] = NewSymmetricValueWithKind("ts", id, SymmetryKindOrdinal)
	}
	v, err := starlark.Eval(thread, "test", expr, env)
	require.NoError(t, err)
	list := v.(*starlark.List)
	var out []string
	for i := 0; i < list.Len(); i++ {
		out = append(out, list.Index(i).String())
	}
	return out
}

func TestOrdinalSegmentsIncludePoints(t *testing.T) {
	d := &SymmetryDomain{Name: "ts", Limit: 5, Kind: SymmetryKindOrdinal}
	ids := []int64{10, 20, 30}

	tests := []struct {
		expr string
		want []string
	}{
		{"TIMES.segments()", []string{"(-inf, 10)", "(10, 20)", "(20, 30)", "(30, +inf)"}},
		{"TIMES.segments(include_points=True)", []string{"(-inf, 10)", "[10, 10]", "(10, 20)", "[20, 20]", "(20, 30)", "[30, 30]", "(30, +inf)"}},
		{"TIMES.segments(include_points=True, ge=a, lt=c)", []string{"[10, 10]", "(10, 20)", "[20, 20]", "(20, 30)"}},
		{"TIMES.segments(include_points=True, gt=a, lt=c)", []string{"(10, 20)", "[20, 20]", "(20, 30)"}},
		{"TIMES.segments(include_points=True, gt=b, le=c)", []string{"(20, 30)", "[30, 30]"}},
		{"TIMES.segments(include_points=True, after=b)", []string{"(20, 30)", "[30, 30]", "(30, +inf)"}},
		{"TIMES.segments(include_points=True, le=a)", []string{"(-inf, 10)", "[10, 10]"}},
		{"TIMES.segments(after=a, before=c)", []string{"(10, 20)", "(20, 30)"}},
	}
	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			got := evalSegments(t, newOrdinalThread(ids...), d, tt.expr, ids...)
			for i := range got {
				got[i] = strings.TrimSuffix(strings.TrimPrefix(got[i], "<segment ts "), ">")
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestOrdinalSegmentsEmptyDomain(t *testing.T) {
	d := &SymmetryDomain{Name: "ts", Limit: 2, Kind: SymmetryKindOrdinal}
	got := evalSegments(t, newOrdinalThread(), d, "TIMES.segments(include_points=True)")
	assert.Equal(t, []string{"<segment ts (-inf, +inf)>"}, got)
}

func TestOrdinalSegmentsBoundConflicts(t *testing.T) {
	d := &SymmetryDomain{Name: "ts", Limit: 5, Kind: SymmetryKindOrdinal}
	env := starlark.StringDict{"TIMES": d, "a": NewSymmetricValueWithKind("ts", 10, SymmetryKindOrdinal)}
	for _, expr := range []string{
		"TIMES.segments(gt=a, ge=a)",
		"TIMES.segments(after=a, gt=a)",
		"TIMES.segments(lt=a, le=a)",
		"TIMES.segments(before=a, le=a)",
		"TIMES.segments(gt=1)",
	} {
		_, err := starlark.Eval(newOrdinalThread(10), "test", expr, env)
		assert.Error(t, err, expr)
	}
}

func TestOrdinalSegmentValue(t *testing.T) {
	d := &SymmetryDomain{Name: "ts", Limit: 3, Kind: SymmetryKindOrdinal}
	thread := newOrdinalThread(10, 20)
	segs, err := starlark.Eval(thread, "test", "TIMES.segments(include_points=True)", starlark.StringDict{"TIMES": d})
	require.NoError(t, err)
	list := segs.(*starlark.List)
	require.Equal(t, 5, list.Len())

	call := func(s starlark.Value, method string) (starlark.Value, error) {
		fn, err := s.(*Segment).Attr(method)
		require.NoError(t, err)
		return starlark.Call(thread, fn, nil, nil)
	}

	// Point: returns the existing value, allocates nothing, fresh() is an error.
	point := list.Index(1)
	isPoint, _ := point.(*Segment).Attr("is_point")
	assert.Equal(t, starlark.True, isPoint)
	v, err := call(point, "value")
	require.NoError(t, err)
	assert.Equal(t, int64(10), v.(*SymmetricValue).GetId())
	_, err = call(point, "fresh")
	assert.Error(t, err)

	// Gap (10, 20): allocates strictly inside, and is memoized.
	gap := list.Index(2)
	v1, err := call(gap, "value")
	require.NoError(t, err)
	id := v1.(*SymmetricValue).GetId()
	assert.True(t, id > 10 && id < 20, "id %d not in (10, 20)", id)
	v2, err := call(gap, "value")
	require.NoError(t, err)
	assert.Equal(t, id, v2.(*SymmetricValue).GetId())

	// The domain is now at its limit: another gap disables the transition.
	_, err = call(list.Index(4), "value")
	assert.True(t, IsDisableTransitionError(err), "got %v", err)
}
