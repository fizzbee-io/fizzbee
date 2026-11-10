package lib

// CappedCountStore is an unordered collection that keeps, for each distinct
// element, how many copies there are as a CardinalityCounter: 1 or MANY
// (an absent element is ZERO). It is plain Go, with no Starlark, so the
// checker can use it for its own state, such as identical threads of a
// process, as well as any spec-visible type built on top of it.
//
// Elements are identified by a caller-supplied canonical key. Entries are
// kept sorted by key and a ZERO count is never stored, so two stores with
// the same counts per key have the same entries in the same order, however
// they were built. That makes hashing and encoding canonical by
// construction.
//
// Adding to MANY stays MANY. Removing from MANY has two outcomes, ONE or
// MANY, as for the counter's dec(); the store never picks one itself. The
// caller asks for RemoveOutcomes and takes one with Remove, typically once
// in each fork.

import (
	"fmt"
	"sort"
)

// CountEntry is one distinct element and its count, which is never ZERO.
type CountEntry[T any] struct {
	Key   string
	Value T
	Count *CardinalityCounter
}

// CappedCountStore holds the entries, sorted by key.
type CappedCountStore[T any] struct {
	entries []CountEntry[T]
}

// NewCappedCountStore returns an empty store.
func NewCappedCountStore[T any]() *CappedCountStore[T] {
	return &CappedCountStore[T]{}
}

var zeroCount = NewCardinalityCounter(0)

func (s *CappedCountStore[T]) find(key string) (int, bool) {
	i := sort.Search(len(s.entries), func(i int) bool { return s.entries[i].Key >= key })
	return i, i < len(s.entries) && s.entries[i].Key == key
}

// Count returns key's count, ZERO when absent.
func (s *CappedCountStore[T]) Count(key string) *CardinalityCounter {
	if i, ok := s.find(key); ok {
		return s.entries[i].Count
	}
	return zeroCount
}

// Get returns key's value and whether it is present.
func (s *CappedCountStore[T]) Get(key string) (T, bool) {
	if i, ok := s.find(key); ok {
		return s.entries[i].Value, true
	}
	var zero T
	return zero, false
}

// Has reports whether key has at least one copy.
func (s *CappedCountStore[T]) Has(key string) bool {
	_, ok := s.find(key)
	return ok
}

// Len is the number of distinct elements.
func (s *CappedCountStore[T]) Len() int { return len(s.entries) }

// Total is the number of copies: ZERO, ONE or MANY.
func (s *CappedCountStore[T]) Total() *CardinalityCounter {
	switch {
	case len(s.entries) == 0:
		return zeroCount
	case len(s.entries) == 1:
		return s.entries[0].Count
	default:
		return NewCardinalityCounter(2)
	}
}

// Entries returns the entries in key order. The slice is the store's own;
// callers must not change it.
func (s *CappedCountStore[T]) Entries() []CountEntry[T] { return s.entries }

// Add adds one copy of value under key. When key is already present its
// stored value is kept: equal keys must mean equal values.
func (s *CappedCountStore[T]) Add(key string, value T) {
	i, ok := s.find(key)
	if ok {
		s.entries[i].Count = s.entries[i].Count.Inc()
		return
	}
	s.entries = append(s.entries, CountEntry[T]{})
	copy(s.entries[i+1:], s.entries[i:])
	s.entries[i] = CountEntry[T]{Key: key, Value: value, Count: zeroCount.Inc()}
}

// RemoveOutcomes returns the counts key can have after one copy is removed:
// ZERO from ONE, and ONE or MANY from MANY. It is an error when key is
// absent.
func (s *CappedCountStore[T]) RemoveOutcomes(key string) ([]*CardinalityCounter, error) {
	i, ok := s.find(key)
	if !ok {
		return nil, fmt.Errorf("remove(%s): not present", key)
	}
	return s.entries[i].Count.Dec()
}

// Remove removes one copy of key, leaving outcome, which must be one of
// RemoveOutcomes(key).
func (s *CappedCountStore[T]) Remove(key string, outcome *CardinalityCounter) error {
	outs, err := s.RemoveOutcomes(key)
	if err != nil {
		return err
	}
	for _, o := range outs {
		if o.ValueCode() == outcome.ValueCode() {
			s.setCount(key, outcome)
			return nil
		}
	}
	return fmt.Errorf("remove(%s): %s is not an outcome of removing from %s", key, outcome, s.Count(key))
}

// setCount sets a present key's count; ZERO removes the entry.
func (s *CappedCountStore[T]) setCount(key string, c *CardinalityCounter) {
	i, ok := s.find(key)
	if !ok {
		return
	}
	if c.IsZero() {
		s.entries = append(s.entries[:i], s.entries[i+1:]...)
		return
	}
	s.entries[i].Count = c
}

// Equal reports whether both stores have the same count per key. Values
// are not compared: equal keys mean equal values.
func (s *CappedCountStore[T]) Equal(o *CappedCountStore[T]) bool {
	if len(s.entries) != len(o.entries) {
		return false
	}
	for i := range s.entries {
		if s.entries[i].Key != o.entries[i].Key || s.entries[i].Count.ValueCode() != o.entries[i].Count.ValueCode() {
			return false
		}
	}
	return true
}

// Clone returns a copy, with each value passed through clone (nil keeps
// values as they are). Counters are immutable and are shared.
func (s *CappedCountStore[T]) Clone(clone func(T) T) *CappedCountStore[T] {
	out := &CappedCountStore[T]{entries: make([]CountEntry[T], len(s.entries))}
	copy(out.entries, s.entries)
	if clone != nil {
		for i := range out.entries {
			out.entries[i].Value = clone(out.entries[i].Value)
		}
	}
	return out
}

// Rekey recomputes every key with key and restores the order. Entries that
// now share a key merge, and their counts add up to MANY. Use it after a
// change that can change keys, such as a symmetry permutation of the
// values.
func (s *CappedCountStore[T]) Rekey(key func(T) string) {
	old := s.entries
	s.entries = make([]CountEntry[T], 0, len(old))
	for _, e := range old {
		k := key(e.Value)
		i, ok := s.find(k)
		if ok {
			s.entries[i].Count = NewCardinalityCounter(2)
			continue
		}
		s.entries = append(s.entries, CountEntry[T]{})
		copy(s.entries[i+1:], s.entries[i:])
		s.entries[i] = CountEntry[T]{Key: k, Value: e.Value, Count: e.Count}
	}
}
