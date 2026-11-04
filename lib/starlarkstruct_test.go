package lib

import (
	"testing"

	"go.starlark.net/starlark"
)

// A record cycle (a.peer = b, b.peer = a) must not send Freeze into
// infinite recursion. Before the frozen guard this overflowed the Go stack
// whenever a spec's globals held such a cycle.
func TestStructFreezeCycle(t *testing.T) {
	a := FromStringDict(starlark.String("record"), starlark.StringDict{"name": starlark.String("a")})
	b := FromStringDict(starlark.String("record"), starlark.StringDict{"name": starlark.String("b"), "peer": a})
	if err := a.SetField("peer", b); err != nil {
		t.Fatal(err)
	}

	// Recursion without the guard overflows the Go stack and aborts the
	// test binary, which is the failure signal here.
	a.Freeze()

	if !a.frozen || !b.frozen {
		t.Fatalf("expected both structs frozen, got a=%v b=%v", a.frozen, b.frozen)
	}
}

// The smallest cycle: a record that refers to itself.
func TestStructFreezeSelfCycle(t *testing.T) {
	a := FromStringDict(starlark.String("record"), nil)
	if err := a.SetField("self", a); err != nil {
		t.Fatal(err)
	}
	a.Freeze()
	if !a.frozen {
		t.Fatal("expected struct frozen")
	}
}

// Freezing must still reach nested mutable values, and a second Freeze is a
// no-op rather than a second walk.
func TestStructFreezeDeep(t *testing.T) {
	inner := starlark.NewList([]starlark.Value{starlark.MakeInt(1)})
	s := FromStringDict(starlark.String("record"), starlark.StringDict{"xs": inner})
	s.Freeze()
	if err := inner.Append(starlark.MakeInt(2)); err == nil {
		t.Fatal("expected append to frozen list to fail")
	}
	s.Freeze() // idempotent
}
