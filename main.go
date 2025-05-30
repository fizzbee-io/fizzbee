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
	rootsList := make([]*modelchecker.Node, len(f.Composition.GetSpecs()))
	// create a map to store the interface preserving states
	// key is the hash, value is the list of (list of Nodes)
	joinHashes := make(JoinHashes)
	for i, spec := range f.Composition.GetSpecs() {
		fmt.Printf("\nComposed Spec: %s\n", spec.Name)
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
		composedOutDir := filepath.Join(outDir, spec.Name)
		if err := os.MkdirAll(composedOutDir, 0755); err != nil {
			fmt.Println("Error creating directory:", err)
			return
		}
		fmt.Println("Model checking composed spec:", composedSourceFileName)
		root := modelCheckSingleSpec(composedFile, composedStateConfig, dirPath, composedOutDir, composedSourceFileName)
		if root == nil {
			fmt.Println("Error in model checking composed spec:", spec.Name, "Aborting")
			return
		}
		roots[spec.Name] = root
		rootsList[i] = root
		populateJoinHashes(joinHashes, i, root, fileRef, fnRef)
	}

	err := ComposeTransitions(f, joinHashes, rootsList, outDir)
	if err != nil {
		fmt.Println("Error composing transitions:", err)
		return
	}
	return
}

type ComposedNode = struct {
	*modelchecker.Node
	Nodes []*modelchecker.Node
}

type ComposedTransition = struct {
	From *ComposedNode
	To   *ComposedNode
}

func cartesianProduct(sets []TransitionSet, handler func(ts []lib.Pair[*modelchecker.Node, *modelchecker.Node]) (
	*ComposedNode, *ComposedTransition)) (*ComposedNode, *ComposedTransition) {

	var helper func(int, []lib.Pair[*modelchecker.Node, *modelchecker.Node])
	var failedNode *ComposedNode
	var failedTransition *ComposedTransition
	helper = func(depth int, current []lib.Pair[*modelchecker.Node, *modelchecker.Node]) {
		if depth == len(sets) {
			failedNode, failedTransition = handler(current)
			return
		}
		for t := range sets[depth] {
			helper(depth+1, append(current, t))
			if failedNode != nil || failedTransition != nil {
				return // Stop further processing if a failure is found
			}
		}
	}
	helper(0, []lib.Pair[*modelchecker.Node, *modelchecker.Node]{})
	if failedTransition != nil {
		fmt.Println("Failed Transition:", failedTransition.From.Nodes, "->", failedTransition.To.Nodes)
	} else if failedNode != nil {
		fmt.Println("Failed Node:", failedNode.Nodes)
	}
	return failedNode, failedTransition
}

func ComposeTransitions(f *ast.File, joinHashes JoinHashes, roots []*modelchecker.Node, outDir string) error {
	hasTransitionInvariant := false
	for _, invariant := range f.Invariants {
		if invariant.Block != nil && slices.Contains(invariant.TemporalOperators, "transition") {
			hasTransitionInvariant = true
			break
		}
	}
	for key, sets := range joinHashes {
		if len(sets) < 2 {
			return fmt.Errorf("need at least 2 transition sets for key %s", key)
		}

		failedNode, failedTransition := cartesianProduct(sets, func(ts []lib.Pair[*modelchecker.Node, *modelchecker.Node]) (*ComposedNode, *ComposedTransition) {
			// Check if all transitions are stuttering
			allStuttering := true
			for _, t := range ts {
				if t.First.Heap.HashCode() != t.Second.Heap.HashCode() {
					allStuttering = false
					break
				}
			}
			if allStuttering {
				return nil, nil // Skip all-stuttering transitions
			}
			composedFrom := make(map[string]*modelchecker.Heap)
			composedTo := make(map[string]*modelchecker.Heap)
			from := &ComposedNode{Nodes: make([]*modelchecker.Node, len(ts))}
			to := &ComposedNode{Nodes: make([]*modelchecker.Node, len(ts))}

			for i, t := range ts {
				from.Nodes[i] = t.First
				to.Nodes[i] = t.Second

				composedFrom[f.Composition.GetSpecs()[i].GetName()] = t.First.Heap
				composedTo[f.Composition.GetSpecs()[i].GetName()] = t.Second.Heap
			}

			composed := &ComposedTransition{From: from, To: to}
			// Handle composed transition
			processTo := modelchecker.NewProcess("yield", []*ast.File{f}, nil)
			processTo.Enable()
			processTo.Heap = modelchecker.NewComposedHeap(composedTo)
			failedInvariants := modelchecker.CheckInvariants(processTo)
			to.Node = modelchecker.NewNode(processTo)
			if len(failedInvariants) > 0 && len(failedInvariants[0]) > 0 {
				processTo.FailedInvariants = failedInvariants
				fmt.Println("Composed state failed invariants:", failedInvariants)
				processTo.FailedInvariants = failedInvariants
				return to, nil
			}
			if hasTransitionInvariant {
				processFrom := modelchecker.NewProcess("yield", []*ast.File{f}, nil)
				processFrom.Enable()
				processFrom.Heap = modelchecker.NewComposedHeap(composedFrom)
				processTo.Parent = processFrom
				failedTransitionInvariants := modelchecker.CheckTransitionInvariants(processTo)
				if len(failedTransitionInvariants) > 0 && len(failedTransitionInvariants[0]) > 0 {
					//processTo.FailedInvariants = failedInvariants
					fmt.Println("Composed transition failed invariants:", failedTransitionInvariants)
					link := &modelchecker.Link{
						Node:             to.Node,
						Name:             "composed-transition",
						FailedInvariants: failedTransitionInvariants,
					}
					from.Node = modelchecker.NewNode(processFrom)
					from.Outbound = append(from.Outbound, link)
					to.Inbound = append(to.Inbound, link)
					return to, composed
				}
			}

			return nil, nil
		})
		if failedNode != nil || failedTransition != nil {
			if failedTransition != nil {
				fmt.Println("Failed Composed Node:", failedTransition)
				printComposedTransition(failedTransition)
				dumpComposedFailedTransition(failedTransition, f.Composition, roots, outDir)
			} else /*if failedNode != nil*/ {
				fmt.Println("Failed Composed Node:", failedNode.Nodes)
				dumpComposedFailedNode(failedNode, f.Composition, roots, outDir)
			}
			return fmt.Errorf("failed to compose transitions for key %s", key)
		}
	}
	return nil
}

func printComposedTransition(composed *ComposedTransition) {
	fmt.Printf("Composed transition: ")
	for i := range composed.From.Nodes {
		fmt.Printf("(%s -> %s) ", composed.From.Nodes[i].Heap.String(), composed.To.Nodes[i].Heap.String())
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
			// Add stuttering (to, to) link for new Nodes discovered
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
				fmt.Println("Valid Nodes:", len(nodes), "Unique states:", yieldsCount)
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
				//Nodes, _, _ := modelchecker.GetAllNodes(rootNode)
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
	if p1.GetVisitedNodesCount() < 250 || (simulation && p1.GetVisitedNodesCount() < 1000) {
		dotString := modelchecker.GenerateDotFile(rootNode, make(map[*modelchecker.Node]bool))
		dotFileName := filepath.Join(outDir, "graph.dot")
		// Write the content to the file
		err := os.WriteFile(dotFileName, []byte(dotString), 0644)
		if err != nil {
			fmt.Println("Error writing to file:", err)
			return true
		}
		if !isPlayground {
			fmt.Printf("Writen graph dotfile: %s\nTo generate svg, run: \n"+
				"dot -Tsvg %s -o graph.svg && open graph.svg\n", dotFileName, dotFileName)
		}
	} else if !simulation {
		fmt.Printf("Skipping dotfile generation. Too many Nodes: %d\n", p1.GetVisitedNodesCount())
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

	failurePath := extractFailurePath(failedNode, rootNode)
	GenerateFailurePath(srcFileName, failurePath, nil, outDir)
	_, _, err := modelchecker.GenerateErrorPathProtoOfJson(failurePath, outDir+"/")
	if err != nil {
		fmt.Println("Error writing files", err)
	}
}

func GenerateComposedFailurePath(failed *ComposedNode, composition *ast.Composition, roots []*modelchecker.Node, outDir string) {
	// Collect failure paths from each composing spec
	componentPaths := make([][]*modelchecker.Link, len(failed.Nodes))
	subgraphs := make([]string, len(failed.Nodes))
	lastNodeIDs := make([]string, len(failed.Nodes))

	builder := strings.Builder{}
	builder.WriteString("digraph G {\n")

	for i, node := range failed.Nodes {
		specName := composition.GetSpecs()[i].GetName()
		componentPaths[i] = extractFailurePath(node, roots[i])
		subgraph := modelchecker.GenerateFailurePath(componentPaths[i], specName, nil)
		subgraphs[i] = subgraph
		builder.WriteString(subgraph)

		// ID of the last node in this path
		lastNodeIDs[i] = fmt.Sprintf("\"%s_%d\"", specName, len(componentPaths[i])-1)
	}

	// Add the merged virtual node
	mergedID := "\"merged\""
	builder.WriteString(fmt.Sprintf("  %s [label=\"merged\", shape=doubleoctagon, style=filled, fillcolor=lightgray];\n", mergedID))
	fmt.Println(failed)
	// Add edges from each last node to merged node
	for _, id := range lastNodeIDs {
		builder.WriteString(fmt.Sprintf("  %s -> %s [arrowhead=none, style=dashed, color=gray];\n", id, mergedID))
	}

	builder.WriteString("}\n")
	dotStr := builder.String()
	err := writeErrorDotFile(outDir, dotStr)
	if err != nil {
		return
	}
}

func dumpComposedFailedNode(failed *ComposedNode, composition *ast.Composition, roots []*modelchecker.Node, outDir string) {
	GenerateComposedFailurePath(failed, composition, roots, outDir)
}

func dumpComposedFailedTransition(failed *ComposedTransition, composition *ast.Composition, roots []*modelchecker.Node, outDir string) {
	GenerateTransitionFailurePath(failed.From, failed.To, composition, roots, outDir)
}

func GenerateTransitionFailurePath(from, to *ComposedNode, composition *ast.Composition, roots []*modelchecker.Node, outDir string) {
	lastFromNodeIDs := make([]string, len(from.Nodes))
	lastToNodeIDs := make([]string, len(from.Nodes))

	builder := strings.Builder{}
	builder.WriteString("digraph G {\n")

	for i := range from.Nodes {
		specName := composition.GetSpecs()[i].GetName()

		// Path from root to `from`
		path := extractFailurePath(from.Nodes[i], roots[i])
		lastFromIndex := len(path) - 1

		// Handle transition to `to`
		if from.Nodes[i] == to.Nodes[i] {
			// Add stutter node manually
			lastFromNodeIDs[i] = fmt.Sprintf("\"%s_%d\"", specName, lastFromIndex)
			lastToNodeIDs[i] = lastFromNodeIDs[i]
			builder.WriteString(modelchecker.GenerateFailurePath(path, specName, nil))
			builder.WriteString(fmt.Sprintf("  %s -> %s [label=\"stutter\", color=\"red\", style=dashed];\n", lastFromNodeIDs[i], lastToNodeIDs[i]))
			continue
		}

		// Extract transition path and append
		transition := extractTransitionPath(from.Nodes[i], to.Nodes[i])
		for _, link := range transition {
			link.FailedInvariants = map[int][]int{0: {0}} // Clear failed invariants for transition links
		}
		path = append(path, transition...)

		// Generate full path subgraph
		builder.WriteString(modelchecker.GenerateFailurePath(path, specName, nil))

		// Set FROM node id (before transition path)
		lastFromNodeIDs[i] = fmt.Sprintf("\"%s_%d\"", specName, lastFromIndex)

		// Set TO node id (end of entire path)
		lastToIndex := len(path) - 1
		lastToNodeIDs[i] = fmt.Sprintf("\"%s_%d\"", specName, lastToIndex)
	}

	// Virtual merge node for FROM
	mergedFromID := "\"composite_before\""
	addCompositeNode(builder, mergedFromID, lastFromNodeIDs)

	// Virtual merge node for TO
	mergedToID := "\"composite_after\""
	addCompositeNode(builder, mergedToID, lastToNodeIDs)
	builder.WriteString(fmt.Sprintf("  %s -> %s [style=dashed, color=red];\n", mergedFromID, mergedToID))
	builder.WriteString("}\n")
	dotStr := builder.String()
	err := writeErrorDotFile(outDir, dotStr)
	if err != nil {
		return
	}
}

func addCompositeNode(builder strings.Builder, mergedFromID string, lastFromNodeIDs []string) {
	builder.WriteString(fmt.Sprintf("  %s [label=\"Composite before\", shape=doubleoctagon, style=filled, fillcolor=orange];\n", mergedFromID))
	for _, id := range lastFromNodeIDs {
		builder.WriteString(fmt.Sprintf("  %s -> %s [arrowhead=none, style=dashed, color=gray];\n", id, mergedFromID))
	}
}

func extractTransitionPath(from *modelchecker.Node, to *modelchecker.Node) []*modelchecker.Link {
	type pathEntry struct {
		node *modelchecker.Node
		path []*modelchecker.Link
	}

	visited := make(map[*modelchecker.Node]bool)
	queue := []pathEntry{{node: from, path: []*modelchecker.Link{}}}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		// Prevent revisiting
		if visited[current.node] {
			continue
		}
		visited[current.node] = true

		for _, link := range current.node.Outbound {
			nextNode := link.Node

			// Skip self-loops
			if nextNode == current.node {
				continue
			}

			newPath := append(append([]*modelchecker.Link{}, current.path...), link)

			// If this is the destination node
			if nextNode == to {
				return newPath
			}

			// If it's not a yield node, we can go deeper
			if !nextNode.IsYieldNode() {
				queue = append(queue, pathEntry{node: nextNode, path: newPath})
			} else {
				// If it's a yield node but not the target, stop — we don't want to overshoot
				continue
			}
		}
	}

	// No path found
	return []*modelchecker.Link{}
}

func extractFailurePath(node *modelchecker.Node, rootNode *modelchecker.Node) []*modelchecker.Link {
	failurePath := make([]*modelchecker.Link, 0)
	for node != nil {

		if len(node.Inbound) == 0 || node.Name == "init" || node == rootNode {
			link := modelchecker.InitNodeToLink(node)
			failurePath = append(failurePath, link)
			break
		}
		outLink := modelchecker.ReverseLink(node, node.Inbound[0])
		failurePath = append(failurePath, outLink)
		node = node.Inbound[0].Node
	}
	slices.Reverse(failurePath)
	return failurePath
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

	dotStr := modelchecker.GenerateFailurePath(failurePath, "", invariant)
	err := writeErrorDotFile(outDir, dotStr)
	if err != nil {
		return
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

func writeErrorDotFile(outDir string, dotStr string) error {
	dotFileName := filepath.Join(outDir, "error-graph.dot")
	// Write the content to the file
	err := os.WriteFile(dotFileName, []byte(dotStr), 0644)
	if err != nil {
		fmt.Println("Error writing to file:", err)
		return err
	}
	if !isPlayground {
		fmt.Printf("Writen graph dotfile: %s\nTo generate an image file, run: \n"+
			"dot -Tsvg %s -o error-graph.svg && open error-graph.svg\n", dotFileName, dotFileName)
	}
	return nil
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
