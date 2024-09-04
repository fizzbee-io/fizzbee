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
    "syscall"
    "time"
)

var isPlayground bool
var simulation bool
var internalProfile bool
var saveStates bool

func main() {
    flag.BoolVar(&isPlayground, "playground", false, "is for playground")
    flag.BoolVar(&simulation, "simulation", false, "Runs in simulation mode (DFS). Default=false for no simulation (BFS)")
    flag.BoolVar(&internalProfile, "internal_profile", false, "Enables CPU and memory profiling of the model checker")
    flag.BoolVar(&saveStates, "save_states", false, "Save states to disk")
    flag.Parse()

    args := flag.Args()
    // Check if the correct number of arguments is provided
    if len(args) != 1 {
        fmt.Println("Usage:", os.Args[0], "<json_file>")
        os.Exit(1)
    }

    // Get the input JSON file name from command line argument
    jsonFilename := args[0]

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

    dirPath := filepath.Dir(jsonFilename)
    //fmt.Println("dirPath:", dirPath)
    // Calculate the relative path
    configFileName := filepath.Join(dirPath, "fizz.yaml")
    fmt.Println("configFileName:", configFileName)
    stateConfig, err := modelchecker.ReadOptionsFromYaml(configFileName)
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            if isPlayground {
                deadlockDetection := true
                stateConfig = &ast.StateSpaceOptions{
                    Options: &ast.Options{MaxActions: 100, MaxConcurrentActions: 2},
                    Liveness: "strict",
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
    if f.GetFrontMatter().GetYaml() != "" {
        fmStateConfig, err := modelchecker.ReadOptionsFromYamlString(f.GetFrontMatter().GetYaml())
        if err != nil {
            fmt.Println("Error parsing YAML frontmatter:", err)
            os.Exit(1)
        }
        proto.Merge(stateConfig, fmStateConfig)
    }

    fmt.Printf("StateSpaceOptions: %+v\n", stateConfig)
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

    p1 := modelchecker.NewProcessor([]*ast.File{f}, stateConfig, simulation, dirPath)

    c := make(chan os.Signal)
    signal.Notify(c, os.Interrupt, syscall.SIGTERM)
    go func() {
        <-c
        fmt.Println("\nInterrupted. Stopping state exploration")
        p1.Stop()
    }()

    rootNode, failedNode, endTime := startModelChecker(err, p1)

    outDir, err := createOutputDir(dirPath)
    if err != nil {
        return
    }
    if p1.GetVisitedNodesCount() < 250 {
        dotString := modelchecker.GenerateDotFile(rootNode, make(map[*modelchecker.Node]bool))
        dotFileName := filepath.Join(outDir, "graph.dot")
        // Write the content to the file
        err := os.WriteFile(dotFileName, []byte(dotString), 0644)
        if err != nil {
            fmt.Println("Error writing to file:", err)
            return
        }
        if !isPlayground {
            fmt.Printf("Writen graph dotfile: %s\nTo generate svg, run: \n" +
                "dot -Tsvg %s -o graph.svg && open graph.svg\n", dotFileName, dotFileName)
        }
    } else {
        fmt.Printf("Skipping dotfile generation. Too many nodes: %d\n", p1.GetVisitedNodesCount())
    }

    if err != nil {
        var modelErr *modelchecker.ModelError
        if errors.As(err, &modelErr) {
            fmt.Println("Stack Trace:")
            fmt.Println(modelErr.SprintStackTrace())
        } else {
            fmt.Println("Error:", err)
        }
        os.Exit(1)
    }

    //fmt.Println("root", root)
    if failedNode == nil {
        var failurePath []*modelchecker.Link
        var failedInvariant *modelchecker.InvariantPosition
        nodes, messages, deadlock, _ := modelchecker.GetAllNodes(rootNode)

        if len(messages) > 0 {
            graphDot := modelchecker.GenerateCommunicationGraph(messages)
            dotFileName := filepath.Join(outDir, "communication.dot")
            // Write the content to the file
            err := os.WriteFile(dotFileName, []byte(graphDot), 0644)
            if err != nil {
                fmt.Println("Error writing to file:", err)
                return
            }
            if !isPlayground {
                fmt.Printf("Writen communication diagram dotfile: %s\nTo generate svg, run: \n" +
                    "dot -Tsvg %s -o communication.svg && open communication.svg\n", dotFileName, dotFileName)
            }
        }

        if deadlock != nil && stateConfig.GetDeadlockDetection() && !p1.Stopped() {
            fmt.Println("DEADLOCK detected")
            fmt.Println("FAILED: Model checker failed")
            dumpFailedNode(deadlock, rootNode, outDir)
            return
        }
        if !p1.Stopped() {
            if stateConfig.GetLiveness() == "" || stateConfig.GetLiveness() == "strict" || stateConfig.GetLiveness() == "strict/bfs" {
                failurePath, failedInvariant = modelchecker.CheckStrictLiveness(rootNode)
                fmt.Printf("IsLive: %t\n", failedInvariant == nil)
                fmt.Printf("Time taken to check liveness: %v\n", time.Now().Sub(endTime))
            } else if stateConfig.GetLiveness() == "eventual" {
                failurePath, failedInvariant = modelchecker.CheckFastLiveness(nodes)
                fmt.Printf("IsLive: %t\n", failedInvariant == nil)
                fmt.Printf("Time taken to check liveness: %v\n", time.Now().Sub(endTime))
            }
        }

        if failedInvariant == nil {
            fmt.Println("PASSED: Model checker completed successfully")
            //nodes, _, _ := modelchecker.GetAllNodes(rootNode)
            if saveStates || !isPlayground {
                nodeFiles, linkFileNames, err := modelchecker.GenerateProtoOfJson(nodes, outDir+"/")
                if err != nil {
                    fmt.Println("Error generating proto files:", err)
                    return
                }
                fmt.Printf("Writen %d node files and %d link files to dir %s\n", len(nodeFiles), len(linkFileNames), outDir)
            }
        } else {
            fmt.Println("FAILED: Liveness check failed")
            if failedInvariant.FileIndex > 0 {
                fmt.Printf("Only one file expected. Got %d\n", failedInvariant.FileIndex)
            } else {
                fmt.Printf("Invariant: %s\n", f.Invariants[failedInvariant.InvariantIndex].Name)
            }
            GenerateFailurePath(failurePath, failedInvariant, outDir)
        }

        return
    }
    fmt.Println("FAILED: Model checker failed. Invariant: ", f.Invariants[failedNode.FailedInvariants[0][0]].Name)

    dumpFailedNode(failedNode, rootNode, outDir)
}


func startModelChecker(err error, p1 *modelchecker.Processor) (*modelchecker.Node, *modelchecker.Node, time.Time) {
    if internalProfile {
        startCpuProfile()
        defer pprof.StopCPUProfile()
    }
    startTime := time.Now()
    rootNode, failedNode, err := p1.Start()
    endTime := time.Now()
    fmt.Printf("Time taken for model checking: %v\n", endTime.Sub(startTime))
    if internalProfile {
        startHeapProfile()
    }
    return rootNode, failedNode, endTime
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


func dumpFailedNode(failedNode *modelchecker.Node, rootNode *modelchecker.Node, outDir string) {
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
    GenerateFailurePath(failurePath, nil, outDir)
}

func GenerateFailurePath(failurePath []*modelchecker.Link, invariant *modelchecker.InvariantPosition, outDir string) {
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
}

func createOutputDir(dirPath string) (string, error) {
    // Create the directory name with current date and time
    dateTimeStr := time.Now().Format("2006-01-02_15-04-05") // Format: YYYY-MM-DD_HH-MM-SS
    newDirName := fmt.Sprintf("run_%s", dateTimeStr)

    // Create the full path for the new directory
    newDirPath := filepath.Join(dirPath, "out", newDirName)

    // Create the directory
    if err := os.MkdirAll(newDirPath, 0755); err != nil {
        fmt.Println("Error creating directory:", err)
        return newDirPath, err
    }
    return newDirPath, nil
}
