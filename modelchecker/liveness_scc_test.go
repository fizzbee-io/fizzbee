package modelchecker

import (
	"os"
	"path/filepath"
	"testing"

	ast "fizz/proto"
	"github.com/fizzbee-io/fizzbee/modelchecker/liveness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSccLivenessMatchesCycleFinder runs both liveness checkers on the
// explored graphs of example specs. They must agree, and every
// counterexample either reports must satisfy the fairness definition.
func TestSccLivenessMatchesCycleFinder(t *testing.T) {
	base := ".."
	if dir := os.Getenv("RUNFILES_DIR"); dir != "" {
		base = filepath.Join(dir, "_main")
	}
	for _, spec := range []string{
		"examples/tutorials/03-multiple-serial-counters/Counter.json",
		"examples/tutorials/16-elements-counter-parallel/Counter.json",
		"examples/tutorials/24-while-stmt-atomic/FairCoin.json",
		"examples/tutorials/26-unfair-coin-toss-while/FairCoin.json",
		"examples/tutorials/27-unfair-coin-toss-while-noreset/FairCoin.json",
		"examples/tutorials/28-unfair-coin-toss-while-return/FairCoin.json",
		"examples/tutorials/30-unfair-coin-toss-method/FairCoin.json",
		"examples/tutorials/31-fair-die-from-coin-toss-method/FairDie.json",
		"examples/tutorials/34-simple-hour-clock/HourClock.json",
		"examples/tutorials/37-unfair-coin-toss-labels/FairCoin.json",
		"examples/tutorials/40-simple-hour-clock-init-action/HourClock.json",
		"examples/comparisons/ewd426-token-ring/TokenRing.json",
		"examples/comparisons/gossa-v1/gossa.json",
	} {
		t.Run(spec, func(t *testing.T) {
			file, err := readAstFromFile(filepath.Join(base, spec))
			require.Nil(t, err)
			stateConfig, err := ReadOptionsFromYaml(filepath.Join(base, filepath.Dir(spec), "fizz.yaml"))
			require.Nil(t, err)
			p := NewProcessor([]*ast.File{file}, stateConfig, false, 0, "", "", false, nil, nil, "")
			root, _, err := p.Start()
			require.Nil(t, err)
			nodes, _, _, _ := GetAllNodes(root, stateConfig.GetOptions().GetMaxActions())

			oldPath, oldInv := CheckStrictLiveness(root, nodes)
			newPath, newInv, err := CheckStrictLivenessScc(root, nodes)
			require.Nil(t, err)
			assert.Equal(t, oldInv, newInv)

			s := newSccGraph(root, nodes)
			for name, path := range map[string][]*Link{"cycle finder": oldPath, "scc": newPath} {
				if path == nil {
					continue
				}
				l, err := s.toLasso(path)
				require.Nil(t, err, name)
				if oldInv != nil && newInv != nil && *oldInv == *newInv {
					inv := root.Process.Files[newInv.FileIndex].Invariants[newInv.InvariantIndex]
					_, eventuallyAlways := livenessOperators(inv)
					s.configure(strictLivenessPredicate(nodes, newInv.FileIndex, newInv.InvariantIndex, eventuallyAlways), eventuallyAlways)
					assert.Nil(t, liveness.ValidateLasso(s.g, l), name)
				}
			}
		})
	}
}
