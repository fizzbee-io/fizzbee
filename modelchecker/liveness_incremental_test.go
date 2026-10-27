package modelchecker

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	ast "fizz/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// partialVerdicts checks each liveness assertion of file on s, except
// formulas with possibly, and returns the ones that fail.
func partialVerdicts(t *testing.T, s *sccGraph, file *ast.File) map[string]bool {
	failed := map[string]bool{}
	for _, inv := range file.Invariants {
		if hasPossibly(inv) {
			continue
		}
		_, pos, err := s.checkStrictLiveness([]*ast.File{file}, func(other *ast.Invariant) bool { return other != inv })
		require.Nil(t, err)
		if pos != nil {
			failed[inv.Name] = true
		}
	}
	return failed
}

// TestIncrementalLivenessIsSound checks liveness on the closed part of the
// graph after every step of the exploration. Every assertion that fails on
// a partial graph must fail on the full graph, and once the exploration is
// done the partial graph must give the full graph's verdicts. A node a
// partial check used must not change after: no new links, and no rename,
// as a crash node gets when a later state is a duplicate of it (CrashRename
// covers that). Each spec is also run with its actions declared in
// reverse, for another exploration order.
func TestIncrementalLivenessIsSound(t *testing.T) {
	base := "."
	if dir := os.Getenv("RUNFILES_DIR"); dir != "" {
		base = filepath.Join(dir, "_main", "modelchecker")
	}
	specs := []string{
		"formulas/Counter", "formulas/Fairness", "formulas/Stutter", "formulas/SoftKeywords",
		"formulas/WeakUntil", "formulas/Possibly", "formulas/PossiblyCrash", "liveness/WeakAcrossChoice", "liveness/EarlyViolation", "liveness/CrashRename",
	}
	early := 0
	for _, spec := range specs {
		for _, variant := range []struct{ crashOnYield, reversed bool }{{false, false}, {true, false}, {false, true}} {
			crashOnYield := variant.crashOnYield
			name := spec
			if crashOnYield {
				name += "/crash"
			}
			if variant.reversed {
				name += "/reversed"
			}
			t.Run(name, func(t *testing.T) {
				file, err := readAstFromFile(filepath.Join(base, "testdata", spec+".json"))
				require.Nil(t, err)
				if variant.reversed {
					// Declaring the actions in reverse order explores the
					// graph in another order, so other nodes are open at
					// each step. Init stays first.
					slices.Reverse(file.Actions[1:])
				}
				noDeadlock := false
				stateConfig := &ast.StateSpaceOptions{
					Options:           &ast.Options{MaxActions: 10, MaxConcurrentActions: 2, CrashOnYield: &crashOnYield},
					DeadlockDetection: &noDeadlock,
				}
				p := NewProcessor([]*ast.File{file}, stateConfig, false, 0, "", "", true, nil, nil, "")
				p.SetIncrementalLiveness(true)
				p.incrementalEvery, p.nextIncrementalCheck = 1, 0
				// Every assertion a partial check failed, and those it failed
				// while the queue still had work, before the exploration was
				// done.
				failedPartial, failedEarly := map[string]bool{}, map[string]bool{}
				// What a partial check saw of each node: its name, which
				// liveness labels depend on, and how many links it had.
				type seen struct {
					name  string
					links int
				}
				firstSeen := map[*Node]seen{}
				p.partialCheckHook = func(s *sccGraph) {
					for _, n := range s.nodes {
						if _, ok := firstSeen[n]; !ok {
							firstSeen[n] = seen{n.Name, len(n.Outbound)}
						}
					}
					for name := range partialVerdicts(t, s, file) {
						failedPartial[name] = true
						if p.queue.Len() > 0 {
							failedEarly[name] = true
						}
					}
				}
				root, failed, err := p.Start()
				require.Nil(t, err)
				require.Nil(t, failed)
				assert.Empty(t, p.pendingActions, "every queued action is done")
				for n, was := range firstSeen {
					assert.Equal(t, was, seen{n.Name, len(n.Outbound)}, "a node changed after a partial check used it")
				}

				final := partialVerdicts(t, p.partialSccGraph(), file)
				nodes, _, _, _ := GetAllNodes(root, 10)
				full := partialVerdicts(t, newSccGraph(root, nodes), file)
				assert.Equal(t, full, final, "the partial graph of a finished exploration gives other verdicts")
				for name := range failedPartial {
					assert.True(t, full[name], "%s fails on a partial graph but holds on the full one", name)
				}
				early += len(failedEarly)
			})
		}
	}
	assert.Positive(t, early, "no violation was found before the exploration finished")
}
