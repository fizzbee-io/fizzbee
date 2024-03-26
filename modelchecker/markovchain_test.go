package modelchecker

import (
	ast "fizz/proto"
	"fmt"
	"github.com/jayaprabhakar/fizzbee/lib"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestSteadyStateDistribution(t *testing.T) {

	runfilesDir := os.Getenv("RUNFILES_DIR")
	tests := []struct {
	filename             string
	stateConfig		 	 string
	maxActions           int
	expectedNodes        int
	maxConcurrentActions int
	perfModel            string
	fizzConfig		     string
}{

		{
			filename:   "examples/tutorials/10-coins-to-dice-atomic-3sided/ThreeSidedDie.json",
			maxActions: 10,
		},
		{
			filename:   "examples/tutorials/10.1-coins-to-dice-atomic-6sided/Die.json",
			maxActions: 10,
		},
		{
			filename:   "examples/tutorials/21-unfair-coin/FairCoin.json",
			maxActions: 10,
		},
		{
			filename:   "examples/tutorials/24-while-stmt-atomic/FairCoin.json",
			maxActions: 1,
		},
		{
			filename:   "examples/tutorials/26-unfair-coin-toss-while/FairCoin.json",
			maxActions: 1,
		},
		{
			filename:   "examples/tutorials/27-unfair-coin-toss-while-noreset/FairCoin.json",
			maxActions: 1,
		},
		{
			filename:   "examples/tutorials/28-unfair-coin-toss-while-return/FairCoin.json",
			maxActions: 1,
		},
		{
			filename:   "examples/tutorials/29-simple-function/FlipCoin.json",
			maxActions: 1,
		},
		{
			filename:   "examples/tutorials/30-unfair-coin-toss-method/FairCoin.json",
			maxActions: 1,
		},
		{
			filename:   "examples/tutorials/31-fair-die-from-coin-toss-method/FairDie.json",
			maxActions: 1,
		},
		{
			filename:   "examples/tutorials/31-fair-die-from-coin-toss-method/FairDie.json",
			maxActions: 1,
			perfModel:  "examples/tutorials/31-fair-die-from-coin-toss-method/perf_model.yaml",
		},
		{
			filename:   "examples/tutorials/32-fair-die-from-unfair-coin/FairDie.json",
			maxActions: 1,
		},
		{
			filename:   "examples/tutorials/33-fair-die-from-coin-toss-method-any-stmt/FairDie.json",
			maxActions: 1,
		},
		{
			filename:   "examples/tutorials/16-elements-counter-parallel/Counter.json",
			maxActions: 2,
		},
		{
			filename:      "examples/tutorials/34-simple-hour-clock/HourClock.json",
			maxActions:    100,
		},
		{
			filename:      "examples/tutorials/37-unfair-coin-toss-labels/FairCoin.json",
			maxActions:    1,
		},
		{
			filename:      "examples/tutorials/37-unfair-coin-toss-labels/FairCoin.json",
			maxActions:    1,
			perfModel:     "examples/tutorials/37-unfair-coin-toss-labels/perf_model_unbiased.yaml",
		},
		{
			filename:      "examples/tutorials/37-unfair-coin-toss-labels/FairCoin.json",
			maxActions:    1,
			perfModel:     "examples/tutorials/37-unfair-coin-toss-labels/perf_model_biased.yaml",
		},
		{
			filename:      "examples/tutorials/38-two-dice-with-coins/TwoDice.json",
			maxActions:    1,
			perfModel:     "examples/tutorials/38-two-dice-with-coins/perf_model.yaml",
		},
		{
			filename:      "examples/tutorials/40-simple-hour-clock-init-action/HourClock.json",
			maxActions:    100,
		},
		//{
		//	filename:      "examples/comparisons/gossa-v1/gossa.json",
		//	maxActions:    20,
		//},
		//{
		//	filename:      "examples/comparisons/ewd426-token-ring/TokenRing.json",
		//	stateConfig:   "examples/comparisons/ewd426-token-ring/fizz.yaml",
		//	perfModel:     "examples/comparisons/ewd426-token-ring/perf_model.yaml",
		//},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%s", test.filename), func(t *testing.T) {
			filename := filepath.Join(runfilesDir, "_main", test.filename)
			file, err := readAstFromFile(filename)
			require.Nil(t, err)
			files := []*ast.File{file}
			stateCfg := &ast.StateSpaceOptions{}
			if test.stateConfig != "" {
				stateCfgFileName := filepath.Join(runfilesDir, "_main", test.stateConfig)
				stateCfg, err = ReadOptionsFromYaml(stateCfgFileName)
				require.Nil(t, err)
			} else {
				maxThreads := test.maxConcurrentActions
				if maxThreads == 0 {
					maxThreads = test.maxActions
				}
				stateCfg = &ast.StateSpaceOptions{
					ContinuePathOnInvariantFailures: true,
					ContinueOnInvariantFailures: true,
					Options: &ast.Options{
						MaxActions:           int64(test.maxActions),
						MaxConcurrentActions: int64(maxThreads),
					},
				}
			}
			p1 := NewProcessor(files, stateCfg)
			root, _, _ := p1.Start()
			//RemoveMergeNodes(root)

			//dotString := GenerateDotFile(root, make(map[*Node]bool))
			//fmt.Printf("\n%s\n", dotString)

			perfModel := &ast.PerformanceModel{}
			if test.perfModel != "" {
				perfModelFileName := filepath.Join(runfilesDir, "_main", test.perfModel)
				err = lib.ReadProtoFromFile(perfModelFileName, perfModel)
				require.Nil(t, err)
			}

			steadyStateDist, histogram := steadyStateDistribution(root, perfModel)
			fmt.Println(steadyStateDist)
			fmt.Println(histogram.GetMeanCounts())
			//fmt.Println(histogram.GetAllHistogram())
			allNodes, _, _ := getAllNodes(root)
			for j, prob := range steadyStateDist {
				if prob > 1e-6 && allNodes[j].Process != nil {
					fmt.Printf("%2d: prob: %1.6f, state: %s / returns: %s\n", j, prob, allNodes[j].Heap.String(), allNodes[j].Returns.String())
				}
			}
			for k, inv := range files[0].Invariants {
				if !inv.Eventually && !slices.Contains(inv.TemporalOperators, "eventually") {
					continue
				}
				_, histogram := FindAbsorptionCosts(root, perfModel, 0, k)
				fmt.Println("Absorption Cost")
				fmt.Println(histogram.GetMeanCounts())
				//fmt.Println(histogram.GetAllHistogram())
				eventuallyAlways := (inv.Eventually && inv.GetNested().GetAlways()) || (len(inv.TemporalOperators) == 2 &&
									inv.TemporalOperators[0] == "eventually" && inv.TemporalOperators[1] == "always")

				if eventuallyAlways {
					fmt.Println("Eventually Always")
					for j, prob := range steadyStateDist {
						if prob > 1e-6 && allNodes[j].Process != nil && len(allNodes[j].Process.Threads) == 0 {
							status := "DEAD"
							if allNodes[j].Process.Witness[0][k] {
								status = "LIVE"
							}
							fmt.Printf("%s %3d: prob: %1.6f, state: %s / returns: %s\n", status, j, prob, allNodes[j].Heap.String(), allNodes[j].Returns.String())
						}

					}
					continue
				} else {
					liveness, _ := checkLivenessAndCost(root, perfModel, 0, k)
					//liveness := checkLiveness(root, 0, k)
					fmt.Println(liveness)
					fmt.Println("Liveness")
					for j, prob := range liveness {
						if prob > 1e-6 {
							status := "DEAD"

							if allNodes[j].Process.Witness[0][k] {
								status = "LIVE"
							}
							fmt.Printf("%s %3d: prob: %1.6f, state: %s / returns: %s\n", status, j, prob, allNodes[j].Heap.String(), allNodes[j].Returns.String())
						}
					}
				}

			}
		})
	}
}
