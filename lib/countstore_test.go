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

func TestCappedCountStoreAPIContracts(t *testing.T) {
	s := NewCappedCountStore[string]()
	s.Add("a", "a")
	s.Add("a", "a")
	s.Add("b", "b")

	// A nil outcome is an error, not a panic.
	assert.ErrorContains(t, s.Remove("a", nil), "nil outcome")

	// Changing the slice Entries returns does not change the store.
	es := s.Entries()
	es[0].Key = "zzz"
	es[0].Count = NewCardinalityCounter(0)
	assert.Equal(t, "MANY", s.Count("a").String())
	assert.Equal(t, 2, s.Len())
	assert.Equal(t, "a", s.Entries()[0].Key)

	// The key is the identity: Add under an existing key keeps the first
	// value and counts the copy.
	s.Add("b", "other")
	v, _ := s.Get("b")
	assert.Equal(t, "b", v)
	assert.Equal(t, "MANY", s.Count("b").String())

	// A clone is independent of later changes to either store.
	c := s.Clone(nil)
	require.NoError(t, c.Remove("a", NewCardinalityCounter(1)))
	c.Add("c", "c")
	assert.Equal(t, "MANY", s.Count("a").String())
	assert.False(t, s.Has("c"))

	// The empty store.
	e := NewCappedCountStore[string]()
	assert.Equal(t, 0, e.Len())
	assert.Equal(t, "0", e.Total().String())
	assert.Equal(t, "0", e.Count("a").String())
	assert.Empty(t, e.Entries())
	assert.True(t, e.Equal(NewCappedCountStore[string]()))
}

func TestCappedCountStoreRekeyMerges(t *testing.T) {
	mk := func() *CappedCountStore[int] {
		s := NewCappedCountStore[int]()
		for _, v := range []int{1, 1, 2, 2, 3, 4} { // 1: MANY, 2: MANY, 3: ONE, 4: ONE
			s.Add(fmt.Sprint(v), v)
		}
		return s
	}
	parity := func(v int) string { return fmt.Sprint(v % 2) }

	// MANY + MANY, and ONE + ONE, are MANY; ONE alone stays ONE.
	s := mk()
	s.Rekey(parity)
	assert.Equal(t, "[0:MANY 1:MANY]", entriesString(s))
	s = mk()
	s.Rekey(func(v int) string {
		if v == 3 {
			return "odd"
		}
		return "rest"
	})
	assert.Equal(t, "[odd:1 rest:MANY]", entriesString(s))

	// Idempotent.
	s = mk()
	s.Rekey(parity)
	before := entriesString(s)
	s.Rekey(parity)
	assert.Equal(t, before, entriesString(s))

	// A panicking key leaves the store unchanged.
	s = mk()
	before = entriesString(s)
	assert.Panics(t, func() {
		s.Rekey(func(v int) string {
			if v == 3 {
				panic("boom")
			}
			return "x"
		})
	})
	assert.Equal(t, before, entriesString(s))
}

func entriesString[T any](s *CappedCountStore[T]) string {
	var parts []string
	for _, e := range s.Entries() {
		parts = append(parts, e.Key+":"+e.Count.String())
	}
	return fmt.Sprint(parts)
}

func TestCappedCountStoreAddCount(t *testing.T) {
	s := NewCappedCountStore[string]()
	s.AddCount("a", "a", NewCardinalityCounter(0))
	assert.False(t, s.Has("a"), "adding ZERO stores nothing")
	s.AddCount("a", "a", NewCardinalityCounter(2))
	assert.Equal(t, "MANY", s.Count("a").String())
	s.AddCount("b", "b", NewCardinalityCounter(1))
	s.AddCount("b", "other", NewCardinalityCounter(1))
	assert.Equal(t, "MANY", s.Count("b").String(), "ONE + ONE is MANY")
	v, _ := s.Get("b")
	assert.Equal(t, "b", v, "the first value is kept")

	// AddCount(ONE) is Add, and AddCount(MANY) is two Adds.
	x, y := NewCappedCountStore[int](), NewCappedCountStore[int]()
	x.AddCount("k", 1, NewCardinalityCounter(2))
	y.Add("k", 1)
	y.Add("k", 1)
	assert.True(t, x.Equal(y))
	assert.Equal(t, "[a:MANY b:MANY]", entriesString(s))
}
