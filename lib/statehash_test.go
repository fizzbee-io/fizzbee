package lib

import (
	"bytes"
	"math/big"
	"testing"

	"go.starlark.net/starlark"
)

func encode(v starlark.Value) []byte {
	e := NewStateEncoder()
	defer e.Release()
	e.WriteValue(v, 0)
	return append([]byte(nil), e.Bytes()...)
}

func mustJSON(t *testing.T, v starlark.Value) []byte {
	b, err := MarshalJSONStarlarkValue(v, 0)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func mklist(vs ...starlark.Value) *starlark.List { return starlark.NewList(vs) }

func mkset(vs ...starlark.Value) *starlark.Set {
	s := starlark.NewSet(len(vs))
	for _, v := range vs {
		if err := s.Insert(v); err != nil {
			panic(err)
		}
	}
	return s
}

func mkdict(kv ...starlark.Value) *starlark.Dict {
	d := starlark.NewDict(len(kv) / 2)
	for i := 0; i < len(kv); i += 2 {
		if err := d.SetKey(kv[i], kv[i+1]); err != nil {
			panic(err)
		}
	}
	return d
}

func mkrecord(kv ...interface{}) *Struct {
	sd := starlark.StringDict{}
	for i := 0; i < len(kv); i += 2 {
		sd[kv[i].(string)] = kv[i+1].(starlark.Value)
	}
	return FromStringDict(starlark.String("record"), sd)
}

func bigInt(s string) *big.Int {
	n, _ := new(big.Int).SetString(s, 10)
	return n
}

func i(n int64) starlark.Int       { return starlark.MakeInt64(n) }
func str(s string) starlark.String { return starlark.String(s) }

// family tells apart the one place the binary encoding is finer than the
// JSON text: ordered sequences vs sets.
func family(v starlark.Value) string {
	switch v.Type() {
	case "set", "genericset", "bag":
		return "set"
	}
	return "other"
}

// TestStateEncoderMatchesJSONPartition checks that two values get the same
// binary encoding exactly when they had the same JSON text, except for the
// documented list-vs-set case where the encoding is finer.
func TestStateEncoderMatchesJSONPartition(t *testing.T) {
	values := []starlark.Value{
		starlark.None, starlark.True, starlark.False,
		str(""), str("1"), str("true"), str("null"), str("a\"b"), str("é"), str("TOO_DEEP"),
		starlark.Bytes("1"), starlark.Bytes("x"),
		i(0), i(1), i(-1), i(1 << 40), starlark.MakeBigInt(bigInt("123456789012345678901234567890")),
		starlark.Float(1.0), starlark.Float(0), starlark.Float(-0.0), starlark.Float(1.5),
		mklist(), mklist(i(1)), mklist(i(1), i(2)), mklist(i(2), i(1)), mklist(str("1")), mklist(mklist(i(1))),
		starlark.Tuple{}, starlark.Tuple{i(1), i(2)}, starlark.Tuple{i(2), i(1)},
		mkset(), mkset(i(1), i(2)), mkset(i(2), i(1)), mkset(str("a"), str("b")),
		mkdict(), mkdict(str("a"), i(1)), mkdict(i(1), i(1)), mkdict(str("1"), i(1)),
		mkdict(str("aa"), i(1), str("b"), i(2)), mkdict(str("b"), i(2), str("aa"), i(1)),
		mkdict(str("a"), mklist(i(1))),
		mkrecord(), mkrecord("a", i(1)), mkrecord("aa", i(1), "b", i(2)), mkrecord("a", str("1")),
		NewModelValue("Customer#", 1), NewModelValue("Customer#", 2), str("Customer#1"),
		NewSymmetricValue("sv", 3), str("sv3"),
	}
	for a := 0; a < len(values); a++ {
		for b := 0; b < len(values); b++ {
			jsonEq := bytes.Equal(mustJSON(t, values[a]), mustJSON(t, values[b]))
			binEq := bytes.Equal(encode(values[a]), encode(values[b]))
			if binEq && !jsonEq {
				t.Errorf("binary encoding merges %s and %s but JSON kept them apart", values[a], values[b])
			}
			if jsonEq && !binEq && family(values[a]) == family(values[b]) {
				t.Errorf("JSON merges %s and %s but binary encoding keeps them apart", values[a], values[b])
			}
		}
	}
}

func TestStateEncoderOrderIndependent(t *testing.T) {
	a := mkset(i(1), str("x"), i(300), starlark.Tuple{i(1)})
	b := mkset(starlark.Tuple{i(1)}, i(300), str("x"), i(1))
	if !bytes.Equal(encode(a), encode(b)) {
		t.Errorf("set encoding depends on insertion order")
	}
	d1 := mkdict(str("k1"), i(1), str("k2"), mkset(i(1), i(2)), i(3), str("v"))
	d2 := mkdict(i(3), str("v"), str("k2"), mkset(i(2), i(1)), str("k1"), i(1))
	if !bytes.Equal(encode(d1), encode(d2)) {
		t.Errorf("dict encoding depends on insertion order")
	}
	sd1 := starlark.StringDict{"a": i(1), "b": str("x"), `q"\\`: i(2)}
	sd2 := starlark.StringDict{`q"\\`: i(2), "b": str("x"), "a": i(1)}
	e1, e2 := NewStateEncoder(), NewStateEncoder()
	e1.WriteStringDict(sd1)
	e2.WriteStringDict(sd2)
	if !bytes.Equal(e1.Bytes(), e2.Bytes()) {
		t.Errorf("string dict encoding depends on map order")
	}
	// A StringDict and a dict with the same string keys and values encode
	// alike, as their JSON did, including keys that need escaping.
	if !bytes.Equal(e1.Bytes(), encode(mkdict(str("a"), i(1), str("b"), str("x"), str(`q"\\`), i(2)))) {
		t.Errorf("string dict and dict with the same entries encode differently")
	}
	e1.Release()
	e2.Release()
}

func TestStateEncoderRole(t *testing.T) {
	mk := func(x int64) *Role {
		return &Role{
			ID:     NewModelValue("R#", 1),
			Name:   "R",
			Params: mkrecord("p", i(1)),
			Fields: mkrecord("f", i(x)),
		}
	}
	enc := func(r *Role) []byte {
		e := NewStateEncoder()
		defer e.Release()
		e.WriteRole(r)
		return append([]byte(nil), e.Bytes()...)
	}
	if !bytes.Equal(enc(mk(1)), enc(mk(1))) {
		t.Errorf("same role encodes differently")
	}
	if bytes.Equal(enc(mk(1)), enc(mk(2))) {
		t.Errorf("roles with different fields encode alike")
	}
}
