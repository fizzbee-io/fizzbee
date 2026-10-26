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
// done the partial graph must give the full graph's verdicts. Each spec is
// also run with its actions declared in reverse, for another exploration
// order.
func TestIncrementalLivenessIsSound(t *testing.T) {
	base := "."
	if dir := os.Getenv("RUNFILES_DIR"); dir != "" {
		base = filepath.Join(dir, "_main", "modelchecker")
	}
	specs := []string{
		"formulas/Counter", "formulas/Fairness", "formulas/Stutter", "formulas/SoftKeywords",
		"formulas/WeakUntil", "formulas/Possibly", "formulas/PossiblyCrash", "liveness/WeakAcrossChoice", "liveness/EarlyViolation",
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
					Options:           &ast.Options{MaxActions: 10, MaxConcurrentActions: 1, CrashOnYield: &crashOnYield},
					DeadlockDetection: &noDeadlock,
				}
				p := NewProcessor([]*ast.File{file}, stateConfig, false, 0, "", "", true, nil, nil, "")
				p.SetIncrementalLiveness(true)
				p.incrementalEvery, p.nextIncrementalCheck = 1, 0
				seenEarly := map[string]bool{}
				p.partialCheckHook = func(s *sccGraph) {
					for name := range partialVerdicts(t, s, file) {
						if len(p.pendingActions) > 0 {
							seenEarly[name] = true
						}
					}
				}
				root, failed, err := p.Start()
				require.Nil(t, err)
				require.Nil(t, failed)
				assert.Empty(t, p.pendingActions, "every queued action is done")

				final := partialVerdicts(t, p.partialSccGraph(), file)
				nodes, _, _, _ := GetAllNodes(root, 10)
				full := partialVerdicts(t, newSccGraph(root, nodes), file)
				assert.Equal(t, full, final, "the partial graph of a finished exploration gives other verdicts")
				for name := range seenEarly {
					assert.True(t, full[name], "%s fails on a partial graph but holds on the full one", name)
				}
				early += len(seenEarly)
			})
		}
	}
	assert.Positive(t, early, "no violation was found before the exploration finished")
}
