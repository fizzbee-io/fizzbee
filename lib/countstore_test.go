package lib

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func capOf(n int) int {
	if n > Many {
		return Many
	}
	return n
}

// Random adds and removes against exact counts. An exact store matches the
// counts; a capped store matches their abstraction, when each removal from
// Many picks the outcome that matches the concrete count. Entries stay
// sorted with no zero counts.
func TestCountStoreMatchesConcreteCounts(t *testing.T) {
	keys := []string{"a", "b", "c", "d"}
	for _, policy := range []CountPolicy{CountExact, CountCapped} {
		r := rand.New(rand.NewSource(int64(policy) + 1))
		for run := 0; run < 200; run++ {
			s := NewCountStore[int](policy)
			counts := map[string]int{}
			for op := 0; op < 40; op++ {
				k := keys[r.Intn(len(keys))]
				if counts[k] == 0 || r.Intn(2) == 0 {
					s.Add(k, len(k))
					counts[k]++
				} else {
					outs, err := s.RemoveOutcomes(k)
					require.NoError(t, err)
					counts[k]--
					want := counts[k]
					if policy == CountCapped {
						want = capOf(want)
					}
					require.Contains(t, outs, want)
					require.NoError(t, s.Remove(k, want))
				}
				total, distinct := 0, 0
				for _, k := range keys {
					want := counts[k]
					if policy == CountCapped {
						want = capOf(want)
					}
					assert.Equal(t, want, s.Count(k), "%s", k)
					assert.Equal(t, counts[k] > 0, s.Has(k))
					total += counts[k]
					if counts[k] > 0 {
						distinct++
					}
				}
				if policy == CountCapped {
					total = capOf(total)
				}
				assert.Equal(t, distinct, s.Len())
				assert.Equal(t, total, s.Total())
				for i, e := range s.Entries() {
					assert.NotZero(t, e.Count)
					if i > 0 {
						assert.Less(t, s.Entries()[i-1].Key, e.Key)
					}
				}
			}
		}
	}
}

func TestCountStoreCanonical(t *testing.T) {
	x := NewCountStore[string](CountCapped)
	for _, k := range []string{"a", "b", "a", "a"} {
		x.Add(k, k)
	}
	y := NewCountStore[string](CountCapped)
	for _, k := range []string{"b", "a", "a"} {
		y.Add(k, k)
	}
	assert.True(t, x.Equal(y), "MANY == MANY")
	assert.Equal(t, fmt.Sprint(x.Entries()), fmt.Sprint(y.Entries()))

	// Exact stores tell 3 from 2; a capped store and an exact one differ.
	e3 := NewCountStore[string](CountExact)
	e2 := NewCountStore[string](CountExact)
	for i := 0; i < 3; i++ {
		e3.Add("a", "a")
	}
	e2.Add("a", "a")
	e2.Add("a", "a")
	assert.False(t, e3.Equal(e2))
	assert.False(t, x.Equal(e3))

	// Removing the last copy is the same state as never adding it.
	z := NewCountStore[string](CountCapped)
	z.Add("a", "a")
	z.Add("q", "q")
	require.NoError(t, z.Remove("q", 0))
	only := NewCountStore[string](CountCapped)
	only.Add("a", "a")
	assert.True(t, z.Equal(only))
}

func TestCountStoreOutcomesAndErrors(t *testing.T) {
	s := NewCountStore[int](CountCapped)
	_, err := s.RemoveOutcomes("a")
	assert.ErrorContains(t, err, "not present")
	s.Add("a", 1)
	outs, _ := s.RemoveOutcomes("a")
	assert.Equal(t, []int{0}, outs)
	s.Add("a", 1)
	outs, _ = s.RemoveOutcomes("a")
	assert.Equal(t, []int{1, Many}, outs)
	assert.Error(t, s.Remove("a", 0), "0 is not an outcome from Many")
	assert.Error(t, s.SetCount("zz", 1), "no value for an absent key")
	assert.Error(t, s.SetCount("a", 3), "capped store holds at most Many")

	ex := NewCountStore[int](CountExact)
	for i := 0; i < 5; i++ {
		ex.Add("a", 1)
	}
	outs, _ = ex.RemoveOutcomes("a")
	assert.Equal(t, []int{4}, outs)
}

func TestCountStoreCloneAndRekey(t *testing.T) {
	s := NewCountStore[[]int](CountExact)
	s.Add("x1", []int{1})
	s.Add("x2", []int{2})
	s.Add("x2", []int{2})
	c := s.Clone(func(v []int) []int { return append([]int(nil), v...) })
	c.Entries()[0].Value[0] = 99
	assert.Equal(t, 1, s.Entries()[0].Value[0], "clone copies values")

	// A permutation that maps both values to the same key merges them.
	c.Rekey(func(v []int) string { return "same" })
	assert.Equal(t, 1, c.Len())
	assert.Equal(t, 3, c.Count("same"))

	capped := NewCountStore[int](CountCapped)
	capped.Add("p", 1)
	capped.Add("q", 2)
	capped.Add("q", 2)
	capped.Rekey(func(int) string { return "r" })
	assert.Equal(t, Many, capped.Count("r"))
}
