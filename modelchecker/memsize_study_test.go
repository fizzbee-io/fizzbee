package modelchecker

// Memory-footprint study for Starlark values as FizzBee stores them.
// Run with: go test ./modelchecker -run TestMemsizeStudy -v -count=1

import (
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"unsafe"

	ast "fizz/proto"
	"github.com/fizzbee-io/fizzbee/lib"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

const studyN = 20000

type memResult struct {
	bytes  float64 // retained heap bytes per value (excludes the 16B interface slot)
	allocs float64 // heap objects per value
}

var sink []starlark.Value

// measure retains studyN values produced by mk and reports the retained heap
// per value. The []Value holding them is allocated before the baseline, so
// the 16-byte interface slot each value occupies in its parent is not counted.
func measure(mk func(i int) starlark.Value) memResult {
	sink = make([]starlark.Value, studyN)
	runtime.GC()
	runtime.GC()
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	for i := 0; i < studyN; i++ {
		sink[i] = mk(i)
	}
	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&b)
	r := memResult{
		bytes:  float64(int64(b.HeapAlloc)-int64(a.HeapAlloc)) / studyN,
		allocs: float64(b.Mallocs-a.Mallocs) / studyN,
	}
	runtime.KeepAlive(sink)
	sink = nil
	return r
}

// measureClone retains studyN deep clones of src, the way the checker copies
// state when it forks a node. Immutable parts that the clone shares with src
// cost nothing here, which is what we want.
func measureClone(src starlark.Value) memResult {
	return measure(func(i int) starlark.Value {
		v, err := deepCloneStarlarkValue(src, make(map[starlark.Value]starlark.Value))
		if err != nil {
			panic(err)
		}
		return v
	})
}

type row struct {
	name    string
	sizeof  uintptr
	payload int
	fresh   memResult
	cloned  *memResult
}

func ints(n, base int) []starlark.Value {
	out := make([]starlark.Value, n)
	for i := range out {
		out[i] = starlark.MakeInt(base + i)
	}
	return out
}

func TestMemsizeStudy(t *testing.T) {
	if os.Getenv("FIZZ_MEMSTUDY") == "" {
		t.Skip("set FIZZ_MEMSTUDY=1 to run the memory study")
	}
	var rows []row
	add := func(name string, sizeof uintptr, payload int, mk func(i int) starlark.Value, clone bool) {
		r := row{name: name, sizeof: sizeof, payload: payload, fresh: measure(mk)}
		if clone {
			c := measureClone(mk(7))
			r.cloned = &c
		}
		rows = append(rows, r)
	}

	// --- scalars ---
	add("None", unsafe.Sizeof(starlark.None), 0, func(i int) starlark.Value { return starlark.None }, false)
	add("Bool", unsafe.Sizeof(starlark.True), 1, func(i int) starlark.Value { return starlark.Bool(i%2 == 0) }, false)
	add("Int small (int32 range)", unsafe.Sizeof(starlark.MakeInt(1)), 8, func(i int) starlark.Value { return starlark.MakeInt(i + 1000) }, false)
	add("Int big (> int32)", unsafe.Sizeof(starlark.MakeInt(1)), 8, func(i int) starlark.Value { return starlark.MakeInt64(int64(math.MaxInt32) + 10 + int64(i)) }, false)
	add("Int huge (2^100)", unsafe.Sizeof(starlark.MakeInt(1)), 13, func(i int) starlark.Value {
		x := new(big.Int).Lsh(big.NewInt(1), 100)
		return starlark.MakeBigInt(x.Add(x, big.NewInt(int64(i))))
	}, false)
	add("Float", unsafe.Sizeof(starlark.Float(0)), 8, func(i int) starlark.Value { return starlark.Float(float64(i) + 0.5) }, false)
	// Strings: the Go string data already exists (an interned literal); this
	// is only the cost of boxing it as a Starlark value, e.g. an enum member
	// or a constant assigned to a field.
	lit := strings.Repeat("x", 8)
	add("String 8B, shared data (box only)", unsafe.Sizeof(starlark.String("")), 0, func(i int) starlark.Value { return starlark.String(lit) }, true)
	add("String 8B, fresh data", unsafe.Sizeof(starlark.String("")), 8, func(i int) starlark.Value { return starlark.String(fmt.Sprintf("s%07d", i)) }, true)
	add("String 32B, fresh data", unsafe.Sizeof(starlark.String("")), 32, func(i int) starlark.Value { return starlark.String(fmt.Sprintf("%032d", i)) }, true)

	// --- containers of small ints ---
	for _, n := range []int{0, 1, 4, 16, 64} {
		n := n
		add(fmt.Sprintf("List[%d] of int", n), unsafe.Sizeof(starlark.List{}), 8*n, func(i int) starlark.Value { return starlark.NewList(ints(n, i)) }, true)
	}
	for _, n := range []int{0, 1, 4, 16, 64} {
		n := n
		add(fmt.Sprintf("Tuple[%d] of int", n), unsafe.Sizeof(starlark.Tuple{}), 8*n, func(i int) starlark.Value { return starlark.Tuple(ints(n, i)) }, true)
	}
	for _, n := range []int{0, 1, 4, 8, 9, 16, 64} {
		n := n
		add(fmt.Sprintf("Dict[%d] int->int", n), unsafe.Sizeof(starlark.Dict{}), 16*n, func(i int) starlark.Value {
			d := starlark.NewDict(n)
			for j := 0; j < n; j++ {
				_ = d.SetKey(starlark.MakeInt(j), starlark.MakeInt(i+j))
			}
			return d
		}, true)
	}
	for _, n := range []int{0, 1, 4, 16, 64} {
		n := n
		add(fmt.Sprintf("Set[%d] of int", n), unsafe.Sizeof(starlark.Set{}), 8*n, func(i int) starlark.Value {
			s := starlark.NewSet(n)
			for _, v := range ints(n, i) {
				_ = s.Insert(v)
			}
			return s
		}, true)
	}

	// --- records / structs ---
	for _, n := range []int{1, 4, 16} {
		n := n
		names := make([]string, n)
		for j := range names {
			names[j] = fmt.Sprintf("field%02d", j)
		}
		add(fmt.Sprintf("record (lib.Struct) %d int fields", n), unsafe.Sizeof(lib.Struct{}), 8*n, func(i int) starlark.Value {
			kw := make([]starlark.Tuple, n)
			for j := range kw {
				kw[j] = starlark.Tuple{starlark.String(names[j]), starlark.MakeInt(i + j)}
			}
			return lib.FromKeywords(lib.Default, kw)
		}, true)
		add(fmt.Sprintf("struct (starlarkstruct) %d int fields", n), unsafe.Sizeof(starlarkstruct.Struct{}), 8*n, func(i int) starlark.Value {
			kw := make([]starlark.Tuple, n)
			for j := range kw {
				kw[j] = starlark.Tuple{starlark.String(names[j]), starlark.MakeInt(i + j)}
			}
			return starlarkstruct.FromKeywords(starlarkstruct.Default, kw)
		}, false)
	}
	add("enum member (String)", unsafe.Sizeof(starlark.String("")), 1, func(i int) starlark.Value { return starlark.String("WORKING") }, false)

	// --- FizzBee collection types ---
	for _, n := range []int{0, 1, 4, 16} {
		n := n
		add(fmt.Sprintf("genericmap[%d] int->int", n), unsafe.Sizeof(lib.GenericMap{}), 16*n, func(i int) starlark.Value {
			kv := make([]starlark.Tuple, n)
			for j := range kv {
				kv[j] = starlark.Tuple{starlark.MakeInt(j), starlark.MakeInt(i + j)}
			}
			return lib.GenericMapFromEntries(kv)
		}, true)
		add(fmt.Sprintf("genericset[%d] of int", n), unsafe.Sizeof(lib.GenericSet{}), 8*n, func(i int) starlark.Value {
			return lib.GenericSetFromDistinct(ints(n, i))
		}, true)
		add(fmt.Sprintf("bag[%d] of int", n), unsafe.Sizeof(lib.Bag{}), 8*n, func(i int) starlark.Value {
			return lib.BagFromSortedSlice(ints(n, i))
		}, true)
	}

	// --- identity types ---
	add("model_value", unsafe.Sizeof(lib.ModelValue{}), 8, func(i int) starlark.Value { return lib.NewModelValue("node", int64(i)) }, true)
	add("symmetric_value", unsafe.Sizeof(lib.SymmetricValue{}), 8, func(i int) starlark.Value { return lib.NewSymmetricValue("node", int64(i%5)) }, true)

	methods := map[string]*starlark.Function{}
	roleMethods := map[string]*starlark.Builtin{}
	for _, n := range []int{0, 2, 8} {
		n := n
		names := make([]string, n)
		for j := range names {
			names[j] = fmt.Sprintf("f%02d", j)
		}
		mkRole := func(i int) starlark.Value {
			kw := make([]starlark.Tuple, n)
			for j := range kw {
				kw[j] = starlark.Tuple{starlark.String(names[j]), starlark.MakeInt(i + j)}
			}
			return &lib.Role{
				ID:          lib.NewModelValue("Node", int64(i)),
				Name:        "Node",
				Params:      lib.FromKeywords(lib.Default, nil),
				Fields:      lib.FromKeywords(lib.Default, kw),
				Methods:     methods,
				RoleMethods: roleMethods,
				InitValues:  nil,
			}
		}
		add(fmt.Sprintf("role, %d int fields, 0 params", n), unsafe.Sizeof(lib.Role{}), 8+8*n, mkRole, true)
	}
	ch := &lib.Channel{Id: 1}
	add("RoleStub", unsafe.Sizeof(lib.RoleStub{}), 8, func(i int) starlark.Value {
		return lib.NewRoleStub(&lib.Role{ID: lib.NewModelValue("Node", 1), Name: "Node",
			Params: lib.FromKeywords(lib.Default, nil), Fields: lib.FromKeywords(lib.Default, nil),
			Methods: methods, RoleMethods: roleMethods}, ch)
	}, false)

	// --- nested, realistic shapes ---
	add("list[8] of record{a:int,b:str}", 0, 8*(8+1), func(i int) starlark.Value {
		elems := make([]starlark.Value, 8)
		for j := range elems {
			elems[j] = lib.FromKeywords(lib.Default, []starlark.Tuple{
				{starlark.String("a"), starlark.MakeInt(i + j)},
				{starlark.String("b"), starlark.String("IDLE")},
			})
		}
		return starlark.NewList(elems)
	}, true)
	add("dict[4] str->set[2] of int", 0, 4*(1+16), func(i int) starlark.Value {
		d := starlark.NewDict(4)
		for j, k := range []string{"a", "b", "c", "d"} {
			s := starlark.NewSet(2)
			_ = s.Insert(starlark.MakeInt(i + j))
			_ = s.Insert(starlark.MakeInt(i + j + 1))
			_ = d.SetKey(starlark.String(k), s)
		}
		return d
	}, true)

	var sb strings.Builder
	fmt.Fprintf(&sb, "| Value | unsafe.Sizeof | payload B | heap B (fresh) | allocs | slot+heap B | overhead x | heap B (deep clone) | clone allocs |\n")
	fmt.Fprintf(&sb, "|---|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, r := range rows {
		total := r.fresh.bytes + 16
		ratio := "-"
		if r.payload > 0 {
			ratio = fmt.Sprintf("%.1f", total/float64(r.payload))
		}
		cb, ca := "-", "-"
		if r.cloned != nil {
			cb = fmt.Sprintf("%.0f", r.cloned.bytes)
			ca = fmt.Sprintf("%.1f", r.cloned.allocs)
		}
		fmt.Fprintf(&sb, "| %s | %d | %d | %.0f | %.1f | %.0f | %s | %s | %s |\n",
			r.name, r.sizeof, r.payload, r.fresh.bytes, r.fresh.allocs, total, ratio, cb, ca)
	}
	fmt.Println(sb.String())
	if out := os.Getenv("FIZZ_MEMSTUDY_OUT"); out != "" {
		_ = os.WriteFile(out, []byte(sb.String()), 0o644)
	}
}

// payloadOf walks a value and returns its information content in bytes
// (8 per int/float/id, 1 per bool, len for strings; container and record
// structure, field names and role names count as zero) and per-type counts.
func payloadOf(v starlark.Value, counts map[string]int, seen map[starlark.Value]bool) int {
	if v == nil {
		return 0
	}
	t := v.Type()
	counts[t]++
	switch x := v.(type) {
	case starlark.NoneType:
		return 0
	case starlark.Bool:
		return 1
	case starlark.Int:
		if _, ok := x.Int64(); ok {
			return 8
		}
		return (x.BigInt().BitLen() + 7) / 8
	case starlark.Float:
		return 8
	case starlark.String:
		return len(x)
	case *lib.ModelValue, *lib.SymmetricValue:
		return 8
	case *lib.Role:
		if seen[x] {
			return 0
		}
		seen[x] = true
		return 8 + payloadOf(x.Params, counts, seen) + payloadOf(x.Fields, counts, seen)
	case *lib.Struct:
		n := 0
		x.EachField(func(_ string, fv starlark.Value) { n += payloadOf(fv, counts, seen) })
		return n
	case *starlarkstruct.Struct:
		n := 0
		for _, name := range x.AttrNames() {
			fv, _ := x.Attr(name)
			n += payloadOf(fv, counts, seen)
		}
		return n
	case *starlark.Dict:
		n := 0
		for _, it := range x.Items() {
			n += payloadOf(it[0], counts, seen) + payloadOf(it[1], counts, seen)
		}
		return n
	case *lib.GenericMap:
		n := 0
		for _, it := range x.Items() {
			n += payloadOf(it[0], counts, seen) + payloadOf(it[1], counts, seen)
		}
		return n
	case starlark.Iterable:
		n := 0
		it := x.Iterate()
		defer it.Done()
		var e starlark.Value
		for it.Next(&e) {
			n += payloadOf(e, counts, seen)
		}
		return n
	}
	return 0
}

func readStudyOptions(f *ast.File, specDir string) *ast.StateSpaceOptions {
	cfg := &ast.StateSpaceOptions{Options: &ast.Options{MaxActions: 100, MaxConcurrentActions: 2}}
	if _, err := os.Stat(filepath.Join(specDir, "fizz.yaml")); err == nil {
		c, err := ReadOptionsFromYaml(filepath.Join(specDir, "fizz.yaml"))
		if err != nil {
			panic(err)
		}
		cfg = c
	}
	if f.GetFrontMatter().GetYaml() != "" {
		fm, err := ReadOptionsFromYamlString(f.GetFrontMatter().GetYaml())
		if err != nil {
			panic(err)
		}
		proto.Merge(cfg, fm)
	}
	return cfg
}

// TestMemsizeStudySpec runs a spec (FIZZ_MEMSTUDY_SPEC=path/to/Spec.json),
// then samples stored nodes and measures what their Heap.state costs when
// deep cloned, against the payload it holds.
func TestMemsizeStudySpec(t *testing.T) {
	spec := os.Getenv("FIZZ_MEMSTUDY_SPEC")
	if spec == "" {
		t.Skip("set FIZZ_MEMSTUDY_SPEC to a Spec.json")
	}
	f, err := readAstFromFile(spec)
	require.Nil(t, err)
	cfg := readStudyOptions(f, filepath.Dir(spec))
	var m0, m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m0)
	p := NewProcessor([]*ast.File{f}, cfg, false, 0, "", "", false, nil, nil, "")
	_, _, err = p.Start()
	require.Nil(t, err)
	runtime.GC()
	runtime.ReadMemStats(&m1)
	nodes := len(p.visited)
	liveTotal := float64(m1.HeapAlloc-m0.HeapAlloc) / float64(nodes)

	// Deterministic sample of stored nodes.
	keys := make([]string, 0, nodes)
	for k := range p.visited {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	step := len(keys)/400 + 1
	var sample []*Node
	for i := 0; i < len(keys); i += step {
		sample = append(sample, p.visited[keys[i]])
	}

	counts := map[string]int{}
	payload := 0
	for _, n := range sample {
		seen := map[starlark.Value]bool{}
		for _, v := range n.Heap.state {
			payload += payloadOf(v, counts, seen)
		}
	}
	// Retained bytes of the cloned state dicts (map + values), and of the
	// values alone (so the Go map overhead can be split out).
	const reps = 20
	measureFn := func(mk func(n *Node) any) float64 {
		hold := make([]any, 0, reps*len(sample))
		runtime.GC()
		runtime.GC()
		var a, b runtime.MemStats
		runtime.ReadMemStats(&a)
		for r := 0; r < reps; r++ {
			for _, n := range sample {
				hold = append(hold, mk(n))
			}
		}
		runtime.GC()
		runtime.GC()
		runtime.ReadMemStats(&b)
		runtime.KeepAlive(hold)
		return float64(int64(b.HeapAlloc)-int64(a.HeapAlloc)) / float64(reps*len(sample))
	}
	dictBytes := measureFn(func(n *Node) any {
		return CloneDict(n.Heap.state, make(map[starlark.Value]starlark.Value), nil, 0)
	})
	valueBytes := measureFn(func(n *Node) any {
		refs := make(map[starlark.Value]starlark.Value)
		out := make([]starlark.Value, 0, len(n.Heap.state))
		for _, v := range n.Heap.state {
			c, err := deepCloneStarlarkValue(v, refs)
			if err != nil {
				panic(err)
			}
			out = append(out, c)
		}
		return out
	})
	perNodePayload := float64(payload) / float64(len(sample))
	fmt.Printf("spec=%s nodes=%d sampled=%d\n", spec, nodes, len(sample))
	fmt.Printf("live heap per stored node (whole checker): %.0f B\n", liveTotal)
	fmt.Printf("Heap.state per node: cloned dict %.0f B (values only %.0f B, slice of %d slots incl.), payload %.1f B, overhead %.1fx\n",
		dictBytes, valueBytes, len(sample[0].Heap.state), perNodePayload, dictBytes/perNodePayload)
	type kv struct {
		k string
		v int
	}
	var cs []kv
	for k, v := range counts {
		cs = append(cs, kv{k, v})
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].v > cs[j].v })
	for _, c := range cs {
		fmt.Printf("  %-16s %6.1f per node\n", c.k, float64(c.v)/float64(len(sample)))
	}
}
