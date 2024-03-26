package modelchecker

import (
	ast "fizz/proto"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
	"google.golang.org/protobuf/encoding/protojson"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRemoveCurrentThread is a unit test for Process.removeCurrentThread.
func TestRemoveCurrentThread(t *testing.T) {
	p := &Process{
		Threads: []*Thread{
			&Thread{},
			&Thread{},
			&Thread{},
		},
		Current: 1,
	}
	p.removeCurrentThread()
	assert.Equal(t, 2, len(p.Threads))
	assert.Equal(t, 0, p.Current)

	p.Current = 1
	p.removeCurrentThread()
	assert.Equal(t, 1, len(p.Threads))
	assert.Equal(t, 0, p.Current)

	p.Current = 0
	p.removeCurrentThread()
	assert.Equal(t, 0, len(p.Threads))
	assert.Equal(t, 0, p.Current)
}

// TestHash is a unit test for Process.Hash.
func TestHash(t *testing.T) {
	file, err := parseAstFromString(ActionsWithMultipleBlocks)
	require.Nil(t, err)
	files := []*ast.File{file}
	process := NewProcess("", files, nil)
	process.Heap.globals = starlark.StringDict{"a": starlark.MakeInt(10), "b": starlark.MakeInt(20)}
	assert.Len(t, process.Threads, 0)
	process.NewThread()
	thread := process.currentThread()
	assert.Equal(t, thread.Stack.Len(), 1)

	thread.currentFrame().pc = "Actions[0]"

	h1 := process.HashCode()
	process.removeCurrentThread()
	assert.NotEqual(t, h1, process.HashCode())

	t0 := NewThread(process, files, 0, "Actions[0]")
	t1 := NewThread(process, files, 0, "Actions[1]")
	t2 := NewThread(process, files, 0, "Actions[2]")
	t3 := NewThread(process, files, 0, "Actions[3]")
	p1 := &Process{
		Threads: []*Thread{
			t0,
			t1,
			t2,
			t3,
		},
		Current: 1,
		Heap: &Heap{
			globals: starlark.StringDict{"a": starlark.MakeInt(10), "b": starlark.MakeInt(20)},
		},
	}
	p2 := &Process{
		Threads: []*Thread{
			t2,
			t3,
			t0,
			t1,
		},
		Current: 3,
		Heap: &Heap{
			globals: starlark.StringDict{"a": starlark.MakeInt(10), "b": starlark.MakeInt(20)},
		},
	}

	assert.Equal(t, p1.HashCode(), p2.HashCode())
}

func TestProcessor_Start(t *testing.T) {
	file, err := parseAstFromString(ActionsWithMultipleBlocks)
	require.Nil(t, err)
	files := []*ast.File{file}
	p1 := NewProcessor(files, &ast.StateSpaceOptions{
		Options: &ast.Options{
			MaxActions:           1,
		},
	})
	root, _, _ := p1.Start()
	assert.NotNil(t, root)
	assert.Equal(t, 93, len(p1.visited))
}

func printFileNames(rootDir string) error {
	return filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			relativePath, _ := filepath.Rel(rootDir, path)
			fmt.Println(relativePath)
		}
		return nil
	})
}

func TestProcessor_Tutorials(t *testing.T) {
	runfilesDir := os.Getenv("RUNFILES_DIR")
	tests := []struct {
		filename             string
		stateConfig		 	 string
		maxActions           int
		expectedNodes        int
		maxConcurrentActions int
	}{
		{
			filename:      "examples/tutorials/00-no-op/Counter.json",
			maxActions:    5,
			expectedNodes: 1, // 1 nodes: 1 for the init
		},
		{
			filename:      "examples/tutorials/01-atomic-counters/Counter.json",
			maxActions:    1,
			expectedNodes: 2, // 2 nodes: 1 for the init and 1 for the first action
		},
		{
			filename:      "examples/tutorials/01-atomic-counters/Counter.json",
			maxActions:    3,
			expectedNodes: 4, // 2 nodes: 1 for the init and 1 for each action
		},
		{
			filename:      "examples/tutorials/01-atomic-counters/Counter.json",
			stateConfig:   "examples/tutorials/01-atomic-counters/fizz.yaml",
			expectedNodes: 6,
		},
		{
			filename:      "examples/tutorials/01-atomic-counters/Counter.json",
			maxActions:    100,
			expectedNodes: 101, // 0.01s
		},
		{
			filename:      "examples/tutorials/02-multiple-atomic-counters/Counter.json",
			maxActions:    1,
			expectedNodes: 2,
		},
		{
			filename:      "examples/tutorials/02-multiple-atomic-counters/Counter.json",
			maxActions:    2,
			expectedNodes: 3,
		},
		{
			filename:      "examples/tutorials/02-multiple-atomic-counters/Counter.json",
			maxActions:    4,
			expectedNodes: 8,
		},
		{
			filename:      "examples/tutorials/02-multiple-atomic-counters/Counter.json",
			maxActions:    10,
			expectedNodes: 144, // 0.01s
			// 20 actions, 17711 nodes, 3.79s
		},
		{
			filename:      "examples/tutorials/06-inc-dec-atomic-counters/Counter.json",
			maxActions:    2,
			expectedNodes: 5,
		},
		{
			filename:      "examples/tutorials/06-inc-dec-atomic-counters/Counter.json",
			maxActions:    10,
			expectedNodes: 21,
			// 20 actions, 41 nodes, 0.01s
			// this grows much slower than multiply counter, because any combination of inc / dec forms a loop
		},
		{
			filename:      "examples/tutorials/02-multiple-atomic-counters/Counter.json",
			maxActions:    3,
			expectedNodes: 5,
		},
		{
			filename:      "examples/tutorials/06-inc-dec-atomic-counters/Counter.json",
			maxActions:    3,
			expectedNodes: 7,
		},
		{
			filename:      "examples/tutorials/03-multiple-serial-counters/Counter.json",
			maxActions:    1,
			expectedNodes: 6,
		},
		{
			filename:      "examples/tutorials/03-multiple-serial-counters/Counter.json",
			maxActions:    2,
			expectedNodes: 45,
		},
		{
			filename:      "examples/tutorials/03-multiple-serial-counters/Counter.json",
			maxActions:    4,
			expectedNodes: 1622,
		},
		{
			filename:      "examples/tutorials/04-multiple-oneof-counters/Counter.json",
			maxActions:    1,
			expectedNodes: 5, // 5 nodes: 1 for the init and 1 for each action and 1 for each stmt in add. multipy counteres end up being no-op
		},
		{
			filename:      "examples/tutorials/04-multiple-oneof-counters/Counter.json",
			maxActions:    2,
			expectedNodes: 12, // 7 nodes: 1 for the init and 1 for each action and 1 for each stmt in each action
		},
		{
			filename:      "examples/tutorials/04-multiple-oneof-counters/Counter.json",
			maxActions:    3,
			expectedNodes: 24,
		},
		{
			filename:      "examples/tutorials/05-multiple-parallel-counters/Counter.json",
			maxActions:    1,
			expectedNodes: 8,
		},
		{
			filename:      "examples/tutorials/09-inc-dec-parallel-counters/Counter.json",
			maxActions:    1,
			expectedNodes: 9,
		},
		{
			filename:      "examples/tutorials/05-multiple-parallel-counters/Counter.json",
			maxActions:    2,
			expectedNodes: 61,
		},
		{
			filename:      "examples/tutorials/05-multiple-parallel-counters/Counter.json",
			maxActions:    3,
			expectedNodes: 287, // .03s
		},
		{
			filename:      "examples/tutorials/10-coins-to-dice-atomic-3sided/ThreeSidedDie.json",
			maxActions:    1,
			expectedNodes: 4, // 2 nodes: 1 for the init and 1 for the Toss action and 1 for each fork
		},
		{
			filename:      "examples/tutorials/10-coins-to-dice-atomic-3sided/ThreeSidedDie.json",
			maxActions:    2,
			expectedNodes: 9,
		},
		{
			filename:      "examples/tutorials/10-coins-to-dice-atomic-3sided/ThreeSidedDie.json",
			maxActions:    3,
			expectedNodes: 9,
		},
		{
			filename:      "examples/tutorials/10-coins-to-dice-atomic-3sided/ThreeSidedDie.json",
			maxActions:    10,
			expectedNodes: 9,
		},
		{
			filename:      "examples/tutorials/13-any-stmt/Counter.json",
			maxActions:    1,
			expectedNodes: 7,
		},
		{
			filename:      "examples/tutorials/13-any-stmt/Counter.json",
			maxActions:    10,
			expectedNodes: 63,
		},
		{
			filename:      "examples/tutorials/14-elements-counter-atomic/Counter.json",
			maxActions:    1,
			expectedNodes: 5,
		},
		{
			filename:      "examples/tutorials/14-elements-counter-atomic/Counter.json",
			maxActions:    3,
			expectedNodes: 21,
		},
		{
			filename:   "examples/tutorials/14-elements-counter-atomic/Counter.json",
			maxActions: 10,
			// Just one more node than 3 actions, because maximum unique state is 3 added followed by 1 remove
			expectedNodes: 22,
		},
		{
			filename:      "examples/tutorials/15-elements-counter-serial/Counter.json",
			maxActions:    1,
			expectedNodes: 15,
		},
		{
			filename:      "examples/tutorials/15-elements-counter-serial/Counter.json",
			maxActions:    2,
			expectedNodes: 216,
		},
		{
			filename:      "examples/tutorials/15-elements-counter-serial/Counter.json",
			maxActions:    3,
			expectedNodes: 1964,
		},
		{
			filename:      "examples/tutorials/16-elements-counter-parallel/Counter.json",
			maxActions:    1,
			expectedNodes: 18,
		},
		{
			filename:      "examples/tutorials/16-elements-counter-parallel/Counter.json",
			maxActions:    2,
			expectedNodes: 308,
		},
		{
			filename:             "examples/tutorials/16-elements-counter-parallel/Counter.json",
			maxActions:           3,
			expectedNodes:        3088,
			maxConcurrentActions: 3,
		},
		{
			filename:             "examples/tutorials/16-elements-counter-parallel/Counter.json",
			maxActions:           3,
			maxConcurrentActions: 2,
			expectedNodes:        1733, // 0.16s 131
		},
		{
			filename:             "examples/tutorials/16-elements-counter-parallel/Counter.json",
			stateConfig: 		  "examples/tutorials/16-elements-counter-parallel/fizz.yaml",
			expectedNodes:        1733, // 0.16s 131
		},
		{
			filename:             "examples/tutorials/16-elements-counter-parallel/Counter.json",
			maxActions:           4,
			maxConcurrentActions: 2,
			expectedNodes:        4773, // 0.16s 162
		},
		{
			filename:      "examples/tutorials/17-for-stmt-atomic/ForLoop.json",
			maxActions:    5,
			expectedNodes: 2, // Only 2 nodes, because the for loop is executed as a single action
		},
		{
			filename:   "examples/tutorials/18-for-stmt-serial/ForLoop.json",
			maxActions: 2,
			// The main reason for the significant increase in the nodes is because, the two threads can execute
			// concurrently. So, in one thread might have deleted first element, then the second thread would start
			// the loop, then both the threads would start interleaving between the two threads for each iteration.
			expectedNodes: 36,
		},
		{
			filename:      "examples/tutorials/19-for-stmt-serial-check-again/ForLoop.json",
			maxActions:    1,
			expectedNodes: 6,
		},
		{
			filename:      "examples/tutorials/19-for-stmt-serial-check-again/ForLoop.json",
			maxActions:    2,
			expectedNodes: 20,
		},
		{
			filename:      "examples/tutorials/20-for-stmt-parallel-check-again/ForLoop.json",
			maxActions:    1,
			expectedNodes: 25,
		},
		{
			filename:      "examples/tutorials/20-for-stmt-parallel-check-again/ForLoop.json",
			maxActions:    2,
			expectedNodes: 194,
		},
		{
			filename:      "examples/tutorials/21-unfair-coin/FairCoin.json",
			maxActions:    10,
			expectedNodes: 8,
		},
		{
			filename:      "examples/tutorials/22-while-stmt-atomic/Counter.json",
			maxActions:    1,
			expectedNodes: 2,
		},
		{
			filename:      "examples/tutorials/22-while-stmt-atomic/Counter.json",
			maxActions:    5,
			expectedNodes: 2,
		},
		{
			filename:      "examples/tutorials/23-while-stmt-serial/Counter.json",
			maxActions:    1,
			expectedNodes: 8,
		},
		{
			filename:      "examples/tutorials/23-while-stmt-serial/Counter.json",
			maxActions:    4,
			expectedNodes: 29,
		},
		{
			filename:      "examples/tutorials/24-while-stmt-atomic/FairCoin.json",
			maxActions:    1,
			expectedNodes: 6,
		},
		{
			filename:      "examples/tutorials/25-break-continue/Loop.json",
			maxActions:    1,
			expectedNodes: 3,
		},
		{
			filename:      "examples/tutorials/26-unfair-coin-toss-while/FairCoin.json",
			maxActions:    1,
			expectedNodes: 6,
		},
		{
			filename:      "examples/tutorials/27-unfair-coin-toss-while-noreset/FairCoin.json",
			maxActions:    1,
			expectedNodes: 12,
		},
		{
			filename:      "examples/tutorials/28-unfair-coin-toss-while-return/FairCoin.json",
			maxActions:    1,
			expectedNodes: 6,
		},
		{
			filename:      "examples/tutorials/29-simple-function/FlipCoin.json",
			maxActions:    1,
			expectedNodes: 4,
		},
		{
			filename:      "examples/tutorials/30-unfair-coin-toss-method/FairCoin.json",
			maxActions:    1,
			expectedNodes: 6,
		},
		{
			filename:      "examples/tutorials/31-fair-die-from-coin-toss-method/FairDie.json",
			maxActions:    1,
			expectedNodes: 14,
		},
		{
			filename:      "examples/tutorials/32-fair-die-from-unfair-coin/FairDie.json",
			maxActions:    1,
			expectedNodes: 28,
		},
		{
			filename:      "examples/tutorials/33-fair-die-from-coin-toss-method-any-stmt/FairDie.json",
			maxActions:    1,
			expectedNodes: 14,
		},
		{
			filename:      "examples/tutorials/34-simple-hour-clock/HourClock.json",
			maxActions:    100,
			expectedNodes: 12,
		},
		{
			filename:      "examples/tutorials/35-list-example/Counter.json",
			maxActions:    1,
			expectedNodes: 5,
		},
		{
			filename:      "examples/tutorials/35-list-example/Counter.json",
			maxActions:    10,
			expectedNodes: 15,
		},
		{
			filename:      "examples/tutorials/36-dict-example/Counter.json",
			maxActions:    1,
			expectedNodes: 5,
		},
		{
			filename:      "examples/tutorials/36-dict-example/Counter.json",
			maxActions:    10,
			expectedNodes: 16,
		},
		{
			filename:      "examples/tutorials/37-unfair-coin-toss-labels/FairCoin.json",
			maxActions:    1,
			expectedNodes: 6,
		},
		{
			filename:      "examples/tutorials/39-actions-limit/Limit.json",
			stateConfig:   "examples/tutorials/39-actions-limit/fizz.yaml",
			expectedNodes: 5,
		},
		{
			filename:      "examples/tutorials/40-simple-hour-clock-init-action/HourClock.json",
			maxActions:    100,
			expectedNodes: 13,
		},
		//{
		//	filename:      "examples/comparisons/gossa-v1/gossa.json",
		//	maxActions:    30,
		//	expectedNodes: 455,
		//},
		{
			filename:      "examples/comparisons/ewd426-token-ring/TokenRing.json",
			maxActions:    10,
			expectedNodes: 2389,
		},
		{
			filename:      "examples/comparisons/ewd426-token-ring/TokenRing.json",
			stateConfig:   "examples/comparisons/ewd426-token-ring/fizz.yaml",
			expectedNodes: 2389,
		},

	}
	tempDir := CreateTempDirectory(t)
	_ = tempDir
	//defer profile.Start(profile.CPUProfile, profile.ProfilePath(".")).Stop()
	for _, test := range tests {
		t.Run(fmt.Sprintf("%s", test.filename), func(t *testing.T) {
			filename := filepath.Join(runfilesDir, "_main", test.filename)
			file, err := readAstFromFile(filename)
			require.Nil(t, err)
			files := []*ast.File{file}
			stateConfig := &ast.StateSpaceOptions{}
			if test.stateConfig != "" {
				stateCfgFileName := filepath.Join(runfilesDir, "_main", test.stateConfig)
				stateConfig, err = ReadOptionsFromYaml(stateCfgFileName)
				require.Nil(t, err)
			} else {
				maxThreads := test.maxConcurrentActions
				if maxThreads == 0 {
					maxThreads = test.maxActions
				}
				stateConfig = &ast.StateSpaceOptions{
					ContinuePathOnInvariantFailures: true,
					ContinueOnInvariantFailures:     true,
					Options: &ast.Options{
						MaxActions:           int64(test.maxActions),
						MaxConcurrentActions: int64(maxThreads),
					},
				}
			}

			p1 := NewProcessor(files, stateConfig)
			startTime := time.Now()
			root, _, err := p1.Start()
			require.Nil(t, err)
			require.NotNil(t, root)
			assert.Equal(t, test.expectedNodes, len(p1.visited))
			fmt.Printf("Completed Nodes: %d, elapsed: %s\n", len(p1.visited), time.Since(startTime))

			//RemoveMergeNodes(root)
			// Print the modified graph
			//fmt.Printf("Removing merge nodes, elapsed: %s\n", time.Since(startTime))
			//fmt.Println("\nModified Graph:")

			//dotString := GenerateDotFile(root, make(map[*Node]bool))
			//fmt.Printf("Generating dotfile, elapsed: %s\n", time.Since(startTime))
			////dotFileName := RemoveLastSegment(filename, ".json") + ".dot"
			////WriteFile(t, tempDir, dotFileName, []byte(dotString))
			////fmt.Printf("Writing dotfile, elapsed: %s\n", time.Since(startTime))
			//fmt.Printf("\n%s\n", dotString)


			//nodes, _ := getAllNodes(root)
			//_ = nodes
			//outFileName := RemoveLastSegment(filename, ".json") + "-out-"
			//filenamePrefix := filepath.Join(tempDir, outFileName)
			//fmt.Println("Generating proto of json", filenamePrefix)
			//
			//nodeFiles, linkFileNames, err := GenerateProtoOfJson(nodes, filenamePrefix)
			//require.Nil(t, err)
			//fmt.Println("Generated proto of json", nodeFiles, linkFileNames)
			//fmt.Printf("Generating proto of json, elapsed: %s\n", time.Since(startTime))
		})
	}
}

func readAstFromFile(filename string) (*ast.File, error) {
	jsonFile, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer jsonFile.Close()
	bytes, _ := io.ReadAll(jsonFile)
	f := &ast.File{}
	err = protojson.Unmarshal(bytes, f)
	return f, err
}

func CreateTempDirectory(t *testing.T) string {
	tempDir, err := ioutil.TempDir("", "test_artifacts_")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	//defer os.RemoveAll(tempDir)
	return tempDir
}

func WriteFile(t *testing.T, tempDir string, filename string, content []byte) {
	fullPath := filepath.Join(tempDir, filename)
	dir := filepath.Dir(fullPath)
	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		fmt.Printf("Error creating directory %s: %v\n", dir, err)
		return
	}
	fmt.Println("Writing file: ", fullPath)
	err = os.WriteFile(fullPath, content, 0644)
	if err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
}
