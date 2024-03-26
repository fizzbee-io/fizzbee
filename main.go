package main

import (
    "encoding/json"
    "errors"
    ast "fizz/proto"
    "flag"
    "fmt"
    "github.com/jayaprabhakar/fizzbee/modelchecker"
    "google.golang.org/protobuf/encoding/protojson"
    "os"
    "path/filepath"
    "slices"
    "time"
)

var isPlayground bool

func main() {
    flag.BoolVar(&isPlayground, "playground", false, "is for playground")
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
                stateConfig = &ast.StateSpaceOptions{
                    Options: &ast.Options{MaxActions: 100, MaxConcurrentActions: 2},
                    Liveness: "strict",
                    DeadlockDetection: true,
                }
            } else {
                fmt.Println("fizz.yaml not found. Using default options")
                stateConfig = &ast.StateSpaceOptions{Options: &ast.Options{MaxActions: 100, MaxConcurrentActions: 5}}
            }
        } else {
            fmt.Println("Error reading fizz.yaml:", err)
            os.Exit(1)
        }

    }
    fmt.Printf("StateSpaceOptions: %+v\n", stateConfig)
    if stateConfig.Options.MaxConcurrentActions == 0 {
        stateConfig.Options.MaxConcurrentActions = stateConfig.Options.MaxActions
    }

    p1 := modelchecker.NewProcessor([]*ast.File{f}, stateConfig)
    startTime := time.Now()
    rootNode, failedNode, err := p1.Start()
    endTime := time.Now()
    fmt.Printf("Time taken for model checking: %v\n", endTime.Sub(startTime))

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
            fmt.Printf("Writen graph dotfile: %s\nTo generate png, run: \n" +
                "dot -Tpng %s -o graph.png && open graph.png\n", dotFileName, dotFileName)
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
        //failurePath := nil
        //failedInvariant := nil
        var failurePath []*modelchecker.Link
        var failedInvariant *modelchecker.InvariantPosition
        nodes, deadlock, _ := modelchecker.GetAllNodes(rootNode)
        if deadlock != nil && stateConfig.GetDeadlockDetection() {
            fmt.Println("DEADLOCK detected")
            fmt.Println("FAILED: Model checker failed")
            dumpFailedNode(deadlock, rootNode, outDir)
            return
        }
        if stateConfig.GetLiveness() == "strict" || stateConfig.GetLiveness() == "strict/bfs" {
            failurePath, failedInvariant = modelchecker.CheckStrictLiveness(rootNode)
            fmt.Printf("IsLive: %t\n", failedInvariant == nil)
            fmt.Printf("Time taken to check liveness: %v\n", time.Now().Sub(endTime))
        } else if stateConfig.GetLiveness() == "eventual" {
            failurePath, failedInvariant = modelchecker.CheckFastLiveness(nodes)
            fmt.Printf("IsLive: %t\n", failedInvariant == nil)
            fmt.Printf("Time taken to check liveness: %v\n", time.Now().Sub(endTime))
        }

        if failedInvariant == nil {
            fmt.Println("PASSED: Model checker completed successfully")
            //nodes, _, _ := modelchecker.GetAllNodes(rootNode)
            if !isPlayground {
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
    fmt.Println("FAILED: Model checker failed")

    dumpFailedNode(failedNode, rootNode, outDir)
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

        fmt.Printf("--\nstate: %s\n", node.Heap.ToJson())
        if len(node.Returns) > 0 {
            fmt.Printf("returns: %s\n", node.Returns.String())
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
        fmt.Printf("Writen graph dotfile: %s\nTo generate png, run: \n"+
            "dot -Tpng %s -o error-graph.png && open error-graph.png\n", dotFileName, dotFileName)
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
