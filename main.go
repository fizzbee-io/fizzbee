package main

import (
	"encoding/json"
	"errors"
	ast "fizz/proto"
	"flag"
	"fmt"
	"github.com/fizzbee-io/fizzbee/lib"
	"github.com/fizzbee-io/fizzbee/modelchecker"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/pprof"
	"slices"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

var isPlayground bool
var simulation bool
var internalProfile bool
var saveStates bool
var seed int64
var maxRuns int
var explorationStrategy string

var isTest bool

func main() {
	args := parseFlags()
	// Check if the correct number of arguments is provided
	if len(args) != 1 {
		fmt.Println("Usage:", os.Args[0], "<json_file>")
		os.Exit(1)
	}

	// Get the input JSON file name from command line argument
	jsonFilename := args[0]
	dirPath := filepath.Dir(jsonFilename)

	f := loadInputJSON(jsonFilename)

	sourceFileName := filepath.Join(dirPath, f.SourceInfo.GetFileName())
	//fmt.Println("dirPath:", dirPath)
	// Calculate the relative path
	stateConfig := loadStateOptions(dirPath, f.GetFrontMatter())

	fmt.Printf("StateSpaceOptions: %+v\n", stateConfig)
	applyDefaultStateOptions(stateConfig)

	outDir, err := createOutputDir(dirPath, isTest)
	if err != nil {
		return
	}

	if f.Composition != nil {
		runCompositionalModelChecking(f, dirPath, outDir)
	} else {
		modelCheckSingleSpec(f, stateConfig, dirPath, outDir, sourceFileName)
	}
}

func runCompositionalModelChecking(f *ast.File, dirPath string, outDir string) {
	if simulation {
		fmt.Println("Simulation mode not supported for composition")
		return
	}
	roots := make(map[string]*modelchecker.Node)
	// create a map to store the interface preserving states
	// key is the hash, value is the list of (list of nodes)
	joinHashes := make(JoinHashes)
	for i, spec := range f.Composition.GetSpecs() {
		fmt.Printf("Composed Spec %s: %s\n", spec.Name, spec.Expr)
		fileRef, fnRef, err := ParseFunctionRef(spec.Expr.GetPyExpr())
		if err != nil {
			fmt.Println("Error parsing function reference:", err)
			return
		} else if fileRef != spec.Name {
			fmt.Println("File reference does not match spec name:", fileRef, "!=", spec.Name)
			return
		}
		// Get the new json file name as dirPath + "/" + name + ".json"
		composedJsonFileName := filepath.Join(dirPath, spec.Name+".json")
		composedFile := loadInputJSON(composedJsonFileName)
		composedSourceFileName := filepath.Join(dirPath, composedFile.SourceInfo.GetFileName())
		composedStateConfig := loadStateOptions(dirPath, composedFile.GetFrontMatter())
		applyDefaultStateOptions(composedStateConfig)
		fmt.Println("modelCheckSingleSpec:", composedSourceFileName)
		composedOutDir := filepath.Join(outDir, spec.Name)
		if err := os.MkdirAll(composedOutDir, 0755); err != nil {
			fmt.Println("Error creating directory:", err)
			return
		}
		root := modelCheckSingleSpec(composedFile, composedStateConfig, dirPath, composedOutDir, composedSourceFileName)
		if root == nil {
			fmt.Println("Error in model checking composed spec:", spec.Name, "Aborting")
			return
		}
		roots[spec.Name] = root
		populateJoinHashes(joinHashes, i, root, fileRef, fnRef)
	}
	fmt.Println("Composed root nodes:", roots)

	err := ComposeTransitions(joinHashes)
	if err != nil {
		fmt.Println("Error composing transitions:", err)
		return
	}
	return
}

type ComposedNode = []*modelchecker.Node
type ComposedTransition = struct {
	From ComposedNode
	To   ComposedNode
}

func cartesianProduct(sets []TransitionSet, handler func([]lib.Pair[*modelchecker.Node, *modelchecker.Node])) {
	var helper func(int, []lib.Pair[*modelchecker.Node, *modelchecker.Node])
	helper = func(depth int, current []lib.Pair[*modelchecker.Node, *modelchecker.Node]) {
		if depth == len(sets) {
			handler(current)
			return
		}
		for t := range sets[depth] {
			helper(depth+1, append(current, t))
		}
	}
	helper(0, []lib.Pair[*modelchecker.Node, *modelchecker.Node]{})
}

func ComposeTransitions(joinHashes JoinHashes) error {
	for key, sets := range joinHashes {
		if len(sets) < 2 {
			return fmt.Errorf("need at least 2 transition sets for key %s", key)
		}

		cartesianProduct(sets, func(ts []lib.Pair[*modelchecker.Node, *modelchecker.Node]) {
			// Check if all transitions are stuttering
			allStuttering := true
			for _, t := range ts {
				if t.First.Heap.HashCode() != t.Second.Heap.HashCode() {
					allStuttering = false
					break
				}
			}
			if allStuttering {
				return // Skip all-stuttering transitions
			}

			from := make(ComposedNode, len(ts))
			to := make(ComposedNode, len(ts))
			for i, t := range ts {
				from[i] = t.First
				to[i] = t.Second
			}

			composed := ComposedTransition{From: from, To: to}
			// Handle composed transition
			printComposedTransition(composed)
		})
	}
	return nil
}

func printComposedTransition(composed ComposedTransition) {
	fmt.Printf("Composed transition: ")
	for i := range composed.From {
		fmt.Printf("(%s -> %s) ", composed.From[i].Heap.String(), composed.To[i].Heap.String())
	}
	fmt.Println()
}

func ParseFunctionRef(input string) (string, string, error) {
	input = strings.TrimSpace(input)

	if input == "" {
		return "", "", errors.New("input is empty")
	}

	parts := strings.Split(input, ".")
	switch len(parts) {
	//case 1:
	//	funcName := strings.TrimSpace(parts[0])
	//	if funcName == "" {
	//		return "", "", errors.New("function name is required")
	//	}
	//	return "", funcName, nil
	case 2:
		fileRef := strings.TrimSpace(parts[0])
		funcName := strings.TrimSpace(parts[1])
		if fileRef == "" || funcName == "" {
			return "", "", errors.New("file reference or function name is empty")
		}
		return fileRef, funcName, nil
	default:
		return "", "", errors.New("invalid function reference format. Expected format: 'fileName.funcName'")
	}
}

type HashKey string

type Transition = lib.Pair[*modelchecker.Node, *modelchecker.Node]

// type NodeSet map[*modelchecker.Node]bool
type TransitionSet map[Transition]bool
type JoinHashes map[HashKey][]TransitionSet

func populateJoinHashes(joinHashes JoinHashes, i int, root *modelchecker.Node, fileRef string, fnRef string) {
	visited := make(map[*modelchecker.Node]bool)

	var dfs func(from *modelchecker.Node, current *modelchecker.Node)
	dfs = func(from *modelchecker.Node, current *modelchecker.Node) {

		if from != nil && current.IsYieldNode() {
			addToJoinHashes(joinHashes, i, from, current, fnRef)
		}
		if visited[current] {
			return
		}
		visited[current] = true

		if current.IsYieldNode() {
			// Add stuttering (to, to) link for new nodes discovered
			addToJoinHashes(joinHashes, i, current, current, fnRef)
			from = current // update 'from' for child traversal
		}

		for _, child := range current.Outbound {
			if child.Node != nil {
				dfs(from, child.Node)
			}
		}
	}

	dfs(nil, root)
}

func addToJoinHashes(joinHashes JoinHashes, i int, from, to *modelchecker.Node, fnName string) {
	fromState := modelchecker.ExecFunction(from.Process.CloneForAssert(nil, 0), fnName).String()
	toState := modelchecker.ExecFunction(to.Process.CloneForAssert(nil, 0), fnName).String()

	key := HashKey(fromState + "," + toState)

	if _, exists := joinHashes[key]; !exists {
		joinHashes[key] = make([]TransitionSet, i+1)
	}
	for len(joinHashes[key]) <= i {
		joinHashes[key] = append(joinHashes[key], make(TransitionSet))
	}
	if joinHashes[key][i] == nil {
		joinHashes[key][i] = make(TransitionSet)
	}
	pair := lib.NewPair(from, to)
	joinHashes[key][i][pair] = true
}

func modelCheckSingleSpec(f *ast.File, stateConfig *ast.StateSpaceOptions, dirPath string, outDir string, sourceFileName string) *modelchecker.Node {
	//maxRuns := 10000
	if !simulation || seed != 0 {
		maxRuns = 1
	}
	if simulation && seed != 0 {
		fmt.Println("Seed:", seed)
	}
	if simulation && maxRuns == 0 {
		fmt.Println("MaxRuns: unlimited")
	}
	stopped := false
	runs := 0
	var p1 *modelchecker.Processor
	var holder atomic.Pointer[modelchecker.Processor]

	setupSignalHandler(&holder, &stopped)

	i := 0
	for !stopped && (maxRuns <= 0 || i < maxRuns) {
		i++

		p1 = modelchecker.NewProcessor([]*ast.File{f}, stateConfig, simulation, seed, dirPath, explorationStrategy, isTest)
		holder.Store(p1)

		rootNode, failedNode, endTime, err := startModelChecker(p1)
		runs++

		if writeDotFileIfNeeded(p1, rootNode, outDir) {
			return nil
		}

		if err != nil {
			printTraceAndExit(err)
		}

		//fmt.Println("root", root)
		if failedNode == nil {
			var failurePath []*modelchecker.Link
			var failedInvariant *modelchecker.InvariantPosition
			nodes, messages, deadlock, yieldsCount := modelchecker.GetAllNodes(rootNode, stateConfig.GetOptions().GetMaxActions())

			if writeCommunicationFileIfNeeded(messages, outDir) {
				return nil
			}

			if deadlock != nil && stateConfig.GetDeadlockDetection() && !p1.Stopped() && !simulation {
				fmt.Println("DEADLOCK detected")
				fmt.Println("FAILED: Model checker failed")
				if simulation {
					fmt.Println("seed:", p1.Seed)
				}
				dumpFailedNode(sourceFileName, deadlock, rootNode, outDir)
				return nil
			}
			if !simulation {
				fmt.Println("Valid nodes:", len(nodes), "Unique states:", yieldsCount)
				invariants := modelchecker.CheckSimpleExistsWitness(nodes)
				if len(invariants) > 0 {
					fmt.Println("\nFAILED: Expected states never reached")
					for i2, invariant := range invariants {
						fmt.Printf("Invariant %d: %s\n", i2, f.Invariants[invariant.InvariantIndex].Name)
					}
					if !isTest {
						fmt.Println("Time taken to check invariant: ", time.Now().Sub(endTime))
					}
					return nil
				}
			}
			if !simulation && !p1.Stopped() {
				if stateConfig.GetLiveness() == "" || stateConfig.GetLiveness() == "enabled" || stateConfig.GetLiveness() == "true" || stateConfig.GetLiveness() == "strict" || stateConfig.GetLiveness() == "strict/bfs" {
					failurePath, failedInvariant = modelchecker.CheckStrictLiveness(rootNode)
				} else if stateConfig.GetLiveness() == "eventual" || stateConfig.GetLiveness() == "nondeterministic" {
					failurePath, failedInvariant = modelchecker.CheckFastLiveness(nodes)
				}
				fmt.Printf("IsLive: %t\n", failedInvariant == nil)
				if !isTest {
					fmt.Printf("Time taken to check liveness: %v\n", time.Now().Sub(endTime))
				}
			}

			if failedInvariant == nil && !simulation {
				if p1.Stopped() {
					fmt.Println("Model checker stopped")
					return nil
				}
				fmt.Println("PASSED: Model checker completed successfully")
				//nodes, _, _ := modelchecker.GetAllNodes(rootNode)
				if saveStates || !isPlayground {
					nodeFiles, linkFileNames, err := modelchecker.GenerateProtoOfJson(nodes, outDir+"/")
					if err != nil {
						fmt.Println("Error generating proto files:", err)
						return rootNode
					}
					fmt.Printf("Writen %d node files and %d link files to dir %s\n", len(nodeFiles), len(linkFileNames), outDir)
				}
				return rootNode
			} else if failedInvariant != nil {
				fmt.Println("FAILED: Liveness check failed")
				if failedInvariant.FileIndex > 0 {
					fmt.Printf("Only one file expected. Got %d\n", failedInvariant.FileIndex)
				} else {
					fmt.Printf("Invariant: %s\n", f.Invariants[failedInvariant.InvariantIndex].Name)
				}
				GenerateFailurePath(sourceFileName, failurePath, failedInvariant, outDir)
				_, _, err = modelchecker.GenerateErrorPathProtoOfJson(failurePath, outDir+"/")
				if err != nil {
					fmt.Println("Error writing files", err)
				}
				return nil
			}

		} else if failedNode != nil {
			if failedNode.FailedInvariants != nil && len(failedNode.FailedInvariants) > 0 && len(failedNode.FailedInvariants[0]) > 0 {
				fmt.Println("FAILED: Model checker failed. Invariant: ", f.Invariants[failedNode.FailedInvariants[0][0]].Name)
			} else if simulation {
				fmt.Println("FAILED: Model checker failed. Deadlock/stuttering detected")
			}
			if simulation {
				fmt.Println("seed:", p1.Seed)
			}
			dumpFailedNode(sourceFileName, failedNode, rootNode, outDir)
			return nil
		}
	}
	fmt.Println("Stopped after", runs, "runs at ", time.Now())
	return nil
}

func writeCommunicationFileIfNeeded(messages []string, outDir string) bool {
	if len(messages) > 0 && !simulation {
		graphDot := modelchecker.GenerateCommunicationGraph(messages)
		dotFileName := filepath.Join(outDir, "communication.dot")
		// Write the content to the file
		err := os.WriteFile(dotFileName, []byte(graphDot), 0644)
		if err != nil {
			fmt.Println("Error writing to file:", err)
			return true
		}
		if !isPlayground {
			fmt.Printf("Writen communication diagram dotfile: %s\nTo generate svg, run: \n"+
				"dot -Tsvg %s -o communication.svg && open communication.svg\n", dotFileName, dotFileName)
		}
	}
	return false
}

func printTraceAndExit(err error) {
	var modelErr *modelchecker.ModelError
	if errors.As(err, &modelErr) {
		fmt.Println("Stack Trace:")
		fmt.Println(modelErr.SprintStackTrace())
	} else {
		fmt.Println("Error:", err)
	}
	os.Exit(1)
}

func writeDotFileIfNeeded(p1 *modelchecker.Processor, rootNode *modelchecker.Node, outDir string) bool {
	if p1.GetVisitedNodesCount() < 250 {
		dotString := modelchecker.GenerateDotFile(rootNode, make(map[*modelchecker.Node]bool))
		dotFileName := filepath.Join(outDir, "graph.dot")
		// Write the content to the file
		err := os.WriteFile(dotFileName, []byte(dotString), 0644)
		if err != nil {
			fmt.Println("Error writing to file:", err)
			return true
		}
		if !isPlayground && !simulation {
			fmt.Printf("Writen graph dotfile: %s\nTo generate svg, run: \n"+
				"dot -Tsvg %s -o graph.svg && open graph.svg\n", dotFileName, dotFileName)
		}
	} else if !simulation {
		fmt.Printf("Skipping dotfile generation. Too many nodes: %d\n", p1.GetVisitedNodesCount())
	}
	return false
}

func setupSignalHandler(holder *atomic.Pointer[modelchecker.Processor], stopped *bool) {
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println("\nInterrupted. Stopping state exploration")
		*stopped = true
		p1 := holder.Load()
		if p1 != nil {
			p1.Stop()
		}
	}()
}

func applyDefaultStateOptions(stateConfig *ast.StateSpaceOptions) {
	if stateConfig.Options.MaxActions == 0 {
		stateConfig.Options.MaxActions = 100
	}
	if stateConfig.Options.MaxConcurrentActions == 0 {
		stateConfig.Options.MaxConcurrentActions = min(2, stateConfig.Options.MaxActions)
	}
	if stateConfig.DeadlockDetection == nil {
		deadlockDetection := true
		stateConfig.DeadlockDetection = &deadlockDetection
	}
	if stateConfig.Options.CrashOnYield == nil {
		crashOnYield := true
		stateConfig.Options.CrashOnYield = &crashOnYield
	}
}

func loadStateOptions(dirPath string, f *ast.FrontMatter) *ast.StateSpaceOptions {
	configFileName := filepath.Join(dirPath, "fizz.yaml")
	fmt.Println("configFileName:", configFileName)
	stateConfig, err := modelchecker.ReadOptionsFromYaml(configFileName)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if isPlayground {
				deadlockDetection := true
				crashOnYield := true
				stateConfig = &ast.StateSpaceOptions{
					Options:           &ast.Options{MaxActions: 100, MaxConcurrentActions: 2, CrashOnYield: &crashOnYield},
					Liveness:          "strict",
					DeadlockDetection: &deadlockDetection,
				}
			} else {
				fmt.Println("fizz.yaml not found. Using default options")
				stateConfig = &ast.StateSpaceOptions{Options: &ast.Options{MaxActions: 100, MaxConcurrentActions: 2}}
			}
		} else {
			fmt.Println("Error reading fizz.yaml:", err)
			os.Exit(1)
		}

	}
	if f.GetYaml() != "" {
		fmStateConfig, err := modelchecker.ReadOptionsFromYamlString(f.GetYaml())
		if err != nil {
			fmt.Println("Error parsing YAML frontmatter:", err)
			os.Exit(1)
		}
		proto.Merge(stateConfig, fmStateConfig)
	}
	return stateConfig
}

func loadInputJSON(jsonFilename string) *ast.File {
	// Read the content of the JSON file
	jsonContent, err := os.ReadFile(jsonFilename)
	if err != nil {
		fmt.Println("Error reading JSON file:", err)
		os.Exit(1)
	}
	f := &ast.File{}
	err = protojson.Unmarshal(jsonContent, f)
	if err != nil {
		fmt.Println("Error unmarshalling JSON:", err)
		os.Exit(1)
	}
	return f
}

func parseFlags() []string {
	flag.BoolVar(&isPlayground, "playground", false, "is for playground")
	flag.BoolVar(&simulation, "simulation", false, "Runs in simulation mode (DFS). Default=false for no simulation (BFS)")
	flag.BoolVar(&internalProfile, "internal_profile", false, "Enables CPU and memory profiling of the model checker")
	flag.BoolVar(&saveStates, "save_states", false, "Save states to disk")
	flag.Int64Var(&seed, "seed", 0, "Seed for random number generator used in simulation mode")
	flag.IntVar(&maxRuns, "max_runs", 0, "Maximum number of simulation runs/paths to explore. Default=0 for unlimited")
	flag.StringVar(&explorationStrategy, "exploration_strategy", "bfs", "Exploration strategy for exhaustive model checking. Options: bfs (default), dfs, random.")
	flag.BoolVar(&isTest, "test", false, "Testing mode (prevents printing timestamps and other non-deterministic behavior. Default=false")
	flag.Parse()

	args := flag.Args()
	return args
}

func startModelChecker(p1 *modelchecker.Processor) (*modelchecker.Node, *modelchecker.Node, time.Time, error) {
	if simulation {
		rootNode, failedNode, _ := p1.Start()
		return rootNode, failedNode, time.Now(), nil
	}
	if internalProfile {
		startCpuProfile()
		defer pprof.StopCPUProfile()
	}
	startTime := time.Now()
	rootNode, failedNode, err := p1.Start()
	endTime := time.Now()
	if !isTest {
		fmt.Printf("Time taken for model checking: %v\n", endTime.Sub(startTime))
	}
	if internalProfile {
		startHeapProfile()
	}
	return rootNode, failedNode, endTime, err
}

func startCpuProfile() {
	// Start CPU profiling
	f, err := os.Create("cpu.pprof")
	if err != nil {
		panic(err)
	}
	err = pprof.StartCPUProfile(f)
	if err != nil {
		panic(err)
	}
}

func startHeapProfile() {
	f, err := os.Create("mem.pprof")
	if err != nil {
		panic(err)
	}
	err = pprof.WriteHeapProfile(f)
	if err != nil {
		panic(err)
	}
}

func dumpFailedNode(srcFileName string, failedNode *modelchecker.Node, rootNode *modelchecker.Node, outDir string) {
	failurePath := make([]*modelchecker.Link, 0)
	node := failedNode
	for node != nil {

		if len(node.Inbound) == 0 || node.Name == "init" || node == rootNode {
			link := modelchecker.InitNodeToLink(node)
			failurePath = append(failurePath, link)
			break
		}
		outLink := modelchecker.ReverseLink(node, node.Inbound[0])
		failurePath = append(failurePath, outLink)
		//node.Name = node.Name + "/" + node.Inbound[0].Name
		node = node.Inbound[0].Node
	}
	slices.Reverse(failurePath)
	GenerateFailurePath(srcFileName, failurePath, nil, outDir)
	_, _, err := modelchecker.GenerateErrorPathProtoOfJson(failurePath, outDir+"/")
	if err != nil {
		fmt.Println("Error writing files", err)
	}
}

func GenerateFailurePath(srcFileName string, failurePath []*modelchecker.Link, invariant *modelchecker.InvariantPosition, outDir string) {
	for _, link := range failurePath {
		node := link.Node
		stepName := link.Name

		fmt.Printf("------\n%s\n", stepName)

		nodeStr := node.Heap.ToJson()
		nodeStr = strings.ReplaceAll(nodeStr, lib.SymmetryPrefix, "")
		fmt.Printf("--\nstate: %s\n", nodeStr)
		if len(node.Returns) > 0 {
			fmt.Printf("returns: %s\n", strings.ReplaceAll(node.Returns.String(), lib.SymmetryPrefix, ""))
		}
	}
	fmt.Println("------")
	if !isPlayground {
		errJsonFileName := filepath.Join(outDir, "error-graph.json")
		bytes, err := json.MarshalIndent(failurePath, "", "  ")
		if err != nil {
			fmt.Println("Error creating json:", err)
		}
		err = os.WriteFile(errJsonFileName, bytes, 0644)
		if err != nil {
			fmt.Println("Error writing to file:", err)
			return
		}
		fmt.Printf("Writen graph json: %s\n", errJsonFileName)
	}

	dotStr := modelchecker.GenerateFailurePath(failurePath, invariant)
	//fmt.Println(dotStr)
	dotFileName := filepath.Join(outDir, "error-graph.dot")
	// Write the content to the file
	err := os.WriteFile(dotFileName, []byte(dotStr), 0644)
	if err != nil {
		fmt.Println("Error writing to file:", err)
		return
	}
	if !isPlayground {
		fmt.Printf("Writen graph dotfile: %s\nTo generate an image file, run: \n"+
			"dot -Tsvg %s -o error-graph.svg && open error-graph.svg\n", dotFileName, dotFileName)
	}
	err = modelchecker.GenerateFailurePathHtml(srcFileName, failurePath, invariant, outDir)
	if err != nil {
		return
	}
	if !isPlayground {
		fmt.Printf("Writen error states as html: %s/error-states.html\nTo open: \n"+
			"open %s/error-states.html\n", outDir, outDir)
	}
}

func createOutputDir(dirPath string, testing bool) (string, error) {
	var newDirName string
	if testing {
		newDirName = "run_test"
	} else {
		// Create the directory name with current date and time
		dateTimeStr := time.Now().Format("2006-01-02_15-04-05") // Format: YYYY-MM-DD_HH-MM-SS
		newDirName = fmt.Sprintf("run_%s", dateTimeStr)
	}

	// Create the full path for the new directory
	newDirPath := filepath.Join(dirPath, "out", newDirName)

	// Create the directory
	if err := os.MkdirAll(newDirPath, 0755); err != nil {
		fmt.Println("Error creating directory:", err)
		return newDirPath, err
	}

	// Define the symlink path
	latestSymlinkPath := filepath.Join(dirPath, "out", "latest")

	// Remove the existing symlink if it exists
	if _, err := os.Lstat(latestSymlinkPath); err == nil {
		if err := os.Remove(latestSymlinkPath); err != nil {
			fmt.Println("Error removing existing symlink:", err)
			return newDirPath, err
		}
	}
	// Convert to absolute path
	absNewDirPath, err := filepath.Abs(newDirPath)
	if err != nil {
		fmt.Println("Error resolving absolute path:", err)
		return "", err
	}
	// Create the new symlink
	if err := os.Symlink(absNewDirPath, latestSymlinkPath); err != nil {
		fmt.Println("Error creating symlink:", err)
		return newDirPath, err
	}
	// Still returning the newDirPath instead of the symlink path
	// So, all the output logs will still point to the newDirPath.
	// This reduces issues when multiple executions are run in parallel.
	return newDirPath, nil
}
