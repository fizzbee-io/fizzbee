package lib

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"sync"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// State codec: a compact binary encoding of starlark values that restores
// them exactly, aliasing and cycles included. It is the value layer of the
// model checker's state serialization (modelchecker/statecodec.go writes the
// process around it). See docs/state-serialization.md for the format.
//
// Unlike StateEncoder (statehash.go), which writes a canonical form for
// hashing and can't be read back, this encoding is meant to be decoded:
//   - Every value that has identity (lists, dicts, sets, records, roles,
//     model values, ...) is numbered in the order it is first written. A
//     later occurrence of the same pointer is written as a back-reference
//     to that number, so two references to one list decode to one list,
//     and a cyclic structure terminates. The numbering is the "pool": it
//     is shared by everything written with one StateWriter, which is one
//     whole process state.
//   - Strings and record shapes (constructor + field names) are numbered
//     the same way, in their own tables, so a field name or a repeated
//     string costs one or two bytes after its first use.
//   - What a deep clone shares between states rather than copies
//     (channels, symmetry domains, a role's methods and init values, spec
//     functions) is written by name or by value and goes through a
//     CodecContext, which hands out one shared object per name or value.
//     A state decodes in any process running the same spec.
//
// The writer and reader must agree on when a value is numbered: containers
// are numbered before their elements (so cycles can refer back to them),
// immutable starlarkstruct.Structs after their fields (they can't exist
// before their fields do), and roles after their id (a role's hash depends
// on its id).

// StateCodecVersion is the first byte of every encoded state. Bump it on any
// change that an older reader would misread.
const StateCodecVersion byte = 1

// Value tags. A tag is one byte; tagExt is reserved so tags never run out.
const (
	tagInvalid byte = iota
	vtNone
	vtFalse
	vtTrue
	vtInt      // zigzag varint
	vtBigInt   // sign byte, length-prefixed big-endian magnitude
	vtFloat    // 8 bytes little-endian IEEE 754
	vtString   // string header (see writeStr)
	vtBytes    // length-prefixed raw bytes
	vtTuple    // count, elements
	vtRef      // back-reference: index into the value table
	vtList     // count, elements
	vtDict     // count, key/value pairs in insertion order
	vtSet      // count, elements in insertion order
	vtRecord   // lib.Struct: shape, field values
	vtStruct   // starlarkstruct.Struct: shape, field values
	vtModel    // ModelValue: prefix, id
	vtSymValue // SymmetricValue: prefix, id, kind, limit
	vtRole     // name, symmetric, id, statics, params, fields
	vtRoleStub // role, channel
	vtBag      // count, elements in bag order
	vtGenMap   // count, key/value pairs
	vtGenSet   // count, keys
	vtSegment  // domain, left, right, flags, resolved id
	vtChannel  // id, name, ordering, delivery, blocking
	vtDomain   // name, limit, kind, divergence, start, flags
	vtSymValues
	vtRange  // start, stop, step
	vtOpaque // index into the CodecContext's opaque table
	vtGlobal // name of a spec-level global (a function or builtin)
	tagExt   = 0xFF
)

// CodecContext holds what all states of one model-checking run share:
// interned channels and symmetry domains, what every role of a type shares
// (its methods), interned role init values, and the RoleResolver that
// rebuilds role methods and globals from the spec. Decoding needs a context
// of the same spec, not necessarily the one the state was encoded with:
// everything above is written by name or by value. Decoding returns the
// shared objects rather than fresh copies, as a deep clone shares them.
//
// The one exception is the opaque table, the fallback for a value that has
// neither an encoding nor a name (a lambda kept in the state, say). Its
// entries only mean something inside this process.
//
// It is safe for concurrent use.
type CodecContext struct {
	mu        sync.RWMutex
	opaque    []any
	opaqueIdx map[any]uint64
	channels  map[channelKey]*Channel
	domains   map[SymmetryDomain]*SymmetryDomain
	resolver  RoleResolver
	roleTypes map[string]*roleType
	// initValues interns role init values by role name and encoding.
	initValues map[string]*Struct
}

// RoleResolver rebuilds, from the spec, what a state refers to by name.
type RoleResolver interface {
	// RoleMethods returns the methods a role of this type has once its
	// Init has run, and its RoleMethods builtins.
	RoleMethods(roleName string) (map[string]*starlark.Function, map[string]*starlark.Builtin, error)
	// Global returns the spec-level global of this name.
	Global(name string) (starlark.Value, bool)
}

// roleType is what every role of one type shares. Methods is nil until a
// role with its methods is seen or resolved.
type roleType struct {
	methods     map[string]*starlark.Function
	roleMethods map[string]*starlark.Builtin
}

type channelKey struct {
	id                                 int
	name, ordering, delivery, blocking string
}

func NewCodecContext() *CodecContext {
	return &CodecContext{
		opaqueIdx: make(map[any]uint64),
		channels:  make(map[channelKey]*Channel),
		domains:   make(map[SymmetryDomain]*SymmetryDomain),
		roleTypes: make(map[string]*roleType),

		initValues: make(map[string]*Struct),
	}
}

// SetRoleResolver sets the resolver if none is set yet.
func (c *CodecContext) SetRoleResolver(r RoleResolver) {
	if c.HasRoleResolver() {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.resolver == nil {
		c.resolver = r
	}
}

func (c *CodecContext) HasRoleResolver() bool { return c.getResolver() != nil }

func (c *CodecContext) getResolver() RoleResolver {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.resolver
}

// noteRole records the shared parts of a role being encoded, so decoding
// in this process returns them rather than resolving them again.
func (c *CodecContext) noteRole(r *Role) {
	c.mu.RLock()
	t, ok := c.roleTypes[r.Name]
	known := ok && (t.methods != nil || len(r.Methods) == 0)
	c.mu.RUnlock()
	if known {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	t, ok = c.roleTypes[r.Name]
	if !ok {
		t = &roleType{roleMethods: r.RoleMethods}
		c.roleTypes[r.Name] = t
	}
	if t.methods == nil && len(r.Methods) > 0 {
		t.methods = r.Methods
	}
}

// roleType returns the shared parts of a role type, resolving them from
// the spec if needed. needMethods asks for the methods too.
func (c *CodecContext) roleType(name string, needMethods bool) (*roleType, error) {
	c.mu.RLock()
	t, ok := c.roleTypes[name]
	c.mu.RUnlock()
	if ok && (!needMethods || t.methods != nil) {
		return t, nil
	}
	resolver := c.getResolver()
	if resolver == nil {
		return nil, fmt.Errorf("role %s: no role resolver in the codec context", name)
	}
	methods, roleMethods, err := resolver.RoleMethods(name)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	t, ok = c.roleTypes[name]
	if !ok {
		t = &roleType{roleMethods: roleMethods}
		c.roleTypes[name] = t
	}
	if t.methods == nil {
		t.methods = methods
	}
	return t, nil
}

// internInitValues returns the shared init values of this role type and
// encoding, decoding them on first use. Init values are never changed in
// place once set (see Role.SetInitValue), so sharing them is safe.
func (c *CodecContext) internInitValues(roleName, enc string) (*Struct, error) {
	key := roleName + "\x00" + enc
	c.mu.RLock()
	s, ok := c.initValues[key]
	c.mu.RUnlock()
	if ok {
		return s, nil
	}
	r := NewStateReader(c, []byte(enc))
	v, err := r.ReadValue()
	if err == nil {
		err = r.Done()
	}
	if err != nil {
		return nil, err
	}
	s, ok = v.(*Struct)
	if !ok {
		return nil, fmt.Errorf("%w: init values is %s", ErrCorruptState, v.Type())
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.initValues[key]; ok {
		return existing, nil
	}
	c.initValues[key] = s
	return s, nil
}

func (c *CodecContext) internOpaque(key any, v any) uint64 {
	c.mu.RLock()
	idx, ok := c.opaqueIdx[key]
	c.mu.RUnlock()
	if ok {
		return idx
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if idx, ok := c.opaqueIdx[key]; ok {
		return idx
	}
	idx = uint64(len(c.opaque))
	c.opaque = append(c.opaque, v)
	c.opaqueIdx[key] = idx
	return idx
}

func (c *CodecContext) opaqueAt(idx uint64) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if idx >= uint64(len(c.opaque)) {
		return nil, false
	}
	return c.opaque[idx], true
}

// internChannel returns the context's channel equal to ch, adding ch if
// there is none. The writer interns what it writes, so the reader returns
// the very pointers that were written, as a deep clone shares them.
func (c *CodecContext) internChannel(ch *Channel) *Channel {
	k := channelKey{ch.Id, ch.Name, ch.ordering, ch.delivery, ch.blocking}
	c.mu.RLock()
	existing, ok := c.channels[k]
	c.mu.RUnlock()
	if ok {
		return existing
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.channels[k]; ok {
		return existing
	}
	c.channels[k] = ch
	return ch
}

func (c *CodecContext) internDomain(d *SymmetryDomain) *SymmetryDomain {
	c.mu.RLock()
	existing, ok := c.domains[*d]
	c.mu.RUnlock()
	if ok {
		return existing
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.domains[*d]; ok {
		return existing
	}
	c.domains[*d] = d
	return d
}

// StateWriter appends the encoding of one state to a byte buffer.
// Everything written with one writer between Resets shares one value pool.
type StateWriter struct {
	ctx    *CodecContext
	buf    []byte
	refs   map[starlark.Value]uint64
	nrefs  uint64
	strs   map[string]uint64
	shapes map[shapeKey]uint64
	// nshapes counts shapes written, including uncached ones.
	nshapes uint64
	// strBytes counts the bytes of first occurrences of strings.
	strBytes int
}

// shapeKey identifies a record shape by its kind and the encoding of its
// constructor and field names.
type shapeKey struct {
	kind  byte
	shape string
}

var writerPool = sync.Pool{New: func() any {
	return &StateWriter{
		buf:    make([]byte, 0, 1024),
		refs:   make(map[starlark.Value]uint64),
		strs:   make(map[string]uint64),
		shapes: make(map[shapeKey]uint64),
	}
}}

// NewStateWriter returns an empty writer from a pool. Call Release when done
// with Bytes().
func NewStateWriter(ctx *CodecContext) *StateWriter {
	w := writerPool.Get().(*StateWriter)
	w.ctx = ctx
	w.Reset()
	return w
}

func (w *StateWriter) Reset() {
	w.buf = w.buf[:0]
	clear(w.refs)
	clear(w.strs)
	clear(w.shapes)
	w.nrefs = 0
	w.nshapes = 0
	w.strBytes = 0
}

func (w *StateWriter) Release() {
	if cap(w.buf) > 1<<20 {
		w.buf = make([]byte, 0, 1024)
	}
	w.ctx = nil
	writerPool.Put(w)
}

// StringBytes returns how many of the bytes written so far are first
// occurrences of strings (headers included): what a dictionary shared
// across states could save at most.
func (w *StateWriter) StringBytes() int { return w.strBytes }

// Bytes returns the encoding written so far, valid until the next write,
// Reset or Release.
func (w *StateWriter) Bytes() []byte { return w.buf }

func (w *StateWriter) WriteUint8(b byte) { w.buf = append(w.buf, b) }

func (w *StateWriter) WriteUvarint(n uint64) { w.buf = binary.AppendUvarint(w.buf, n) }

func (w *StateWriter) WriteVarint(n int64) { w.buf = binary.AppendVarint(w.buf, n) }

func (w *StateWriter) WriteBool(b bool) {
	if b {
		w.buf = append(w.buf, 1)
	} else {
		w.buf = append(w.buf, 0)
	}
}

// WriteString writes a string through the string table: the first
// occurrence is (len<<1) followed by the bytes, a repeat is (index<<1)|1.
func (w *StateWriter) WriteString(s string) {
	if idx, ok := w.strs[s]; ok {
		w.WriteUvarint(idx<<1 | 1)
		return
	}
	w.strs[s] = uint64(len(w.strs))
	n := len(w.buf)
	w.WriteUvarint(uint64(len(s)) << 1)
	w.strBytes += len(w.buf) - n + len(s)
	w.buf = append(w.buf, s...)
}

func (w *StateWriter) WriteStrings(ss []string) {
	w.WriteUvarint(uint64(len(ss)))
	for _, s := range ss {
		w.WriteString(s)
	}
}

// WriteRawBytes writes length-prefixed bytes, not through the string table.
func (w *StateWriter) WriteRawBytes(b []byte) {
	w.WriteUvarint(uint64(len(b)))
	w.buf = append(w.buf, b...)
}

// skipInStringDict reports whether a StringDict entry is left out, as
// modelchecker.CopyDict leaves it out of a cloned state.
func skipInStringDict(v starlark.Value) bool {
	switch v.Type() {
	case "builtin_function_or_method", "module", "failed_module":
		return true
	}
	return false
}

// WriteStringDict writes the entries of d in sorted key order, leaving out
// builtins and modules as a deep clone does.
func (w *StateWriter) WriteStringDict(d starlark.StringDict) error {
	keys := d.Keys()
	n := 0
	for _, k := range keys {
		if !skipInStringDict(d[k]) {
			n++
		}
	}
	w.WriteUvarint(uint64(n))
	for _, k := range keys {
		v := d[k]
		if skipInStringDict(v) {
			continue
		}
		w.WriteString(k)
		if err := w.WriteValue(v); err != nil {
			return fmt.Errorf("%s: %w", k, err)
		}
	}
	return nil
}

// ref writes a back-reference if v was already written and reports whether
// it did.
func (w *StateWriter) ref(v starlark.Value) bool {
	if idx, ok := w.refs[v]; ok {
		w.buf = append(w.buf, vtRef)
		w.WriteUvarint(idx)
		return true
	}
	return false
}

func (w *StateWriter) register(v starlark.Value) {
	w.refs[v] = w.nrefs
	w.nrefs++
}

func (w *StateWriter) writeValues(n int, iter starlark.Iterator) error {
	defer iter.Done()
	w.WriteUvarint(uint64(n))
	var x starlark.Value
	for iter.Next(&x) {
		if err := w.WriteValue(x); err != nil {
			return err
		}
	}
	return nil
}

func (w *StateWriter) writeSlice(elems []starlark.Value) error {
	w.WriteUvarint(uint64(len(elems)))
	for _, x := range elems {
		if err := w.WriteValue(x); err != nil {
			return err
		}
	}
	return nil
}

// writeShape writes a record shape: (index+1) for a known one, or 0 followed
// by the constructor and the field names.
func (w *StateWriter) writeShape(kind byte, constructor starlark.Value, names []string) error {
	// Constructors are strings in practice ("record", "params", "fields",
	// a role or enum name); anything else gets a shape of its own.
	if c, ok := constructor.(starlark.String); ok {
		var sb []byte
		sb = binary.AppendUvarint(sb, uint64(len(c)))
		sb = append(sb, c...)
		for _, n := range names {
			sb = binary.AppendUvarint(sb, uint64(len(n)))
			sb = append(sb, n...)
		}
		key := shapeKey{kind: kind, shape: string(sb)}
		if idx, ok := w.shapes[key]; ok {
			w.WriteUvarint(idx + 1)
			return nil
		}
		w.shapes[key] = w.nshapes
	}
	// The shape is numbered before its constructor is written, as the
	// reader numbers it before reading the constructor.
	w.nshapes++
	w.WriteUvarint(0)
	if err := w.WriteValue(constructor); err != nil {
		return err
	}
	w.WriteStrings(names)
	return nil
}

// WriteValue writes one starlark value.
func (w *StateWriter) WriteValue(v starlark.Value) error {
	switch x := v.(type) {
	case starlark.NoneType:
		w.buf = append(w.buf, vtNone)
	case starlark.Bool:
		if x {
			w.buf = append(w.buf, vtTrue)
		} else {
			w.buf = append(w.buf, vtFalse)
		}
	case starlark.Int:
		if i, ok := x.Int64(); ok {
			w.buf = append(w.buf, vtInt)
			w.WriteVarint(i)
		} else {
			b := x.BigInt()
			w.buf = append(w.buf, vtBigInt)
			w.WriteUint8(byte(b.Sign() + 1))
			w.WriteRawBytes(b.Bytes())
		}
	case starlark.Float:
		w.buf = append(w.buf, vtFloat)
		w.buf = binary.LittleEndian.AppendUint64(w.buf, math.Float64bits(float64(x)))
	case starlark.String:
		w.buf = append(w.buf, vtString)
		w.WriteString(string(x))
	case starlark.Bytes:
		w.buf = append(w.buf, vtBytes)
		w.WriteRawBytes([]byte(x))
	case starlark.Tuple:
		w.buf = append(w.buf, vtTuple)
		return w.writeSlice(x)
	case SymmetricValues:
		w.buf = append(w.buf, vtSymValues)
		return w.writeSlice(x.Tuple)
	case *SymmetricValues:
		w.buf = append(w.buf, vtSymValues)
		return w.writeSlice(x.Tuple)
	case *starlark.List:
		if w.ref(v) {
			return nil
		}
		w.register(v)
		w.buf = append(w.buf, vtList)
		return w.writeValues(x.Len(), x.Iterate())
	case *starlark.Dict:
		if w.ref(v) {
			return nil
		}
		w.register(v)
		w.buf = append(w.buf, vtDict)
		items := x.Items()
		w.WriteUvarint(uint64(len(items)))
		for _, kv := range items {
			if err := w.WriteValue(kv[0]); err != nil {
				return err
			}
			if err := w.WriteValue(kv[1]); err != nil {
				return err
			}
		}
	case *starlark.Set:
		if w.ref(v) {
			return nil
		}
		w.register(v)
		w.buf = append(w.buf, vtSet)
		return w.writeValues(x.Len(), x.Iterate())
	case *Struct:
		if w.ref(v) {
			return nil
		}
		w.register(v)
		w.buf = append(w.buf, vtRecord)
		names := make([]string, len(x.entries))
		for i, e := range x.entries {
			names[i] = e.name
		}
		if err := w.writeShape(vtRecord, x.constructor, names); err != nil {
			return err
		}
		for _, e := range x.entries {
			if err := w.WriteValue(e.value); err != nil {
				return fmt.Errorf("%s: %w", e.name, err)
			}
		}
	case *starlarkstruct.Struct:
		if w.ref(v) {
			return nil
		}
		w.buf = append(w.buf, vtStruct)
		names := x.AttrNames()
		if err := w.writeShape(vtStruct, x.Constructor(), names); err != nil {
			return err
		}
		for _, name := range names {
			fv, err := x.Attr(name)
			if err != nil {
				return err
			}
			if err := w.WriteValue(fv); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
		// Numbered after its fields: see the package comment.
		w.register(v)
	case *SymmetricValue:
		if w.ref(v) {
			return nil
		}
		w.register(v)
		w.buf = append(w.buf, vtSymValue)
		w.WriteString(x.prefix)
		w.WriteVarint(x.id)
		w.WriteString(string(x.Kind))
		w.WriteVarint(int64(x.Limit))
	case *ModelValue:
		if w.ref(v) {
			return nil
		}
		w.register(v)
		w.buf = append(w.buf, vtModel)
		w.WriteString(x.prefix)
		w.WriteVarint(x.id)
	case *Role:
		if w.ref(v) {
			return nil
		}
		w.buf = append(w.buf, vtRole)
		w.WriteString(x.Name)
		w.WriteBool(x.Symmetric)
		if err := w.WriteValue(x.ID); err != nil {
			return err
		}
		w.register(v)
		// Methods are the same for every role of a type once its Init has
		// run, so only whether they are there is written.
		w.ctx.noteRole(x)
		w.WriteBool(len(x.Methods) > 0)
		// Init values are written as a self-contained encoding, through
		// the string table: roles with equal init values share it, and the
		// reader interns them by it.
		sub := NewStateWriter(w.ctx)
		if x.InitValues == nil {
			sub.buf = append(sub.buf, vtNone)
		} else if err := sub.WriteValue(x.InitValues); err != nil {
			sub.Release()
			return err
		}
		w.WriteString(string(sub.Bytes()))
		sub.Release()
		if err := w.WriteValue(x.Params); err != nil {
			return err
		}
		return w.WriteValue(x.Fields)
	case *RoleStub:
		if w.ref(v) {
			return nil
		}
		w.buf = append(w.buf, vtRoleStub)
		if err := w.WriteValue(x.Role); err != nil {
			return err
		}
		if err := w.WriteValue(x.Channel); err != nil {
			return err
		}
		w.register(v)
	case *Bag:
		if w.ref(v) {
			return nil
		}
		w.register(v)
		w.buf = append(w.buf, vtBag)
		return w.writeSlice(x.elems)
	case *GenericMap:
		if w.ref(v) {
			return nil
		}
		w.register(v)
		w.buf = append(w.buf, vtGenMap)
		w.WriteUvarint(uint64(len(x.entries)))
		for _, kv := range x.entries {
			if err := w.WriteValue(kv[0]); err != nil {
				return err
			}
			if err := w.WriteValue(kv[1]); err != nil {
				return err
			}
		}
	case *GenericSet:
		if w.ref(v) {
			return nil
		}
		w.register(v)
		w.buf = append(w.buf, vtGenSet)
		w.WriteUvarint(uint64(len(x.dict.entries)))
		for _, kv := range x.dict.entries {
			if err := w.WriteValue(kv[0]); err != nil {
				return err
			}
		}
	case *Segment:
		if w.ref(v) {
			return nil
		}
		w.buf = append(w.buf, vtSegment)
		if err := w.WriteValue(x.Domain); err != nil {
			return err
		}
		w.register(v)
		w.WriteVarint(x.Left)
		w.WriteVarint(x.Right)
		w.WriteUint8(boolBits(x.IsHead, x.IsTail, x.IsPoint, x.Resolved))
		w.WriteVarint(x.ResolvedID)
	case *Channel:
		if w.ref(v) {
			return nil
		}
		w.register(v)
		w.ctx.internChannel(x)
		w.buf = append(w.buf, vtChannel)
		w.WriteVarint(int64(x.Id))
		w.WriteString(x.Name)
		w.WriteString(x.ordering)
		w.WriteString(x.delivery)
		w.WriteString(x.blocking)
	case *SymmetryDomain:
		if w.ref(v) {
			return nil
		}
		w.register(v)
		w.ctx.internDomain(x)
		w.buf = append(w.buf, vtDomain)
		w.WriteString(x.Name)
		w.WriteVarint(int64(x.Limit))
		w.WriteString(string(x.Kind))
		w.WriteVarint(int64(x.Divergence))
		w.WriteVarint(int64(x.Start))
		w.WriteUint8(boolBits(x.Materialize, x.Reflection))
	default:
		switch v.Type() {
		case "range":
			r := v.(starlark.Indexable)
			start, stop, step, err := rangeBounds(r)
			if err != nil {
				return err
			}
			w.buf = append(w.buf, vtRange)
			w.WriteVarint(start)
			w.WriteVarint(stop)
			w.WriteVarint(step)
			return nil
		}
		// A function or builtin that is a spec global is written by name.
		if fn, ok := v.(interface{ Name() string }); ok {
			if resolver := w.ctx.getResolver(); resolver != nil {
				if g, ok := resolver.Global(fn.Name()); ok && g == v {
					w.buf = append(w.buf, vtGlobal)
					w.WriteString(fn.Name())
					return nil
				}
			}
		}
		// Anything else without an encoding of its own (a lambda, say): a
		// deep clone shares these between states, so share them through
		// the context's opaque table, which only works in this process.
		// Only pointers have a stable identity.
		if reflect.TypeOf(v).Kind() != reflect.Ptr {
			return fmt.Errorf("state codec: unsupported value type %T (%s)", v, v.Type())
		}
		w.buf = append(w.buf, vtOpaque)
		w.WriteUvarint(w.ctx.internOpaque(v, v))
	}
	return nil
}

func boolBits(bs ...bool) byte {
	var b byte
	for i, x := range bs {
		if x {
			b |= 1 << i
		}
	}
	return b
}

// rangeBounds recovers start, stop and step of a starlark range, whose type
// is unexported. Ranges compare by their elements, so for an empty or
// one-element range any bounds with the same elements will do.
func rangeBounds(r starlark.Indexable) (int64, int64, int64, error) {
	n := r.Len()
	if n == 0 {
		return 0, 0, 1, nil
	}
	first, ok := r.Index(0).(starlark.Int).Int64()
	if !ok {
		return 0, 0, 0, fmt.Errorf("state codec: range out of int64")
	}
	step := int64(1)
	if n > 1 {
		second, _ := r.Index(1).(starlark.Int).Int64()
		step = second - first
	}
	return first, first + int64(n)*step, step, nil
}

// ErrCorruptState is returned (wrapped) for input that isn't a valid
// encoding.
var ErrCorruptState = errors.New("state codec: corrupt state")

// StateReader decodes what a StateWriter wrote. Primitive reads record the
// first error and return zero values after it; check Err.
type StateReader struct {
	ctx    *CodecContext
	buf    []byte
	pos    int
	err    error
	refs   []starlark.Value
	strs   []string
	shapes []readShape
}

type readShape struct {
	constructor starlark.Value
	names       []string
}

func NewStateReader(ctx *CodecContext, data []byte) *StateReader {
	return &StateReader{ctx: ctx, buf: data}
}

func (r *StateReader) Err() error { return r.err }

// Done reports an error unless every byte was consumed.
func (r *StateReader) Done() error {
	if r.err == nil && r.pos != len(r.buf) {
		r.fail("%d trailing bytes", len(r.buf)-r.pos)
	}
	return r.err
}

func (r *StateReader) fail(format string, args ...any) {
	if r.err == nil {
		r.err = fmt.Errorf("%w at byte %d: %s", ErrCorruptState, r.pos, fmt.Sprintf(format, args...))
	}
}

func (r *StateReader) ReadUint8() byte {
	if r.err != nil {
		return 0
	}
	if r.pos >= len(r.buf) {
		r.fail("unexpected end")
		return 0
	}
	b := r.buf[r.pos]
	r.pos++
	return b
}

func (r *StateReader) ReadUvarint() uint64 {
	if r.err != nil {
		return 0
	}
	n, k := binary.Uvarint(r.buf[r.pos:])
	if k <= 0 {
		r.fail("bad uvarint")
		return 0
	}
	r.pos += k
	return n
}

func (r *StateReader) ReadVarint() int64 {
	if r.err != nil {
		return 0
	}
	n, k := binary.Varint(r.buf[r.pos:])
	if k <= 0 {
		r.fail("bad varint")
		return 0
	}
	r.pos += k
	return n
}

func (r *StateReader) ReadBool() bool { return r.ReadUint8() != 0 }

// ReadCount reads a count of items that take at least one byte each, so a
// corrupt count can't make the reader allocate more than the input size.
func (r *StateReader) ReadCount() int {
	n := r.ReadUvarint()
	if n > uint64(len(r.buf)-r.pos) {
		r.fail("count %d exceeds input", n)
		return 0
	}
	return int(n)
}

func (r *StateReader) readRaw() []byte {
	n := r.ReadUvarint()
	if r.err != nil {
		return nil
	}
	if n > uint64(len(r.buf)-r.pos) {
		r.fail("length %d exceeds input", n)
		return nil
	}
	b := r.buf[r.pos : r.pos+int(n)]
	r.pos += int(n)
	return b
}

// ReadRawBytes reads what WriteRawBytes wrote. The result aliases the input.
func (r *StateReader) ReadRawBytes() []byte { return r.readRaw() }

func (r *StateReader) ReadString() string {
	h := r.ReadUvarint()
	if r.err != nil {
		return ""
	}
	if h&1 == 1 {
		idx := h >> 1
		if idx >= uint64(len(r.strs)) {
			r.fail("string ref %d out of range", idx)
			return ""
		}
		return r.strs[idx]
	}
	n := h >> 1
	if n > uint64(len(r.buf)-r.pos) {
		r.fail("string length %d exceeds input", n)
		return ""
	}
	s := string(r.buf[r.pos : r.pos+int(n)])
	r.pos += int(n)
	r.strs = append(r.strs, s)
	return s
}

func (r *StateReader) ReadStrings() []string {
	n := r.ReadCount()
	if n == 0 {
		return nil
	}
	ss := make([]string, n)
	for i := range ss {
		ss[i] = r.ReadString()
	}
	return ss
}

func (r *StateReader) ReadStringDict() (starlark.StringDict, error) {
	n := r.ReadCount()
	d := make(starlark.StringDict, n)
	for i := 0; i < n; i++ {
		k := r.ReadString()
		v, err := r.ReadValue()
		if err != nil {
			return nil, err
		}
		d[k] = v
	}
	return d, r.err
}

// reserve takes the next value-table slot, to be filled by the caller.
func (r *StateReader) reserve() int {
	r.refs = append(r.refs, nil)
	return len(r.refs) - 1
}

func (r *StateReader) register(v starlark.Value) {
	r.refs = append(r.refs, v)
}

func (r *StateReader) readValues() ([]starlark.Value, error) {
	n := r.ReadCount()
	if r.err != nil {
		return nil, r.err
	}
	elems := make([]starlark.Value, n)
	for i := range elems {
		v, err := r.ReadValue()
		if err != nil {
			return nil, err
		}
		elems[i] = v
	}
	return elems, nil
}

func (r *StateReader) readShape() (readShape, error) {
	h := r.ReadUvarint()
	if r.err != nil {
		return readShape{}, r.err
	}
	if h > 0 {
		if h-1 >= uint64(len(r.shapes)) {
			r.fail("shape ref %d out of range", h-1)
			return readShape{}, r.err
		}
		return r.shapes[h-1], nil
	}
	// Take the slot before reading the constructor, as the writer numbers
	// the shape before writing it.
	idx := len(r.shapes)
	r.shapes = append(r.shapes, readShape{})
	constructor, err := r.ReadValue()
	if err != nil {
		return readShape{}, err
	}
	s := readShape{constructor: constructor, names: r.ReadStrings()}
	r.shapes[idx] = s
	return s, r.err
}

// ReadValue reads one value written by WriteValue.
func (r *StateReader) ReadValue() (starlark.Value, error) {
	tag := r.ReadUint8()
	if r.err != nil {
		return nil, r.err
	}
	switch tag {
	case vtNone:
		return starlark.None, nil
	case vtFalse:
		return starlark.False, nil
	case vtTrue:
		return starlark.True, nil
	case vtInt:
		return starlark.MakeInt64(r.ReadVarint()), r.err
	case vtBigInt:
		sign := int(r.ReadUint8()) - 1
		b := new(big.Int).SetBytes(r.readRaw())
		if sign < 0 {
			b.Neg(b)
		}
		return starlark.MakeBigInt(b), r.err
	case vtFloat:
		if len(r.buf)-r.pos < 8 {
			r.fail("short float")
			return nil, r.err
		}
		f := math.Float64frombits(binary.LittleEndian.Uint64(r.buf[r.pos:]))
		r.pos += 8
		return starlark.Float(f), nil
	case vtString:
		return starlark.String(r.ReadString()), r.err
	case vtBytes:
		return starlark.Bytes(r.readRaw()), r.err
	case vtTuple:
		elems, err := r.readValues()
		return starlark.Tuple(elems), err
	case vtSymValues:
		elems, err := r.readValues()
		return SymmetricValues{Tuple: elems}, err
	case vtRef:
		idx := r.ReadUvarint()
		if r.err != nil {
			return nil, r.err
		}
		if idx >= uint64(len(r.refs)) || r.refs[idx] == nil {
			r.fail("value ref %d out of range or not yet built", idx)
			return nil, r.err
		}
		return r.refs[idx], nil
	case vtList:
		list := starlark.NewList(nil)
		r.register(list)
		elems, err := r.readValues()
		if err != nil {
			return nil, err
		}
		*list = *starlark.NewList(elems)
		return list, nil
	case vtDict:
		n := r.ReadCount()
		dict := starlark.NewDict(n)
		r.register(dict)
		for i := 0; i < n; i++ {
			k, err := r.ReadValue()
			if err != nil {
				return nil, err
			}
			v, err := r.ReadValue()
			if err != nil {
				return nil, err
			}
			if err := dict.SetKey(k, v); err != nil {
				return nil, err
			}
		}
		return dict, r.err
	case vtSet:
		n := r.ReadCount()
		set := starlark.NewSet(n)
		r.register(set)
		for i := 0; i < n; i++ {
			k, err := r.ReadValue()
			if err != nil {
				return nil, err
			}
			if err := set.Insert(k); err != nil {
				return nil, err
			}
		}
		return set, r.err
	case vtRecord:
		s := &Struct{}
		r.register(s)
		shape, err := r.readShape()
		if err != nil {
			return nil, err
		}
		s.constructor = shape.constructor
		s.entries = make(entries, len(shape.names))
		for i, name := range shape.names {
			v, err := r.ReadValue()
			if err != nil {
				return nil, err
			}
			s.entries[i] = entry{name, v}
		}
		return s, nil
	case vtStruct:
		shape, err := r.readShape()
		if err != nil {
			return nil, err
		}
		d := make(starlark.StringDict, len(shape.names))
		for _, name := range shape.names {
			v, err := r.ReadValue()
			if err != nil {
				return nil, err
			}
			d[name] = v
		}
		s := starlarkstruct.FromStringDict(shape.constructor, d)
		r.register(s)
		return s, nil
	case vtSymValue:
		prefix := r.ReadString()
		id := r.ReadVarint()
		kind := SymmetryKind(r.ReadString())
		limit := int(r.ReadVarint())
		sv := &SymmetricValue{ModelValue: ModelValue{prefix: prefix, id: id}, Kind: kind, Limit: limit}
		r.register(sv)
		return sv, r.err
	case vtModel:
		mv := NewModelValue(r.ReadString(), r.ReadVarint())
		r.register(mv)
		return mv, r.err
	case vtRole:
		name := r.ReadString()
		symmetric := r.ReadBool()
		id, err := r.ReadValue()
		if err != nil {
			return nil, err
		}
		role := &Role{ID: id, Name: name, Symmetric: symmetric}
		r.register(role)
		hasMethods := r.ReadBool()
		initEnc := r.ReadString()
		if r.err != nil {
			return nil, r.err
		}
		t, err := r.ctx.roleType(name, hasMethods)
		if err != nil {
			return nil, err
		}
		role.RoleMethods = t.roleMethods
		if hasMethods {
			role.Methods = t.methods
		} else {
			// Init adds the methods to this map, so it is the role's own.
			role.Methods = map[string]*starlark.Function{}
		}
		if initEnc != string([]byte{vtNone}) {
			if role.InitValues, err = r.ctx.internInitValues(name, initEnc); err != nil {
				return nil, err
			}
		}
		var ok bool
		params, err := r.ReadValue()
		if err != nil {
			return nil, err
		}
		fields, err := r.ReadValue()
		if err != nil {
			return nil, err
		}
		if role.Params, ok = params.(*Struct); !ok {
			r.fail("role params is %s", params.Type())
		}
		if role.Fields, ok = fields.(*Struct); !ok {
			r.fail("role fields is %s", fields.Type())
		}
		return role, r.err
	case vtRoleStub:
		role, err := r.ReadValue()
		if err != nil {
			return nil, err
		}
		ch, err := r.ReadValue()
		if err != nil {
			return nil, err
		}
		rr, ok1 := role.(*Role)
		cc, ok2 := ch.(*Channel)
		if !ok1 || !ok2 {
			r.fail("bad RoleStub")
			return nil, r.err
		}
		stub := NewRoleStub(rr, cc)
		r.register(stub)
		return stub, nil
	case vtBag:
		bag := &Bag{}
		r.register(bag)
		elems, err := r.readValues()
		if err != nil {
			return nil, err
		}
		bag.elems = elems
		return bag, nil
	case vtGenMap:
		m := &GenericMap{}
		r.register(m)
		n := r.ReadCount()
		m.entries = make([]starlark.Tuple, n)
		for i := 0; i < n; i++ {
			k, err := r.ReadValue()
			if err != nil {
				return nil, err
			}
			v, err := r.ReadValue()
			if err != nil {
				return nil, err
			}
			m.entries[i] = starlark.Tuple{k, v}
		}
		return m, r.err
	case vtGenSet:
		s := &GenericSet{dict: &GenericMap{}}
		r.register(s)
		keys, err := r.readValues()
		if err != nil {
			return nil, err
		}
		s.dict.entries = make([]starlark.Tuple, len(keys))
		for i, k := range keys {
			s.dict.entries[i] = starlark.Tuple{k, starlark.None}
		}
		return s, nil
	case vtSegment:
		d, err := r.ReadValue()
		if err != nil {
			return nil, err
		}
		domain, ok := d.(*SymmetryDomain)
		if !ok {
			r.fail("segment domain is %s", d.Type())
			return nil, r.err
		}
		seg := &Segment{Domain: domain}
		r.register(seg)
		seg.Left = r.ReadVarint()
		seg.Right = r.ReadVarint()
		flags := r.ReadUint8()
		seg.IsHead, seg.IsTail, seg.IsPoint, seg.Resolved = flags&1 != 0, flags&2 != 0, flags&4 != 0, flags&8 != 0
		seg.ResolvedID = r.ReadVarint()
		return seg, r.err
	case vtChannel:
		ch := &Channel{Id: int(r.ReadVarint()), Name: r.ReadString()}
		ch.ordering, ch.delivery, ch.blocking = r.ReadString(), r.ReadString(), r.ReadString()
		if r.err != nil {
			return nil, r.err
		}
		ch = r.ctx.internChannel(ch)
		r.register(ch)
		return ch, nil
	case vtDomain:
		d := &SymmetryDomain{Name: r.ReadString(), Limit: int(r.ReadVarint()), Kind: SymmetryKind(r.ReadString())}
		d.Divergence, d.Start = int(r.ReadVarint()), int(r.ReadVarint())
		flags := r.ReadUint8()
		d.Materialize, d.Reflection = flags&1 != 0, flags&2 != 0
		if r.err != nil {
			return nil, r.err
		}
		d = r.ctx.internDomain(d)
		r.register(d)
		return d, nil
	case vtRange:
		start, stop, step := r.ReadVarint(), r.ReadVarint(), r.ReadVarint()
		if r.err != nil {
			return nil, r.err
		}
		return starlark.Call(&starlark.Thread{}, starlark.Universe["range"],
			starlark.Tuple{starlark.MakeInt64(start), starlark.MakeInt64(stop), starlark.MakeInt64(step)}, nil)
	case vtGlobal:
		name := r.ReadString()
		if r.err != nil {
			return nil, r.err
		}
		resolver := r.ctx.getResolver()
		if resolver == nil {
			r.fail("global %s: no resolver in the codec context", name)
			return nil, r.err
		}
		v, ok := resolver.Global(name)
		if !ok {
			r.fail("global %s not found", name)
			return nil, r.err
		}
		return v, nil
	case vtOpaque:
		o, ok := r.ctx.opaqueAt(r.ReadUvarint())
		v, ok2 := o.(starlark.Value)
		if !ok || !ok2 {
			r.fail("opaque value not in context")
			return nil, r.err
		}
		return v, nil
	default:
		r.fail("unknown value tag %d", tag)
		return nil, r.err
	}
}
