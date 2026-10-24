package lib

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOrdinalRespacedIDs(t *testing.T) {
	// One value lands where fresh() puts the first value of an empty domain.
	assert.Equal(t, []int64{ordinalMid}, OrdinalRespacedIDs(1))

	for _, n := range []int{1, 2, 3, 10, 1000} {
		ids := OrdinalRespacedIDs(n)
		assert.Len(t, ids, n)
		assert.False(t, OrdinalNeedsRespace(ids), "respaced ids for n=%d need respacing", n)
		for i := 1; i < n; i++ {
			assert.Less(t, ids[i-1], ids[i])
		}
	}
}

func TestOrdinalNeedsRespace(t *testing.T) {
	assert.False(t, OrdinalNeedsRespace(nil))
	assert.False(t, OrdinalNeedsRespace([]int64{ordinalMid}))
	// Gap at the head, between values, and at the tail.
	assert.True(t, OrdinalNeedsRespace([]int64{OrdinalRespaceMinGap - 1, ordinalMid}))
	assert.True(t, OrdinalNeedsRespace([]int64{ordinalMid, ordinalMid + 1}))
	assert.True(t, OrdinalNeedsRespace([]int64{ordinalMid, ordinalMax - 1}))
	assert.False(t, OrdinalNeedsRespace([]int64{OrdinalRespaceMinGap, ordinalMax - OrdinalRespaceMinGap}))
}
