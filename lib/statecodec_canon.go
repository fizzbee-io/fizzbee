package lib

import (
	"bytes"
	"reflect"
	"sort"
	"unsafe"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// canonicalOrder returns the order in which to write the elements of an
// unordered collection (dict and genericmap keys, set, genericset and bag
// elements): keys[order[i]] is the i-th to write. It is nil when keys is
// already in that order.
//
// The order depends only on the keys' values, not on insertion order, so
// two states the hash treats as equal (StateEncoder sorts the same
// collections) also encode alike. Strings sort as strings and int64s as
// numbers; any other mix sorts by the keys' StateEncoder bytes. Keys whose
// canonical bytes tie (equal but distinct values in a bag, say) keep their
// relative order.
func canonicalOrder(keys []starlark.Value) []int {
	n := len(keys)
	if n < 2 {
		return nil
	}
	allStr, allInt := true, true
	for _, k := range keys {
		switch x := k.(type) {
		case starlark.String:
			allInt = false
		case starlark.Int:
			allStr = false
			if _, ok := x.Int64(); !ok {
				allInt = false
			}
		default:
			allStr, allInt = false, false
		}
		if !allStr && !allInt {
			break
		}
	}
	var less func(i, j int) bool
	switch {
	case allStr:
		less = func(i, j int) bool { return keys[i].(starlark.String) < keys[j].(starlark.String) }
	case allInt:
		less = func(i, j int) bool {
			a, _ := keys[i].(starlark.Int).Int64()
			b, _ := keys[j].(starlark.Int).Int64()
			return a < b
		}
	default:
		enc := make([][]byte, n)
		e := NewStateEncoder()
		for i, k := range keys {
			e.Truncate(0)
			e.WriteValue(k, 0)
			enc[i] = append([]byte(nil), e.Bytes()...)
		}
		e.Release()
		less = func(i, j int) bool { return bytes.Compare(enc[i], enc[j]) < 0 }
	}
	sorted := true
	for i := 1; i < n; i++ {
		if less(i, i-1) {
			sorted = false
			break
		}
	}
	if sorted {
		return nil
	}
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return less(order[a], order[b]) })
	return order
}

// structLayout mirrors starlarkstruct.Struct, whose fields are unexported.
// The reader uses it to number a struct before reading its fields, as it
// does for every other container, so cycles through a struct keep their
// identity. structLayoutOK checks the mirror against the real type; if a
// starlark upgrade changes it, writing a struct fails with an error, and
// the codec tests catch it.
type structLayout struct {
	constructor starlark.Value
	entries     []structEntry
	frozen      bool
}

type structEntry struct {
	name  string
	value starlark.Value
}

var structLayoutOK = func() bool {
	real, mine := reflect.TypeOf(starlarkstruct.Struct{}), reflect.TypeOf(structLayout{})
	if real.Size() != mine.Size() || real.NumField() != mine.NumField() {
		return false
	}
	for i := 0; i < real.NumField(); i++ {
		rf, mf := real.Field(i), mine.Field(i)
		if rf.Name != mf.Name || rf.Offset != mf.Offset || rf.Type.Kind() != mf.Type.Kind() {
			return false
		}
	}
	re, me := real.Field(1).Type.Elem(), mine.Field(1).Type.Elem()
	if re.Size() != me.Size() || re.NumField() != me.NumField() {
		return false
	}
	for i := 0; i < re.NumField(); i++ {
		if re.Field(i).Name != me.Field(i).Name || re.Field(i).Type != me.Field(i).Type || re.Field(i).Offset != me.Field(i).Offset {
			return false
		}
	}
	return real.Field(0).Type == mine.Field(0).Type
}()

// structEntries returns s's entries, sorted by name, to fill in place.
func structEntries(s *starlarkstruct.Struct) []structEntry {
	return (*structLayout)(unsafe.Pointer(s)).entries
}
