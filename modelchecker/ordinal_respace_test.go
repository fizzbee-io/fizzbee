package modelchecker

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	ast "fizz/proto"
	"github.com/fizzbee-io/fizzbee/lib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
)

// TestOrdinalRespaceHeldSegment inserts 80 times into the same ordinal gap
// along one path, well past the ~60 midpoint halvings the ID range allows,
// while a non-atomic action holds the chosen segment across a yield. Every
// stored state must keep its held segments in place relative to the values.
//
// HeldSegment keeps both ends of the segment live. DeadEndpoint drops the
// value at its left end before using it. ResolvedMemo keeps the value()
// result only as the segment's memo between two calls. Each runs with the default respace
// threshold, and with respacing forced on every state.
func TestOrdinalRespaceHeldSegment(t *testing.T) {
	base := "."
	if dir := os.Getenv("RUNFILES_DIR"); dir != "" {
		base = filepath.Join(dir, "_main", "modelchecker")
	}
	defaultGap := lib.OrdinalRespaceMinGap
	defer func() { lib.OrdinalRespaceMinGap = defaultGap }()
	defaultMemoOnly := 0

	for _, spec := range []string{"HeldSegment", "DeadEndpoint", "ResolvedMemo"} {
		for _, gap := range []int64{defaultGap, 1 << 61} {
			t.Run(fmt.Sprintf("%s/gap=%d", spec, gap), func(t *testing.T) {
				lib.OrdinalRespaceMinGap = gap
				file, err := readAstFromFile(filepath.Join(base, "testdata", "ordinal_respace", spec+".json"))
				require.Nil(t, err)
				noDeadlock, noCrash := false, false
				stateConfig := &ast.StateSpaceOptions{
					Options:           &ast.Options{MaxActions: 100, MaxConcurrentActions: 1, CrashOnYield: &noCrash},
					DeadlockDetection: &noDeadlock,
				}
				p := NewProcessor([]*ast.File{file}, stateConfig, false, 0, "", "", false, nil, nil, "")
				root, failed, err := p.Start()
				require.Nil(t, err)
				require.NotNil(t, root)
				assert.Nil(t, failed)

				maxCount := 0
				heldSegments := 0
				memoOnly := 0
				for _, node := range p.visited {
					if count, ok := node.Process.Heap.state["count"].(starlark.Int); ok {
						c, _ := count.Int64()
						maxCount = max(maxCount, int(c))
					}
					for _, thread := range node.Process.Threads {
						if thread == nil {
							continue
						}
						for _, frame := range thread.Stack.RawArray() {
							for scope := frame.scope; scope != nil; scope = scope.parent {
								seg, ok := scope.vars["seg"].(*lib.Segment)
								if !ok {
									continue
								}
								heldSegments++
								// The segment runs from above lo up to hi.
								lo := node.Process.Heap.state["lo"].(*lib.SymmetricValue).GetId()
								hi := node.Process.Heap.state["hi"].(*lib.SymmetricValue).GetId()
								assert.Less(t, lo, seg.Left, "segment %v vs lo %d", seg, lo)
								assert.Less(t, seg.Left, seg.Right, "segment %v", seg)
								assert.Equal(t, hi, seg.Right, "segment %v vs hi %d", seg, hi)
								if seg.Resolved {
									assert.Less(t, seg.Left, seg.ResolvedID, "segment %v", seg)
									assert.Less(t, seg.ResolvedID, seg.Right, "segment %v", seg)
									if node.Process.Heap.state["v"] == starlark.None {
										memoOnly++
									}
								}
							}
						}
					}
				}
				assert.Equal(t, 80, maxCount)
				assert.Positive(t, heldSegments)
				if spec == "ResolvedMemo" {
					assert.Positive(t, memoOnly, "no state kept a memo held only by the segment")
					// Respacing must not drop memos: forcing it on every state
					// keeps exactly as many as the default run.
					if gap == defaultGap {
						defaultMemoOnly = memoOnly
					} else {
						assert.Equal(t, defaultMemoOnly, memoOnly, "memos dropped by respacing")
					}
				}
			})
		}
	}
}

// TestOrdinalRespaceStructAndReturns covers references to ordinal values
// outside plain variables. StructField keeps a value inside a struct, and
// ReturnValue returns the inserted value from each step; the return
// recorded on the transition must match the respaced state.
func TestOrdinalRespaceStructAndReturns(t *testing.T) {
	base := "."
	if dir := os.Getenv("RUNFILES_DIR"); dir != "" {
		base = filepath.Join(dir, "_main", "modelchecker")
	}
	defaultGap := lib.OrdinalRespaceMinGap
	defer func() { lib.OrdinalRespaceMinGap = defaultGap }()

	for _, spec := range []string{"StructField", "ReturnValue"} {
		for _, gap := range []int64{defaultGap, 1 << 61} {
			t.Run(fmt.Sprintf("%s/gap=%d", spec, gap), func(t *testing.T) {
				lib.OrdinalRespaceMinGap = gap
				file, err := readAstFromFile(filepath.Join(base, "testdata", "ordinal_respace", spec+".json"))
				require.Nil(t, err)
				noDeadlock := false
				stateConfig := &ast.StateSpaceOptions{
					Options:           &ast.Options{MaxActions: 100, MaxConcurrentActions: 1},
					DeadlockDetection: &noDeadlock,
				}
				p := NewProcessor([]*ast.File{file}, stateConfig, false, 0, "", "", false, nil, nil, "")
				root, failed, err := p.Start()
				require.Nil(t, err)
				require.NotNil(t, root)
				assert.Nil(t, failed)

				maxCount := 0
				returns := 0
				for _, node := range p.visited {
					if count, ok := node.Process.Heap.state["count"].(starlark.Int); ok {
						c, _ := count.Int64()
						maxCount = max(maxCount, int(c))
					}
					for _, link := range node.Inbound {
						ret, ok := link.Returns["InsertBefore"].(*lib.SymmetricValue)
						if !ok {
							continue
						}
						returns++
						cur := node.Process.Heap.state["t"].(*lib.SymmetricValue)
						assert.Equal(t, cur.GetId(), ret.GetId(), "returned %v, state has %v", ret, cur)
					}
				}
				assert.Equal(t, 80, maxCount)
				if spec == "ReturnValue" {
					assert.Positive(t, returns)
				}
			})
		}
	}
}
