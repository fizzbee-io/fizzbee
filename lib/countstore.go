package lib

// CountStore is an unordered collection that keeps, for each distinct
// element, how many copies there are. It is plain Go, with no Starlark, so
// the checker can use it for its own state (such as identical threads of a
// process) as well as any spec-visible type built on top of it.
//
// Elements are identified by a caller-supplied canonical key. Entries are
// kept sorted by key and a zero count is never stored, so two stores with
// the same counts per key have the same entries in the same order, however
// they were built. That makes hashing and encoding canonical by
// construction.
//
// The count policy is fixed per store:
//   - CountExact keeps exact counts. Remove has one outcome.
//   - CountCapped keeps 0, 1 or Many. Adding to Many stays Many, and
//     removing from Many has two outcomes, 1 or Many; the caller picks one
//     (with a oneof, or by forking).

import (
	"fmt"
	"sort"
)

// CountPolicy is how a CountStore counts copies.
type CountPolicy uint8

const (
	CountExact CountPolicy = iota
	CountCapped
)

// Many is the count a CountCapped store uses for "more than one".
const Many = 2

// CountEntry is one distinct element and its count, which is never 0.
type CountEntry[T any] struct {
	Key   string
	Value T
	Count int
}

// CountStore holds the entries, sorted by key.
type CountStore[T any] struct {
	policy  CountPolicy
	entries []CountEntry[T]
}

// NewCountStore returns an empty store with the given policy.
func NewCountStore[T any](policy CountPolicy) *CountStore[T] {
	return &CountStore[T]{policy: policy}
}

func (s *CountStore[T]) Policy() CountPolicy { return s.policy }

func (s *CountStore[T]) find(key string) (int, bool) {
	i := sort.Search(len(s.entries), func(i int) bool { return s.entries[i].Key >= key })
	return i, i < len(s.entries) && s.entries[i].Key == key
}

// Count returns key's count, 0 when absent.
func (s *CountStore[T]) Count(key string) int {
	if i, ok := s.find(key); ok {
		return s.entries[i].Count
	}
	return 0
}

// Get returns key's value and whether it is present.
func (s *CountStore[T]) Get(key string) (T, bool) {
	if i, ok := s.find(key); ok {
		return s.entries[i].Value, true
	}
	var zero T
	return zero, false
}

// Has reports whether key has at least one copy.
func (s *CountStore[T]) Has(key string) bool {
	_, ok := s.find(key)
	return ok
}

// Len is the number of distinct elements.
func (s *CountStore[T]) Len() int { return len(s.entries) }

// Total is the number of copies: exact, or capped at Many.
func (s *CountStore[T]) Total() int {
	n := 0
	for _, e := range s.entries {
		n += e.Count
	}
	if s.policy == CountCapped && n > Many {
		return Many
	}
	return n
}

// Entries returns the entries in key order. The slice is the store's own;
// callers must not change it.
func (s *CountStore[T]) Entries() []CountEntry[T] { return s.entries }

// Add adds one copy of value under key. When key is already present its
// stored value is kept: equal keys must mean equal values.
func (s *CountStore[T]) Add(key string, value T) {
	i, ok := s.find(key)
	if ok {
		s.entries[i].Count = s.inc(s.entries[i].Count)
		return
	}
	s.entries = append(s.entries, CountEntry[T]{})
	copy(s.entries[i+1:], s.entries[i:])
	s.entries[i] = CountEntry[T]{Key: key, Value: value, Count: 1}
}

func (s *CountStore[T]) inc(n int) int {
	if s.policy == CountCapped && n >= Many {
		return Many
	}
	return n + 1
}

// RemoveOutcomes returns the counts key can have after one copy is removed:
// one outcome for an exact store, and for a capped store {0} from 1 and
// {1, Many} from Many. It is an error when key is absent.
func (s *CountStore[T]) RemoveOutcomes(key string) ([]int, error) {
	n := s.Count(key)
	switch {
	case n == 0:
		return nil, fmt.Errorf("remove(%s): not present", key)
	case s.policy == CountCapped && n == Many:
		return []int{1, Many}, nil
	default:
		return []int{n - 1}, nil
	}
}

// Remove removes one copy of key, taking outcome, which must be one of
// RemoveOutcomes(key).
func (s *CountStore[T]) Remove(key string, outcome int) error {
	outs, err := s.RemoveOutcomes(key)
	if err != nil {
		return err
	}
	for _, o := range outs {
		if o == outcome {
			return s.SetCount(key, outcome)
		}
	}
	return fmt.Errorf("remove(%s): %d is not an outcome of removing from %d", key, outcome, s.Count(key))
}

// SetCount sets key's count; 0 removes the entry. key must be present
// unless n is 0, since the store has no value for it otherwise.
func (s *CountStore[T]) SetCount(key string, n int) error {
	if n < 0 || (s.policy == CountCapped && n > Many) {
		return fmt.Errorf("count %d out of range", n)
	}
	i, ok := s.find(key)
	switch {
	case n == 0:
		if ok {
			s.entries = append(s.entries[:i], s.entries[i+1:]...)
		}
	case ok:
		s.entries[i].Count = n
	default:
		return fmt.Errorf("set count of %s: not present", key)
	}
	return nil
}

// Equal reports whether both stores have the same policy and the same
// count per key. Values are not compared: equal keys mean equal values.
func (s *CountStore[T]) Equal(o *CountStore[T]) bool {
	if s.policy != o.policy || len(s.entries) != len(o.entries) {
		return false
	}
	for i := range s.entries {
		if s.entries[i].Key != o.entries[i].Key || s.entries[i].Count != o.entries[i].Count {
			return false
		}
	}
	return true
}

// Clone returns a copy, with each value passed through clone (nil keeps
// values as they are).
func (s *CountStore[T]) Clone(clone func(T) T) *CountStore[T] {
	out := &CountStore[T]{policy: s.policy, entries: make([]CountEntry[T], len(s.entries))}
	copy(out.entries, s.entries)
	if clone != nil {
		for i := range out.entries {
			out.entries[i].Value = clone(out.entries[i].Value)
		}
	}
	return out
}

// Rekey recomputes every key with key and restores the order, merging
// entries that now share a key. Use it after a change that can change keys,
// such as a symmetry permutation of the values.
func (s *CountStore[T]) Rekey(key func(T) string) {
	old := s.entries
	s.entries = make([]CountEntry[T], 0, len(old))
	for _, e := range old {
		k := key(e.Value)
		i, ok := s.find(k)
		if ok {
			n := s.entries[i].Count + e.Count
			if s.policy == CountCapped && n > Many {
				n = Many
			}
			s.entries[i].Count = n
			continue
		}
		s.entries = append(s.entries, CountEntry[T]{})
		copy(s.entries[i+1:], s.entries[i:])
		s.entries[i] = CountEntry[T]{Key: k, Value: e.Value, Count: e.Count}
	}
}
