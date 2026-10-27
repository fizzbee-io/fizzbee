package modelchecker

import (
	"os"
	"path/filepath"
	"testing"

	ast "fizz/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLivenessSameWithSymmetryReduction checks each spec's liveness
// assertions on the graph explored with symmetry reduction and on the one
// explored without it. Reduction only merges states that are renamings of
// each other, so the verdicts must be the same.
func TestLivenessSameWithSymmetryReduction(t *testing.T) {
	base := "."
	if dir := os.Getenv("RUNFILES_DIR"); dir != "" {
		base = filepath.Join(dir, "_main", "modelchecker")
	}
	specs := []string{"liveness/SymmetricFairness"}
	for _, spec := range specs {
		t.Run(spec, func(t *testing.T) {
			file, err := readAstFromFile(filepath.Join(base, "testdata", spec+".json"))
			require.Nil(t, err)
			verdicts := func(noSymmetry bool) map[string]bool {
				noDeadlock := false
				stateConfig := &ast.StateSpaceOptions{
					Options:           &ast.Options{MaxActions: 10, MaxConcurrentActions: 2},
					DeadlockDetection: &noDeadlock,
				}
				p := NewProcessor([]*ast.File{file}, stateConfig, false, 0, "", "", true, nil, nil, "")
				p.SetDisableSymmetryReduction(noSymmetry)
				root, failed, err := p.Start()
				require.Nil(t, err)
				require.Nil(t, failed)
				nodes, _, _, _ := GetAllNodes(root, 10)
				return partialVerdicts(t, newSccGraph(root, nodes), file)
			}
			assert.Equal(t, verdicts(true), verdicts(false), "failed assertions without symmetry reduction, then with it")
		})
	}
}
