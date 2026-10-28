package lib

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"sync"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// StateEncoder writes a canonical binary encoding of starlark values into a
// byte buffer, for hashing model-checker states. It replaces building the
// JSON text of the state (lib.MarshalJSON) and hashing that: the encoding is
// written straight into one reusable buffer, with no per-value strings.
//
// The guarantee is that equal states encode to equal bytes. Every string and
// map key is length-prefixed, so the encoding itself is unambiguous and needs
// no escaping. Which values count as equal is kept close to the JSON text on
// purpose (changing it would change state counts), except where it is
// strictly finer, which never merges states that the JSON text kept apart:
//   - a list/tuple/range and a set/bag with the same elements no longer
//     share an encoding (JSON rendered both as a sorted-or-not array).
//   - a RoleStub no longer shares an encoding with anything else.
//
// Deliberately kept identical to the JSON partition:
//   - list, tuple and range encode alike; set, genericset and bag encode
//     alike; dict, genericmap, struct and record encode alike (string-keyed
//     maps); string and bytes encode alike.
//   - a role in value position encodes as its reference string, a
//     model/symmetric value as its full string, so each still collides with
//     the string of the same text.
//   - dict keys are encoded as their JSON key text, so an int key 1 and a
//     string key "1" still collide.
type StateEncoder struct {
	buf     []byte
	scratch []byte
	// segs collects (start,end) offsets of sibling encodings that must be
	// sorted (set elements, map entries).
	segs []segment
}

type segment struct{ start, end int }

const (
	tagNone    byte = 'N'
	tagBool    byte = 'B'
	tagString  byte = 'S'
	tagInt64   byte = 'I'
	tagBigInt  byte = 'J'
	tagFloat   byte = 'F'
	tagList    byte = 'L'
	tagSet     byte = 'T'
	tagMap     byte = 'M'
	tagStub    byte = 'U'
	tagChannel byte = 'C'
	tagSegment byte = 'G'
	tagOther   byte = 'X'
)

var encoderPool = sync.Pool{New: func() any { return &StateEncoder{buf: make([]byte, 0, 4096)} }}

// NewStateEncoder returns an empty encoder from the pool. Call Release when
// done with Bytes().
func NewStateEncoder() *StateEncoder {
	e := encoderPool.Get().(*StateEncoder)
	e.buf = e.buf[:0]
	e.segs = e.segs[:0]
	return e
}

func (e *StateEncoder) Release() {
	if cap(e.buf) > 1<<20 {
		// Don't keep huge buffers alive in the pool.
		e.buf = make([]byte, 0, 4096)
	}
	encoderPool.Put(e)
}

// Bytes returns the encoding written so far. It is only valid until the
// next write or Release.
func (e *StateEncoder) Bytes() []byte { return e.buf }

func (e *StateEncoder) Len() int { return len(e.buf) }

// Truncate drops everything written after offset n.
func (e *StateEncoder) Truncate(n int) { e.buf = e.buf[:n] }

func (e *StateEncoder) WriteByte(b byte) { e.buf = append(e.buf, b) }

func (e *StateEncoder) WriteUvarint(n uint64) {
	e.buf = binary.AppendUvarint(e.buf, n)
}

func (e *StateEncoder) WriteInt64(n int64) {
	e.buf = binary.LittleEndian.AppendUint64(e.buf, uint64(n))
}

// WriteString writes a length-prefixed string (no tag).
func (e *StateEncoder) WriteString(s string) {
	e.buf = binary.AppendUvarint(e.buf, uint64(len(s)))
	e.buf = append(e.buf, s...)
}

// WriteBytes writes a length-prefixed byte string (no tag).
func (e *StateEncoder) WriteBytes(b []byte) {
	e.buf = binary.AppendUvarint(e.buf, uint64(len(b)))
	e.buf = append(e.buf, b...)
}

// WriteTaggedString writes a string value.
func (e *StateEncoder) WriteTaggedString(s string) {
	e.buf = append(e.buf, tagString)
	e.WriteString(s)
}

// WriteStringDict writes a starlark.StringDict as a string-keyed map,
// entries sorted by key, the same partition as a dict with string keys.
func (e *StateEncoder) WriteStringDict(d starlark.StringDict) {
	e.buf = append(e.buf, tagMap)
	e.buf = binary.AppendUvarint(e.buf, uint64(len(d)))
	e.sortedEntries(len(d), func(emit func()) {
		for k, v := range d {
			e.writeMapKeyText(k)
			e.WriteValue(v, 1)
			emit()
		}
	})
}

// writeMapKeyText writes a string key the way the JSON text spelled it: a
// quoted identifier. Dict keys go through writeDictKey instead.
func (e *StateEncoder) writeMapKeyText(k string) {
	e.buf = binary.AppendUvarint(e.buf, uint64(len(k)+2))
	e.buf = append(e.buf, '"')
	e.buf = append(e.buf, k...)
	e.buf = append(e.buf, '"')
}

// sortedEntries runs fill, which writes n sibling encodings and calls emit
// after each one, then reorders those siblings into canonical (byte) order
// so the result does not depend on iteration order.
func (e *StateEncoder) sortedEntries(n int, fill func(emit func())) {
	if n == 0 {
		return
	}
	segBase := len(e.segs)
	regionStart := len(e.buf)
	start := regionStart
	fill(func() {
		e.segs = append(e.segs, segment{start, len(e.buf)})
		start = len(e.buf)
	})
	segs := e.segs[segBase:]
	if len(segs) > 1 {
		buf := e.buf
		sorted := true
		for i := 1; i < len(segs); i++ {
			if bytes.Compare(buf[segs[i-1].start:segs[i-1].end], buf[segs[i].start:segs[i].end]) > 0 {
				sorted = false
				break
			}
		}
		if !sorted {
			sort.Slice(segs, func(i, j int) bool {
				return bytes.Compare(buf[segs[i].start:segs[i].end], buf[segs[j].start:segs[j].end]) < 0
			})
			e.scratch = e.scratch[:0]
			for _, s := range segs {
				e.scratch = append(e.scratch, buf[s.start:s.end]...)
			}
			copy(buf[regionStart:], e.scratch)
		}
	}
	e.segs = e.segs[:segBase]
}

// AppendRaw appends already-encoded bytes (for example an earlier encoding
// of the same map, memoized by the caller).
func (e *StateEncoder) AppendRaw(b []byte) { e.buf = append(e.buf, b...) }

// WriteValue writes one starlark value. depth guards against cycles the same
// way MarshalJSONStarlarkValue does.
func (e *StateEncoder) WriteValue(v starlark.Value, depth int) {
	if depth > 20 {
		e.WriteTaggedString("TOO_DEEP")
		return
	}
	switch x := v.(type) {
	case starlark.NoneType:
		e.buf = append(e.buf, tagNone)
	case starlark.Bool:
		e.buf = append(e.buf, tagBool)
		if x {
			e.buf = append(e.buf, 1)
		} else {
			e.buf = append(e.buf, 0)
		}
	case starlark.String:
		e.WriteTaggedString(string(x))
	case starlark.Bytes:
		e.WriteTaggedString(string(x))
	case starlark.Int:
		if n, ok := x.Int64(); ok {
			e.buf = append(e.buf, tagInt64)
			e.WriteInt64(n)
		} else {
			e.buf = append(e.buf, tagBigInt)
			e.WriteString(x.String())
		}
	case starlark.Float:
		e.buf = append(e.buf, tagFloat)
		e.WriteString(x.String())
	case *starlark.List:
		e.buf = append(e.buf, tagList)
		e.buf = binary.AppendUvarint(e.buf, uint64(x.Len()))
		for i := 0; i < x.Len(); i++ {
			e.WriteValue(x.Index(i), depth+1)
		}
	case starlark.Tuple:
		e.buf = append(e.buf, tagList)
		e.buf = binary.AppendUvarint(e.buf, uint64(len(x)))
		for _, el := range x {
			e.WriteValue(el, depth+1)
		}
	case *Role:
		// Same as the JSON text: a role in value position is just its
		// reference; the role's own state is hashed with the process roles.
		e.WriteTaggedString(x.RefString())
	case *SymmetricValue:
		e.WriteTaggedString(x.FullString())
	case *ModelValue:
		e.WriteTaggedString(x.FullString())
	case *Struct:
		e.buf = append(e.buf, tagMap)
		e.buf = binary.AppendUvarint(e.buf, uint64(len(x.entries)))
		e.sortedEntries(len(x.entries), func(emit func()) {
			for _, en := range x.entries {
				e.writeMapKeyText(en.name)
				e.WriteValue(en.value, depth+1)
				emit()
			}
		})
	case *RoleStub:
		e.buf = append(e.buf, tagStub)
		e.WriteString(x.Role.RefStringShort())
		e.WriteString(x.Channel.RefStringShort())
	case *Channel:
		e.buf = append(e.buf, tagChannel)
		e.WriteInt64(int64(x.Id))
		e.WriteString(x.Name)
	case *Segment:
		e.buf = append(e.buf, tagSegment)
		e.WriteString(x.Domain.Name)
		e.WriteInt64(int64(x.Left))
		e.WriteInt64(int64(x.Right))
		e.buf = append(e.buf, boolByte(x.IsHead), boolByte(x.IsTail))
	case *starlarkstruct.Struct:
		names := x.AttrNames()
		e.buf = append(e.buf, tagMap)
		e.buf = binary.AppendUvarint(e.buf, uint64(len(names)))
		e.sortedEntries(len(names), func(emit func()) {
			for _, name := range names {
				e.writeMapKeyText(name)
				val, err := x.Attr(name)
				if err != nil {
					panic(err)
				}
				e.WriteValue(val, depth+1)
				emit()
			}
		})
	default:
		e.writeByType(v, depth)
	}
}

func boolByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}

// writeByType handles the value kinds that are identified by Type() rather
// than by a concrete Go type (ranges, the generic collections, dicts).
func (e *StateEncoder) writeByType(v starlark.Value, depth int) {
	switch v.Type() {
	case "range", "list", "tuple":
		elems := collect(v)
		e.buf = append(e.buf, tagList)
		e.buf = binary.AppendUvarint(e.buf, uint64(len(elems)))
		for _, el := range elems {
			e.WriteValue(el, depth+1)
		}
	case "set", "genericset", "bag":
		elems := collect(v)
		e.buf = append(e.buf, tagSet)
		e.buf = binary.AppendUvarint(e.buf, uint64(len(elems)))
		e.sortedEntries(len(elems), func(emit func()) {
			for _, el := range elems {
				e.WriteValue(el, depth+1)
				emit()
			}
		})
	case "dict", "genericmap":
		items := v.(starlark.IterableMapping).Items()
		e.buf = append(e.buf, tagMap)
		e.buf = binary.AppendUvarint(e.buf, uint64(len(items)))
		e.sortedEntries(len(items), func(emit func()) {
			for _, item := range items {
				e.writeDictKey(item[0])
				e.WriteValue(item[1], depth+1)
				emit()
			}
		})
	case "struct", "record", "role", "RoleStub", "model_value", "symmetric_value", "Channel", "symmetry_segment",
		"NoneType", "bool", "string", "bytes", "int", "float":
		// Concrete-type cases above cover these; reaching here means a
		// different implementation of the same type name. Fall back to the
		// JSON text so the partition stays the same as before.
		b, err := MarshalJSONStarlarkValue(v, depth)
		if err != nil {
			panic(err)
		}
		e.buf = append(e.buf, tagOther)
		e.WriteBytes(b)
	default:
		fmt.Println("Warn: unknown type: ", v.Type(), " value: ", v.String(), " using default json.Marshal")
		b, err := json.Marshal(v)
		if err != nil {
			panic(err)
		}
		e.buf = append(e.buf, tagOther)
		e.WriteBytes(b)
	}
}

func collect(v starlark.Value) []starlark.Value {
	iter := v.(starlark.Iterable).Iterate()
	defer iter.Done()
	var x starlark.Value
	elems := make([]starlark.Value, 0, 8)
	for iter.Next(&x) {
		elems = append(elems, x)
	}
	return elems
}

// writeDictKey writes a dict key as the JSON key text the state JSON used,
// so key collisions are unchanged (an int key 1 and a string key "1" both
// spell "1").
func (e *StateEncoder) writeDictKey(k starlark.Value) {
	switch x := k.(type) {
	case starlark.String:
		e.writeQuoted(string(x))
		return
	case starlark.Int:
		// JSON: non-string keys are stringified then quoted; small ints
		// stringify as decimal.
		e.buf = binary.AppendUvarint(e.buf, uint64(len(x.String())+2))
		e.buf = append(e.buf, '"')
		e.buf = append(e.buf, x.String()...)
		e.buf = append(e.buf, '"')
		return
	}
	var kb []byte
	var err error
	if marshaler, ok := k.(json.Marshaler); ok {
		kb, err = marshaler.MarshalJSON()
		if err != nil {
			panic(err)
		}
		if len(kb) > 0 && kb[0] != '"' {
			kb = []byte(strconv.Quote(string(kb)))
		}
	} else {
		if k.Type() != "string" {
			k = starlark.String(k.String())
		}
		kb, err = MarshalJSONStarlarkValue(k, 0)
		if err != nil {
			panic(err)
		}
		if len(kb) > 0 && kb[0] != '"' {
			kb = []byte(strconv.Quote(string(kb)))
		}
	}
	e.WriteBytes(kb)
}

// writeQuoted writes json.Marshal(s) with a length prefix; the fast path
// covers strings that need no escaping.
func (e *StateEncoder) writeQuoted(s string) {
	needsEscape := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 || c == '"' || c == '\\' || c == '<' || c == '>' || c == '&' || c >= 0x80 {
			needsEscape = true
			break
		}
	}
	if !needsEscape {
		e.buf = binary.AppendUvarint(e.buf, uint64(len(s)+2))
		e.buf = append(e.buf, '"')
		e.buf = append(e.buf, s...)
		e.buf = append(e.buf, '"')
		return
	}
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	e.WriteBytes(b)
}

// WriteRole writes a role's full state (name, ref, params, fields), the
// counterpart of Role.MarshalJSON used by the process hash.
func (e *StateEncoder) WriteRole(r *Role) {
	e.WriteString(r.Name)
	e.WriteInt64(r.GetRef())
	e.WriteValue(r.Params, 0)
	e.WriteValue(r.Fields, 0)
}
