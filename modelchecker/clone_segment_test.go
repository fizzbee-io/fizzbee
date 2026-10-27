package modelchecker

import (
	"testing"

	"github.com/fizzbee-io/fizzbee/lib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
)

var cloneTestDomain = &lib.SymmetryDomain{Name: "ts", Limit: 5, Kind: lib.SymmetryKindOrdinal}

func ordinalPermutations(mapping map[int64]int64) map[*lib.SymmetricValue][]*lib.SymmetricValue {
	perms := make(map[*lib.SymmetricValue][]*lib.SymmetricValue)
	for from, to := range mapping {
		perms[lib.NewSymmetricValueWithKind("ts", from, lib.SymmetryKindOrdinal)] =
			[]*lib.SymmetricValue{lib.NewSymmetricValueWithKind("ts", to, lib.SymmetryKindOrdinal)}
	}
	return perms
}

func TestCloneSegmentKeepsAliases(t *testing.T) {
	seg := &lib.Segment{Domain: cloneTestDomain, Left: 10, Right: 20}
	list := starlark.NewList([]starlark.Value{seg, seg})

	cloned, err := deepCloneStarlarkValue(list, nil)
	require.NoError(t, err)
	clonedList := cloned.(*starlark.List)

	first := clonedList.Index(0).(*lib.Segment)
	assert.NotSame(t, seg, first)
	// Both references must still point at one segment, so value() memoizes for both.
	assert.Same(t, first, clonedList.Index(1).(*lib.Segment))
}

func TestCloneSegmentRemapsResolvedID(t *testing.T) {
	seg := &lib.Segment{Domain: cloneTestDomain, Left: 10, Right: 20, Resolved: true, ResolvedID: 15}
	perms := ordinalPermutations(map[int64]int64{10: 0, 15: 1, 20: 2})

	cloned, err := deepCloneStarlarkValueWithPermutations(seg, map[starlark.Value]starlark.Value{}, perms, 0)
	require.NoError(t, err)
	got := cloned.(*lib.Segment)
	assert.Equal(t, int64(0), got.Left)
	assert.Equal(t, int64(2), got.Right)
	assert.True(t, got.Resolved)
	assert.Equal(t, int64(1), got.ResolvedID)
}

func TestCloneSegmentDropsUnmappedResolvedID(t *testing.T) {
	// The resolved value 15 is no longer in the state, so it has no permuted
	// counterpart: the memo is dropped rather than keeping a raw ID that may
	// not fit between the remapped endpoints.
	seg := &lib.Segment{Domain: cloneTestDomain, Left: 10, Right: 20, Resolved: true, ResolvedID: 15}
	perms := ordinalPermutations(map[int64]int64{10: 0, 20: 1})

	cloned, err := deepCloneStarlarkValueWithPermutations(seg, map[starlark.Value]starlark.Value{}, perms, 0)
	require.NoError(t, err)
	got := cloned.(*lib.Segment)
	assert.Equal(t, int64(0), got.Left)
	assert.Equal(t, int64(1), got.Right)
	assert.False(t, got.Resolved)
}

func TestCloneSegmentWithoutPermutationsKeepsMemo(t *testing.T) {
	seg := &lib.Segment{Domain: cloneTestDomain, Left: 10, Right: 20, Resolved: true, ResolvedID: 15}
	cloned, err := deepCloneStarlarkValue(seg, nil)
	require.NoError(t, err)
	got := cloned.(*lib.Segment)
	assert.True(t, got.Resolved)
	assert.Equal(t, int64(15), got.ResolvedID)
}
