package lib

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// abstract is the counter for an exact count n.
func abstract(n int) string { return NewCardinalityCounter(int64(n)).String() }

// Random adds and removes against exact counts. The store matches their
// abstraction, when each removal from MANY offers and takes the outcome
// that matches the concrete count. Entries stay sorted with no ZERO.
func TestCappedCountStoreMatchesConcreteCounts(t *testing.T) {
	keys := []string{"a", "b", "c", "d"}
	r := rand.New(rand.NewSource(1))
	for run := 0; run < 200; run++ {
		s := NewCappedCountStore[int]()
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
				var picked *CardinalityCounter
				for _, o := range outs {
					if o.String() == abstract(counts[k]) {
						picked = o
					}
				}
				require.NotNil(t, picked, "removing %s offers no %s", k, abstract(counts[k]))
				require.NoError(t, s.Remove(k, picked))
			}
			total, distinct := 0, 0
			for _, k := range keys {
				assert.Equal(t, abstract(counts[k]), s.Count(k).String(), "%s", k)
				assert.Equal(t, counts[k] > 0, s.Has(k))
				total += counts[k]
				if counts[k] > 0 {
					distinct++
				}
			}
			assert.Equal(t, distinct, s.Len())
			assert.Equal(t, abstract(total), s.Total().String())
			for i, e := range s.Entries() {
				assert.False(t, e.Count.IsZero())
				if i > 0 {
					assert.Less(t, s.Entries()[i-1].Key, e.Key)
				}
			}
		}
	}
}

func TestCappedCountStoreCanonical(t *testing.T) {
	x := NewCappedCountStore[string]()
	for _, k := range []string{"a", "b", "a", "a"} {
		x.Add(k, k)
	}
	y := NewCappedCountStore[string]()
	for _, k := range []string{"b", "a", "a"} {
		y.Add(k, k)
	}
	assert.True(t, x.Equal(y), "MANY == MANY")
	assert.Equal(t, fmt.Sprint(x.Entries()), fmt.Sprint(y.Entries()))

	one := NewCappedCountStore[string]()
	one.Add("a", "a")
	one.Add("b", "b")
	assert.False(t, x.Equal(one), "MANY != ONE")

	// Removing the last copy is the same state as never adding it.
	z := NewCappedCountStore[string]()
	z.Add("a", "a")
	z.Add("q", "q")
	require.NoError(t, z.Remove("q", NewCardinalityCounter(0)))
	only := NewCappedCountStore[string]()
	only.Add("a", "a")
	assert.True(t, z.Equal(only))
}

func TestCappedCountStoreOutcomesAndErrors(t *testing.T) {
	s := NewCappedCountStore[int]()
	_, err := s.RemoveOutcomes("a")
	assert.ErrorContains(t, err, "not present")
	assert.Error(t, s.Remove("a", NewCardinalityCounter(0)))

	s.Add("a", 1)
	outs, _ := s.RemoveOutcomes("a")
	assert.Equal(t, "[0]", fmt.Sprint(outs))
	s.Add("a", 1)
	outs, _ = s.RemoveOutcomes("a")
	assert.Equal(t, "[1 MANY]", fmt.Sprint(outs))
	assert.Error(t, s.Remove("a", NewCardinalityCounter(0)), "ZERO is not an outcome from MANY")
	assert.Equal(t, "MANY", s.Count("a").String())
}

func TestCappedCountStoreCloneAndRekey(t *testing.T) {
	s := NewCappedCountStore[[]int]()
	s.Add("x1", []int{1})
	s.Add("x2", []int{2})
	c := s.Clone(func(v []int) []int { return append([]int(nil), v...) })
	c.Entries()[0].Value[0] = 99
	assert.Equal(t, 1, s.Entries()[0].Value[0], "clone copies values")
	assert.True(t, s.Equal(c))

	// A permutation that maps both values to the same key merges them.
	c.Rekey(func(v []int) string { return "same" })
	assert.Equal(t, 1, c.Len())
	assert.Equal(t, "MANY", c.Count("same").String())

	// A permutation that swaps keys restores the order.
	s.Rekey(func(v []int) string { return fmt.Sprintf("y%d", 3-v[0]) })
	assert.Equal(t, "y1", s.Entries()[0].Key)
	assert.Equal(t, 2, s.Entries()[0].Value[0])
}
