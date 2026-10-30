package lib

import (
	"errors"
	"math"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

func roundTripValues(t *testing.T, ctx *CodecContext, vals ...starlark.Value) ([]starlark.Value, []byte) {
	t.Helper()
	w := NewStateWriter(ctx)
	defer w.Release()
	for _, v := range vals {
		require.NoError(t, w.WriteValue(v))
	}
	data := append([]byte(nil), w.Bytes()...)
	r := NewStateReader(ctx, data)
	out := make([]starlark.Value, len(vals))
	for i := range vals {
		v, err := r.ReadValue()
		require.NoError(t, err)
		out[i] = v
	}
	require.NoError(t, r.Done())

	// The decoded values encode to the same bytes.
	w2 := NewStateWriter(ctx)
	defer w2.Release()
	for _, v := range out {
		require.NoError(t, w2.WriteValue(v))
	}
	assert.Equal(t, data, w2.Bytes())
	return out, data
}

func assertStarlarkEqual(t *testing.T, want, got starlark.Value) {
	t.Helper()
	assert.Equal(t, want.Type(), got.Type())
	assert.Equal(t, want.String(), got.String())
	switch want.Type() {
	case "symmetry_segment", "Channel", "symmetric_values":
		// Compared by identity, or (symmetric_values) not comparable.
		return
	}
	if eq, err := starlark.Equal(want, got); err == nil {
		assert.True(t, eq, "%s != %s", want, got)
	}
}

func TestStateCodecScalars(t *testing.T) {
	huge := new(big.Int).Lsh(big.NewInt(1), 100)
	vals := []starlark.Value{
		starlark.None, starlark.True, starlark.False,
		starlark.MakeInt(0), starlark.MakeInt(-1), starlark.MakeInt64(math.MaxInt64), starlark.MakeInt64(math.MinInt64),
		starlark.MakeBigInt(huge), starlark.MakeBigInt(new(big.Int).Neg(huge)),
		starlark.Float(1.5), starlark.Float(math.Inf(-1)),
		starlark.String(""), starlark.String("hello"), starlark.String("hello"),
		starlark.Bytes("\x00\xff"),
		starlark.Tuple{starlark.MakeInt(1), starlark.String("a")},
		starlark.Tuple{},
	}
	out, _ := roundTripValues(t, NewCodecContext(), vals...)
	for i := range vals {
		assertStarlarkEqual(t, vals[i], out[i])
	}
	nan, _ := roundTripValues(t, NewCodecContext(), starlark.Float(math.NaN()))
	assert.True(t, math.IsNaN(float64(nan[0].(starlark.Float))))
}

func TestStateCodecRange(t *testing.T) {
	for _, args := range [][]int{{5}, {2, 10, 3}, {10, 0, -2}, {3, 3}, {7, 8}} {
		tuple := starlark.Tuple{}
		for _, a := range args {
			tuple = append(tuple, starlark.MakeInt(a))
		}
		rng, err := starlark.Call(&starlark.Thread{}, starlark.Universe["range"], tuple, nil)
		require.NoError(t, err)
		out, _ := roundTripValues(t, NewCodecContext(), rng)
		eq, err := starlark.Equal(rng, out[0])
		require.NoError(t, err)
		assert.True(t, eq, "%s != %s", rng, out[0])
	}
}

func TestStateCodecContainers(t *testing.T) {
	dict := starlark.NewDict(2)
	require.NoError(t, dict.SetKey(starlark.String("b"), starlark.MakeInt(2)))
	require.NoError(t, dict.SetKey(starlark.Tuple{starlark.MakeInt(1)}, starlark.NewList([]starlark.Value{starlark.None})))
	set := starlark.NewSet(2)
	require.NoError(t, set.Insert(starlark.MakeInt(3)))
	require.NoError(t, set.Insert(starlark.String("x")))
	rec := FromStringDict(Default, starlark.StringDict{"z": starlark.MakeInt(1), "a": starlark.String("v")})
	rec2 := FromStringDict(Default, starlark.StringDict{"z": starlark.MakeInt(2), "a": starlark.String("w")})
	st := starlarkstruct.FromStringDict(starlark.String("enum"), starlark.StringDict{"RED": starlark.String("RED")})
	gm := GenericMapFromEntries([]starlark.Tuple{{starlark.NewList(nil), starlark.MakeInt(1)}})
	gs := GenericSetFromDistinct([]starlark.Value{starlark.NewDict(0), starlark.MakeInt(1)})
	bag := NewBag([]starlark.Value{starlark.MakeInt(2), starlark.MakeInt(1), starlark.MakeInt(2)})
	mv := NewModelValue("n", 3)
	sv := NewRotationalSymmetricValue("r", 1, 4)
	domain := &SymmetryDomain{Name: "T", Limit: 3, Kind: SymmetryKindOrdinal, Materialize: true}
	seg := &Segment{Domain: domain, Left: 1, Right: 4, IsTail: true, Resolved: true, ResolvedID: 2}
	ch := &Channel{Id: 2, Name: "net", ordering: "unordered", delivery: "atmost_once", blocking: "fire_and_forget"}
	vals := []starlark.Value{
		starlark.NewList([]starlark.Value{starlark.MakeInt(1), starlark.String("a")}),
		dict, set, rec, rec2, st, gm, gs, bag, mv, sv, domain, seg, ch,
		SymmetricValues{Tuple: starlark.Tuple{NewSymmetricValue("s", 0), NewSymmetricValue("s", 1)}},
	}
	ctx := NewCodecContext()
	out, _ := roundTripValues(t, ctx, vals...)
	for i := range vals {
		assertStarlarkEqual(t, vals[i], out[i])
	}
	// Configuration values come back as the very same pointers.
	assert.Same(t, domain, out[11])
	assert.Same(t, ch, out[13])
	assert.Same(t, domain, out[12].(*Segment).Domain)
	assert.Equal(t, *seg, *out[12].(*Segment))
	assert.Equal(t, *sv, *out[10].(*SymmetricValue))
}

func TestStateCodecAliasing(t *testing.T) {
	shared := starlark.NewList([]starlark.Value{starlark.MakeInt(1)})
	mv := NewModelValue("n", 0)
	holder := FromStringDict(Default, starlark.StringDict{"a": shared, "b": shared, "id": mv})
	outer := starlark.NewList([]starlark.Value{shared, holder, mv})

	out, _ := roundTripValues(t, NewCodecContext(), outer, shared)
	list := out[0].(*starlark.List)
	got := list.Index(0).(*starlark.List)
	rec := list.Index(1).(*Struct)
	a, _ := rec.Attr("a")
	b, _ := rec.Attr("b")
	id, _ := rec.Attr("id")
	assert.Same(t, got, a)
	assert.Same(t, got, b)
	assert.Same(t, got, out[1], "references across top-level values share one pool")
	assert.Same(t, list.Index(2), id)

	// Mutating through one alias is seen through the other, as before.
	require.NoError(t, got.Append(starlark.MakeInt(2)))
	assert.Equal(t, 2, a.(*starlark.List).Len())
}

func TestStateCodecCycles(t *testing.T) {
	list := starlark.NewList(nil)
	dict := starlark.NewDict(1)
	require.NoError(t, dict.SetKey(starlark.String("self"), list))
	require.NoError(t, list.Append(list))
	require.NoError(t, list.Append(dict))
	rec := FromStringDict(Default, starlark.StringDict{})
	require.NoError(t, rec.SetField("me", rec))
	// A struct can't hold itself, but it can sit in a cycle through a
	// mutable container.
	inner := starlark.NewList(nil)
	st := starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{"l": inner})
	require.NoError(t, inner.Append(st))

	out, _ := roundTripValues(t, NewCodecContext(), list, rec, inner)
	l := out[0].(*starlark.List)
	assert.Same(t, l, l.Index(0))
	d := l.Index(1).(*starlark.Dict)
	self, _, _ := d.Get(starlark.String("self"))
	assert.Same(t, l, self)
	r := out[1].(*Struct)
	me, _ := r.Attr("me")
	assert.Same(t, r, me)
	in := out[2].(*starlark.List)
	back, _ := in.Index(0).(*starlarkstruct.Struct).Attr("l")
	assert.Same(t, in, back)
}

// testResolver stands in for the spec in a fresh process.
type testResolver struct {
	methods     map[string]*starlark.Function
	roleMethods map[string]*starlark.Builtin
	globals     starlark.StringDict
}

func (r *testResolver) RoleMethods(string) (map[string]*starlark.Function, map[string]*starlark.Builtin, error) {
	return r.methods, r.roleMethods, nil
}

func (r *testResolver) Global(name string) (starlark.Value, bool) {
	v, ok := r.globals[name]
	return v, ok
}

func TestStateCodecRoles(t *testing.T) {
	methods := map[string]*starlark.Function{"helper": {}}
	roleMethods := map[string]*starlark.Builtin{"Send": starlark.NewBuiltin("Send", nil)}
	mk := func(i int64) *Role {
		return &Role{
			ID: NewSymmetricValue("Node", i), Name: "Node", Symmetric: true,
			Params:  FromStringDict(starlark.String("params"), starlark.StringDict{"n": starlark.MakeInt64(i)}),
			Fields:  FromStringDict(starlark.String("fields"), starlark.StringDict{"log": starlark.NewList(nil)}),
			Methods: methods, RoleMethods: roleMethods,
			InitValues: FromStringDict(starlark.String("init_values"), starlark.StringDict{"log": starlark.NewList(nil)}),
		}
	}
	r0, r1 := mk(0), mk(1)
	require.NoError(t, r0.Fields.SetField("peer", r1))
	// A role whose Init hasn't run yet has no methods.
	r2 := mk(2)
	r2.Methods = map[string]*starlark.Function{}
	ch := &Channel{Id: 0, ordering: "ordered"}
	stub := NewRoleStub(r1, ch)
	global := starlark.NewBuiltin("helper_fn", nil)
	lambda := starlark.NewBuiltin("not_a_global", nil)
	resolver := &testResolver{methods: methods, roleMethods: roleMethods, globals: starlark.StringDict{"helper_fn": global}}

	ctx := NewCodecContext()
	ctx.SetRoleResolver(resolver)
	state := starlark.NewList([]starlark.Value{r0, r1, stub, global, lambda, r2})
	out, data := roundTripValues(t, ctx, state)
	l := out[0].(*starlark.List)
	g0, g1, g2 := l.Index(0).(*Role), l.Index(1).(*Role), l.Index(5).(*Role)
	peer, _ := g0.Fields.Attr("peer")
	assert.Same(t, g1, peer)
	assert.Same(t, g1, l.Index(2).(*RoleStub).Role)
	assert.Same(t, ch, l.Index(2).(*RoleStub).Channel)
	assert.Same(t, global, l.Index(3))
	assert.Same(t, lambda, l.Index(4), "a non-global builtin goes through the opaque table")
	assert.Equal(t, r0.String(), g0.String())
	assert.True(t, g0.IsSymmetric())
	h0, _ := r0.Hash()
	gh0, _ := g0.Hash()
	assert.Equal(t, h0, gh0)
	// Methods are shared per role type; a role before Init gets its own
	// empty map, which Init fills.
	assert.Equal(t, methods, g0.Methods)
	assert.Empty(t, g2.Methods)
	assert.Equal(t, roleMethods, g1.RoleMethods)
	// Equal init values decode to one shared struct, like readResolve.
	assert.Same(t, g0.InitValues, g1.InitValues)
	assert.Equal(t, r0.InitValues.String(), g0.InitValues.String())
	t.Logf("three roles + stub + builtins: %d bytes", len(data))

	// Setting an init value replaces the struct, so the others keep theirs.
	g0.SetInitValue("log", starlark.MakeInt(1))
	assert.NotSame(t, g0.InitValues, g1.InitValues)
	assert.Equal(t, "init_values(log = [])", g1.InitValues.String())

	// A fresh context (another process running the same spec) decodes the
	// roles and the global from the resolver alone.
	w := NewStateWriter(ctx)
	defer w.Release()
	require.NoError(t, w.WriteValue(starlark.NewList([]starlark.Value{r0, r1, global})))
	fresh := NewCodecContext()
	fresh.SetRoleResolver(resolver)
	v, err := NewStateReader(fresh, w.Bytes()).ReadValue()
	require.NoError(t, err)
	fl := v.(*starlark.List)
	assert.Equal(t, r0.String(), fl.Index(0).String())
	assert.Equal(t, methods, fl.Index(0).(*Role).Methods)
	assert.Same(t, global, fl.Index(2))

	// Without a resolver, a fresh context can't resolve the methods.
	_, err = NewStateReader(NewCodecContext(), w.Bytes()).ReadValue()
	assert.Error(t, err)
}

func TestStateCodecUnsupportedAndCorrupt(t *testing.T) {
	ctx := NewCodecContext()
	w := NewStateWriter(ctx)
	defer w.Release()
	require.NoError(t, w.WriteValue(starlark.NewList([]starlark.Value{starlark.String("abc"), starlark.MakeInt(300)})))
	data := append([]byte(nil), w.Bytes()...)

	for i := 0; i < len(data); i++ {
		r := NewStateReader(ctx, data[:i])
		_, err := r.ReadValue()
		if err == nil {
			err = r.Done()
		}
		assert.True(t, errors.Is(err, ErrCorruptState), "truncated at %d: %v", i, err)
	}
	r := NewStateReader(ctx, []byte{0xFE})
	_, err := r.ReadValue()
	assert.ErrorIs(t, err, ErrCorruptState)
	r = NewStateReader(ctx, []byte{vtRef, 5})
	_, err = r.ReadValue()
	assert.ErrorIs(t, err, ErrCorruptState)
	r = NewStateReader(ctx, []byte{vtList, 0xFF, 0xFF, 0xFF, 0x0F})
	_, err = r.ReadValue()
	assert.ErrorIs(t, err, ErrCorruptState)
}
