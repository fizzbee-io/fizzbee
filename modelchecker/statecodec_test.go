package modelchecker

import (
	ast "fizz/proto"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// freshHash recomputes p's hash, ignoring and then restoring the caches.
func freshHash(p *Process) string {
	saved, savedHeap, savedThreads := p.CachedHashCode, p.Heap.CachedHashCode, p.CachedThreadHashes
	p.CachedHashCode, p.Heap.CachedHashCode = "", ""
	h := p.HashCode()
	p.CachedHashCode, p.Heap.CachedHashCode, p.CachedThreadHashes = saved, savedHeap, savedThreads
	return h
}

func checkProcessRoundTrip(t *testing.T, p *Process) []byte {
	t.Helper()
	data, err := EncodeProcess(p)
	require.NoError(t, err)
	p2, err := DecodeProcess(data, p)
	require.NoError(t, err)
	again, err := EncodeProcess(p2)
	require.NoError(t, err)
	require.Equal(t, data, again, "decoded process encodes differently")
	require.Equal(t, freshHash(p), p2.HashCode(), "decoded process hashes differently")
	j1, err := p.MarshalJSON()
	require.NoError(t, err)
	j2, err := p2.MarshalJSON()
	require.NoError(t, err)
	require.JSONEq(t, string(j1), string(j2))
	return data
}

func runSpecForCodec(t testing.TB, spec string, cfg *ast.StateSpaceOptions) *Processor {
	f, err := readAstFromFile(spec)
	require.NoError(t, err)
	if cfg == nil {
		cfg = &ast.StateSpaceOptions{Options: &ast.Options{MaxActions: 5, MaxConcurrentActions: 2}}
	}
	if f.GetFrontMatter().GetYaml() != "" {
		fm, err := ReadOptionsFromYamlString(f.GetFrontMatter().GetYaml())
		require.NoError(t, err)
		proto.Merge(cfg, fm)
	}
	p := NewProcessor([]*ast.File{f}, cfg, false, 0, "", "", false, nil, nil, "")
	defer func() {
		// Some testdata specs exist to check that a model error is
		// reported; there is nothing to round trip in those.
		if r := recover(); r != nil {
			t.Skipf("spec stops with a model error: %v", r)
		}
	}()
	_, _, err = p.Start()
	require.NoError(t, err)
	return p
}

func sortedVisited(p *Processor) []*Node {
	keys := make([]string, 0, len(p.visited))
	for k := range p.visited {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	nodes := make([]*Node, 0, len(keys))
	for _, k := range keys {
		if n := p.visited[k]; n != nil && n.Process != nil {
			nodes = append(nodes, n)
		}
	}
	return nodes
}

// TestStateCodecProcesses round trips every stored state of the specs in
// testdata: the decoded process must encode to the same bytes, hash the
// same and render the same JSON.
func TestStateCodecProcesses(t *testing.T) {
	base := "."
	if dir := os.Getenv("RUNFILES_DIR"); dir != "" {
		base = filepath.Join(dir, "_main", "modelchecker")
	}
	specs, err := filepath.Glob(filepath.Join(base, "testdata", "*", "*.json"))
	require.NoError(t, err)
	require.NotEmpty(t, specs)
	for _, spec := range specs {
		t.Run(spec, func(t *testing.T) {
			if filepath.Base(spec) == "EndlessLoop.json" {
				t.Skip("slow, and stops with a model error")
			}
			var cfg *ast.StateSpaceOptions
			if filepath.Base(filepath.Dir(spec)) == "ordinal_respace" {
				// As ordinal_respace_test runs them: these specs exhaust
				// the ordinal space with concurrent actions.
				cfg = &ast.StateSpaceOptions{Options: &ast.Options{MaxActions: 20, MaxConcurrentActions: 1}}
			}
			p := runSpecForCodec(t, spec, cfg)
			for _, n := range sortedVisited(p) {
				checkProcessRoundTrip(t, n.Process)
			}
		})
	}
}

// TestStateCodecSpecStats reports encoded size and speed for a spec
// (FIZZ_STATECODEC_SPEC=path/to/Spec.json), on a sample of stored states.
func TestStateCodecSpecStats(t *testing.T) {
	spec := os.Getenv("FIZZ_STATECODEC_SPEC")
	if spec == "" {
		t.Skip("set FIZZ_STATECODEC_SPEC to a Spec.json")
	}
	var cfg *ast.StateSpaceOptions
	if yaml := filepath.Join(filepath.Dir(spec), "fizz.yaml"); fileExists(yaml) {
		c, err := ReadOptionsFromYaml(yaml)
		require.NoError(t, err)
		cfg = c
	} else {
		cfg = &ast.StateSpaceOptions{Options: &ast.Options{MaxActions: 100, MaxConcurrentActions: 2}}
	}
	p := runSpecForCodec(t, spec, cfg)
	nodes := sortedVisited(p)
	step := len(nodes)/2000 + 1
	var sample []*Node
	for i := 0; i < len(nodes); i += step {
		sample = append(sample, nodes[i])
	}

	total, heapOnly := 0, 0
	encoded := make([][]byte, len(sample))
	for i, n := range sample {
		encoded[i] = checkProcessRoundTrip(t, n.Process)
		total += len(encoded[i])
		h, err := EncodeProcess(&Process{Heap: n.Heap, Evaluator: n.Evaluator})
		require.NoError(t, err)
		heapOnly += len(h)
	}

	const reps = 20
	start := time.Now()
	for r := 0; r < reps; r++ {
		for _, n := range sample {
			_, _ = EncodeProcess(n.Process)
		}
	}
	encNs := float64(time.Since(start).Nanoseconds()) / float64(reps*len(sample))
	start = time.Now()
	for r := 0; r < reps; r++ {
		for i, n := range sample {
			_, _ = DecodeProcess(encoded[i], n.Process)
		}
	}
	decNs := float64(time.Since(start).Nanoseconds()) / float64(reps*len(sample))
	start = time.Now()
	for r := 0; r < reps; r++ {
		for _, n := range sample {
			_ = n.Process.Fork()
		}
	}
	forkNs := float64(time.Since(start).Nanoseconds()) / float64(reps*len(sample))

	// Retained bytes of a forked process vs its encoding.
	hold := make([]any, 0, reps*len(sample))
	var a, b runtime.MemStats
	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&a)
	for r := 0; r < reps; r++ {
		for _, n := range sample {
			f := n.Process.Fork()
			f.Parent = nil
			hold = append(hold, f)
		}
	}
	runtime.GC()
	runtime.ReadMemStats(&b)
	runtime.KeepAlive(hold)
	runtime.KeepAlive(p)
	forkBytes := float64(int64(b.HeapAlloc)-int64(a.HeapAlloc)) / float64(reps*len(sample))

	k := float64(len(sample))
	fmt.Printf("spec=%s stored=%d sampled=%d\n", spec, len(nodes), len(sample))
	fmt.Printf("encoded process: %.0f B/state (heap state alone %.0f B); forked process in memory: %.0f B\n",
		float64(total)/k, float64(heapOnly)/k, forkBytes)
	fmt.Printf("encode %.1f us, decode %.1f us, Fork %.1f us per state\n", encNs/1e3, decNs/1e3, forkNs/1e3)
}
