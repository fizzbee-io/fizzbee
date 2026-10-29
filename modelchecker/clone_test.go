package modelchecker

import (
	"os"
	"path/filepath"
	"testing"

	ast "fizz/proto"
	"github.com/fizzbee-io/fizzbee/lib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
)

func testRole(name string, ref int64) *lib.Role {
	return &lib.Role{
		ID:         lib.NewModelValue(name, ref),
		Name:       name,
		Params:     lib.FromStringDict(starlark.String("params"), starlark.StringDict{}),
		Fields:     lib.FromStringDict(starlark.String("fields"), starlark.StringDict{"n": starlark.MakeInt64(ref)}),
		InitValues: lib.FromStringDict(starlark.String("init_values"), starlark.StringDict{}),
	}
}

// Bags keep their elements in string order. The clone fast path copies the
// source order without re-sorting, which is only right if cloning keeps
// every element's string form. Check that for the value kinds a bag can
// hold: ints, strings, tuples, records, roles and symmetric values.
func TestCloneBagKeepsSortedOrder(t *testing.T) {
	role := testRole("Node", 2)
	sv := lib.NewSymmetricValue("S", 1)
	rec := lib.FromStringDict(starlark.String("record"), starlark.StringDict{"b": starlark.MakeInt(2), "a": starlark.String("x")})
	elems := []starlark.Value{
		starlark.MakeInt(10), starlark.String("zeta"), starlark.String("alpha"),
		starlark.Tuple{starlark.MakeInt(1), starlark.String("t")}, rec, role, sv, starlark.MakeInt(-3),
	}
	bag := lib.NewBag(elems)

	cloned, err := deepCloneStarlarkValue(bag, map[starlark.Value]starlark.Value{role: role})
	require.NoError(t, err)
	cb := cloned.(*lib.Bag)
	require.Equal(t, bag.Len(), cb.Len())

	src, dst := bag.Iterate(), cb.Iterate()
	defer src.Done()
	defer dst.Done()
	var a, b starlark.Value
	prev := ""
	for src.Next(&a) {
		require.True(t, dst.Next(&b))
		assert.Equal(t, a.String(), b.String())
		assert.LessOrEqual(t, prev, b.String(), "clone must stay sorted")
		prev = b.String()
	}
	assert.Equal(t, bag.String(), cb.String())
}

// Set and map clones append the cloned keys without re-checking them for
// duplicates. That is sound only if cloning, with or without a symmetry
// renaming, keeps distinct keys distinct. Check with scalar, compound and
// identity-bearing keys under a renaming that swaps two symmetric values.
func TestCloneSetAndMapKeysStayDistinct(t *testing.T) {
	s0, s1 := lib.NewSymmetricValue("S", 0), lib.NewSymmetricValue("S", 1)
	perms := map[*lib.SymmetricValue][]*lib.SymmetricValue{s0: {s1}, s1: {s0}}
	role1, role2 := testRole("Node", 1), testRole("Node", 2)

	keys := []starlark.Value{
		starlark.MakeInt(1), starlark.String("1"),
		starlark.Tuple{s0, starlark.MakeInt(1)}, starlark.Tuple{s1, starlark.MakeInt(1)},
		s0, s1, role1, role2,
		lib.FromStringDict(starlark.String("record"), starlark.StringDict{"k": s0}),
		lib.FromStringDict(starlark.String("record"), starlark.StringDict{"k": s1}),
	}
	set := lib.NewGenericSet()
	m := lib.NewGenericMap()
	for i, k := range keys {
		require.NoError(t, set.Insert(k))
		require.NoError(t, m.SetKey(k, starlark.MakeInt(i)))
	}
	require.Equal(t, len(keys), set.Len())
	require.Equal(t, len(keys), m.Len())

	for _, p := range []map[*lib.SymmetricValue][]*lib.SymmetricValue{nil, perms} {
		refs := map[starlark.Value]starlark.Value{role1: role1, role2: role2}
		cs, err := deepCloneStarlarkValueWithPermutations(set, refs, p, 0)
		require.NoError(t, err)
		cm, err := deepCloneStarlarkValueWithPermutations(m, refs, p, 0)
		require.NoError(t, err)

		// Re-inserting every cloned key into a fresh, checking set must
		// not collapse any of them.
		check := lib.NewGenericSet()
		it := cs.(*lib.GenericSet).Iterate()
		var k starlark.Value
		for it.Next(&k) {
			require.NoError(t, check.Insert(k))
		}
		it.Done()
		assert.Equal(t, len(keys), check.Len())

		cmap := cm.(*lib.GenericMap)
		assert.Equal(t, len(keys), cmap.Len())
		seen := map[int]bool{}
		for _, item := range cmap.Items() {
			v, _ := starlark.AsInt32(item[1])
			assert.False(t, seen[v], "value %d reached twice", v)
			seen[v] = true
		}
	}
}

// After findVisitedSymmetric on a spec without symmetry, the node's cached
// hash is the canonical one (roles sorted by name and ref), the role slice
// is back in its original order, and the thread hashes are populated, so
// Node.Duplicate can build its thread map without rehashing.
func TestCanonicalHashWithoutSymmetry(t *testing.T) {
	file, err := parseAstFromString(ActionsWithMultipleBlocks)
	require.Nil(t, err)
	files := []*ast.File{file}

	build := func(roles ...*lib.Role) *Process {
		p := NewProcess("", files, nil)
		p.Heap.state = starlark.StringDict{"a": starlark.MakeInt(1)}
		p.NewThread()
		p.currentThread().currentFrame().pc = "Actions[0]"
		p.Roles = roles
		return p
	}
	b1, b2, c1 := testRole("B", 1), testRole("B", 2), testRole("A", 1)
	unsorted := build(b2, c1, b1)
	sorted := build(c1, b1, b2)

	plain := unsorted.HashCode()
	unsorted.CachedHashCode = ""
	canonical := unsorted.canonicalHashWithoutSymmetry()

	assert.NotEqual(t, plain, canonical, "role order is part of the plain hash")
	assert.Equal(t, sorted.HashCode(), canonical, "canonical hash is the sorted-order hash")
	assert.Equal(t, []*lib.Role{b2, c1, b1}, unsorted.Roles, "role order restored")
	assert.Equal(t, canonical, unsorted.CachedHashCode, "cache holds the visited key")
	assert.Equal(t, canonical, unsorted.HashCode())
	assert.Len(t, unsorted.CachedThreadHashes, 1)
	assert.Equal(t, sorted.CachedThreadHashes, unsorted.CachedThreadHashes, "thread hashes do not depend on role order")
}

// The environment built from a statement's names must resolve a colliding
// name exactly as the full environment did: builtins < globals < state < self < locals, with
// deepcopy, resolve_role, modules, role constructors and Channel on top.
func TestEnvironmentShadowing(t *testing.T) {
	file, err := parseAstFromString(ActionsWithMultipleBlocks)
	require.Nil(t, err)
	files := []*ast.File{file}
	p := NewProcess("", files, nil)
	p.Heap.globals = starlark.StringDict{
		"len":    starlark.String("global-len"),
		"shared": starlark.String("global"),
		"gonly":  starlark.String("global"),
		"random": starlark.String("global-random"),
	}
	p.Heap.state = starlark.StringDict{
		"shared": starlark.String("state"),
		"sonly":  starlark.String("state"),
		"self":   starlark.String("state-self"),
	}
	p.Modules = map[string]starlark.Value{"random": starlark.String("module-random")}
	p.NewThread()
	frame := p.currentThread().currentFrame()
	frame.scope = &Scope{vars: starlark.StringDict{"scoped": starlark.String("scope")}}
	frame.vars = starlark.StringDict{"shared": starlark.String("local"), "deepcopy": starlark.String("local-deepcopy")}
	role := testRole("Node", 1)
	frame.obj = role

	names := []string{"len", "shared", "gonly", "sonly", "self", "random", "deepcopy",
		"resolve_role", "Channel", "scoped", "bag", "undefined_name"}
	for round := 0; round < 2; round++ { // second round is served from the cache
		env := p.envForNames(names)
		assert.Equal(t, starlark.String("global-len"), env["len"], "global shadows builtin")
		assert.Equal(t, starlark.String("local"), env["shared"], "local shadows state and global")
		assert.Equal(t, starlark.String("global"), env["gonly"])
		assert.Equal(t, starlark.String("state"), env["sonly"])
		assert.Equal(t, role, env["self"], "self shadows a state variable of that name")
		assert.Equal(t, starlark.String("module-random"), env["random"], "module wins over a global")
		assert.Equal(t, "deepcopy", env["deepcopy"].(*starlark.Builtin).Name(), "deepcopy builtin wins over a local")
		assert.Equal(t, "resolve_role", env["resolve_role"].(*starlark.Builtin).Name())
		assert.Equal(t, "Channel", env["Channel"].(*starlark.Builtin).Name())
		assert.Equal(t, starlark.String("scope"), env["scoped"])
		assert.NotNil(t, env["bag"], "untouched builtins still present")
		assert.NotContains(t, env, "undefined_name")
	}

	// Globals are read in place, so replacing them (what preinit does) is
	// seen at once.
	p.Heap.globals = starlark.StringDict{"len": starlark.String("new-len")}
	assert.Equal(t, starlark.String("new-len"), p.envForNames([]string{"len"})["len"])

	// The constructors no longer bind a process, so a fork shares them.
	fork := p.Fork()
	assert.Same(t, p.envForNames([]string{"Channel"})["Channel"], fork.envForNames([]string{"Channel"})["Channel"])
}

// Run a spec with ordinary roles through the real dedup path and check the
// invariant canonicalHashWithoutSymmetry leaves behind: every reachable
// node's HashCode is its visited key, the thread hashes are populated so
// the duplicate links carry a thread map, the role slice is back in
// creation order, and the communication-diagram traversal (which compares
// node hashes to find self-loops) runs over the result.
func TestNoSymmetryDedupInvariants(t *testing.T) {
	base := "."
	if dir := os.Getenv("RUNFILES_DIR"); dir != "" {
		base = filepath.Join(dir, "_main", "modelchecker")
	}
	file, err := readAstFromFile(filepath.Join(base, "testdata", "role_dedup", "RoleDedup.json"))
	require.Nil(t, err)
	p1 := NewProcessor([]*ast.File{file}, &ast.StateSpaceOptions{
		Options: &ast.Options{MaxActions: 100, MaxConcurrentActions: 2},
	}, false, 0, "", "", false, nil, nil, "")
	root, _, _ := p1.Start()
	require.NotNil(t, root)
	require.False(t, p1.specUsesSymmetry(root.Process), "the spec has no symmetry, so the fast path must be the one under test")
	// 3 x 3 balances.
	assert.Equal(t, 9, len(p1.visited))

	dupLinks := 0
	for key, node := range p1.visited {
		require.NotNil(t, node.Process, "visited nodes keep their state")
		assert.Equal(t, key, node.Process.CachedHashCode, "cache holds the visited key")
		assert.Equal(t, key, node.HashCode())
		assert.Len(t, node.Process.CachedThreadHashes, len(node.Process.Threads))
		require.Len(t, node.Process.Roles, 2)
		assert.Equal(t, "Zed", node.Process.Roles[0].Name, "creation order restored after the canonical sort")
		assert.Equal(t, "Account", node.Process.Roles[1].Name)
		for _, link := range node.Outbound {
			target := link.Node
			if target.Name != "yield" && target.Name != "init" {
				continue
			}
			assert.Same(t, target, p1.visited[target.HashCode()], "every reachable yield node hashes to its own visited key")
			if len(target.Inbound) > 1 {
				// A node with several inbound links was the target of at
				// least one dedup hit; the merged link carries the thread
				// index map Node.Duplicate built from the cached thread hashes.
				if link.ThreadsMap != nil {
					dupLinks++
					live := 0
					for _, h := range target.Process.CachedThreadHashes {
						if h != "" {
							live++
						}
					}
					assert.Len(t, link.ThreadsMap, live, "one entry per live thread")
				}
			}
		}
	}
	assert.Greater(t, dupLinks, 0, "the run must have exercised Node.Duplicate")

	nodes, _, _, _ := traverseBFS(root, 100)
	assert.Equal(t, 9, len(nodes))
}
